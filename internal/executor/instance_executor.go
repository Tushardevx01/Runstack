package executor

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/runtime"
)

// InstanceExecutor manages long-running Instances on the Agent.
type InstanceExecutor struct {
	NodeID      string
	APIClient   *api.Client
	Runtime     runtime.ContainerRuntime
	Ports       *node.PortAllocator
	ctx         context.Context
	cancel      context.CancelFunc
	proberMu    sync.Mutex
	probers     map[string]context.CancelFunc
	activeSpecs map[string]application.AppSpec
	activePorts map[string]int
}

func NewInstanceExecutor(nodeID string, apiClient *api.Client, cr runtime.ContainerRuntime) *InstanceExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	return &InstanceExecutor{
		NodeID:      nodeID,
		APIClient:   apiClient,
		Runtime:     cr,
		Ports:       node.NewPortAllocator(30000, 32767),
		ctx:         ctx,
		cancel:      cancel,
		probers:     make(map[string]context.CancelFunc),
		activeSpecs: make(map[string]application.AppSpec),
		activePorts: make(map[string]int),
	}
}

func (e *InstanceExecutor) Start() {
	slog.Info("InstanceExecutor started")
	e.syncPorts()
	go e.loop()
}

func (e *InstanceExecutor) Stop() {
	e.cancel()
}

func (e *InstanceExecutor) loop() {
	ticker := time.NewTicker(100 * time.Millisecond) // fast for tests
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.pollAndClaim()
			e.monitorActive()
		}
	}
}

func (e *InstanceExecutor) pollAndClaim() {
	instances, err := e.APIClient.ListInstances(e.NodeID, "")
	if err != nil {
		return
	}

	for _, inst := range instances {
		if inst.Status != instance.StatusAssigned && (inst.Status != instance.StatusUnknown || inst.ExecutionID != "") {
			continue
		}
		claimResp, err := e.APIClient.ClaimInstance(inst.ID, e.NodeID)
		if err != nil {
			continue
		}

		spec := runtime.ContainerSpec{
			InstanceID:    claimResp.Instance.ID,
			ExecutionID:   claimResp.ExecutionID,
			ApplicationID: claimResp.Instance.ApplicationID,
			DeploymentID:  claimResp.Instance.DeploymentID,
			Image:         claimResp.Spec.Image,
			Command:       claimResp.Spec.Command,
			Args:          claimResp.Spec.Args,
			Environment:   claimResp.Spec.Environment,
		}
		if claimResp.Spec.Resources != nil {
			spec.CPU = claimResp.Spec.Resources.CPU
			spec.MemoryMB = claimResp.Spec.Resources.MemoryMB
		}

		for _, p := range claimResp.Spec.Ports {
			hostPort := p.HostPort
			if hostPort == 0 {
				allocated, err := e.Ports.Allocate(claimResp.Instance.ID)
				if err == nil {
					hostPort = allocated
				}
			}
			spec.Ports = append(spec.Ports, runtime.PortMapping{
				Internal: p.ContainerPort,
				External: hostPort,
			})
		}

		info, err := e.Runtime.Start(e.ctx, spec)
		if err != nil {
			_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, claimResp.ExecutionID, instance.StatusCrashed, instance.HealthUnknown, "", nil)
			e.Ports.Release(inst.ID)
			e.stopProbers(inst.ID)
			continue
		}

		status := instance.StatusRunning
		if info.State == runtime.StateExited {
			status = instance.StatusCrashed
		}

		var instPorts []instance.PortMapping
		var mainHostPort int
		for _, p := range spec.Ports {
			if mainHostPort == 0 {
				mainHostPort = p.External
			}
			instPorts = append(instPorts, instance.PortMapping{
				Internal: p.Internal,
				External: p.External,
			})
		}

		e.proberMu.Lock()
		e.activeSpecs[inst.ID] = claimResp.Spec
		if mainHostPort > 0 {
			e.activePorts[inst.ID] = mainHostPort
		}
		e.proberMu.Unlock()

		_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, claimResp.ExecutionID, status, instance.HealthUnknown, info.ContainerID, instPorts)

		if status == instance.StatusRunning {
			inst.ExecutionID = claimResp.ExecutionID
			e.startProbers(inst, mainHostPort, claimResp.Spec)
		}
	}
}

func (e *InstanceExecutor) monitorActive() {
	instances, err := e.APIClient.ListInstances(e.NodeID, "")
	if err != nil {
		return
	}

	for _, inst := range instances {
		if inst.ExecutionID == "" {
			continue
		}

		containerName := "runstack-" + inst.ID
		if inst.ContainerID != "" {
			containerName = inst.ContainerID
		}

		state, err := e.Runtime.Status(e.ctx, containerName)

		if inst.Status == instance.StatusStopped {
			if err == nil {
				// Zombie container!
				_ = e.Runtime.Stop(e.ctx, containerName)
				_ = e.Runtime.Remove(e.ctx, containerName)
			}
			e.Ports.Release(inst.ID)
			e.stopProbers(inst.ID)
			continue
		}
		if inst.Status == instance.StatusCrashed {
			if err == nil && state == runtime.StateRunning {
				// CP thinks it's crashed (e.g. timeout), but it's still running. Fencing requires we kill it.
				_ = e.Runtime.Stop(e.ctx, containerName)
			}
			e.Ports.Release(inst.ID)
			e.stopProbers(inst.ID)
			// Leave the container around for manual docker logs debugging in V1
			continue
		}

		if inst.Status != instance.StatusStarting && inst.Status != instance.StatusRunning && inst.Status != instance.StatusStopping && inst.Status != instance.StatusUnknown {
			continue
		}

		if err != nil {
			if errors.Is(err, runtime.ErrContainerNotFound) {
				if inst.Status == instance.StatusStopping {
					_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusStopped, instance.HealthUnknown, containerName, nil)
				} else {
					_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusCrashed, instance.HealthUnknown, containerName, nil)
				}
			} else if errors.Is(err, runtime.ErrContainerConflict) {
				// stale execution
			}
			continue
		}

		var targetStatus instance.InstanceStatus
		var targetHealth instance.InstanceHealth
		switch state {
		case runtime.StateRunning:
			if inst.Status == instance.StatusStopping {
				_ = e.Runtime.Stop(e.ctx, containerName)
				_ = e.Runtime.Remove(e.ctx, containerName)
				targetStatus = instance.StatusStopped
				targetHealth = instance.HealthUnknown
				e.stopProbers(inst.ID)
			} else if inst.Status == instance.StatusStarting || inst.Status == instance.StatusUnknown {
				targetStatus = instance.StatusRunning
				targetHealth = inst.Health
				e.proberMu.Lock()
				spec, ok1 := e.activeSpecs[inst.ID]
				port, ok2 := e.activePorts[inst.ID]
				e.proberMu.Unlock()
				if ok1 && ok2 {
					e.startProbers(inst, port, spec)
				} else if ok1 {
					e.startProbers(inst, 0, spec)
				}
			}
		case runtime.StateExited:
			if inst.Status == instance.StatusStopping {
				_ = e.Runtime.Remove(e.ctx, containerName)
				targetStatus = instance.StatusStopped
			} else {
				targetStatus = instance.StatusCrashed
				e.stopProbers(inst.ID)
			}
		}

		if targetStatus != "" && targetStatus != inst.Status {
			_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, targetStatus, targetHealth, containerName, nil)
		}
	}
}
