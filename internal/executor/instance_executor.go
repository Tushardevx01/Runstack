package executor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/runtime"
)

// InstanceExecutor manages long-running Instances on the Agent.
type InstanceExecutor struct {
	NodeID    string
	APIClient *api.Client
	Runtime   runtime.ContainerRuntime
	Ports     *node.PortAllocator
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewInstanceExecutor(nodeID string, apiClient *api.Client, cr runtime.ContainerRuntime) *InstanceExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	return &InstanceExecutor{
		NodeID:    nodeID,
		APIClient: apiClient,
		Runtime:   cr,
		Ports:     node.NewPortAllocator(30000, 32767),
		ctx:       ctx,
		cancel:    cancel,
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
	instances, err := e.APIClient.ListInstances(e.NodeID, string(instance.StatusAssigned))
	if err != nil {
		return
	}

	for _, inst := range instances {
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
			continue
		}

		status := instance.StatusRunning
		if info.State == runtime.StateExited {
			status = instance.StatusCrashed
		}

		var instPorts []instance.PortMapping
		for _, p := range spec.Ports {
			instPorts = append(instPorts, instance.PortMapping{
				Internal: p.Internal,
				External: p.External,
			})
		}

		_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, claimResp.ExecutionID, status, instance.HealthUnknown, info.ContainerID, instPorts)
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

		if inst.Status == instance.StatusStopped || inst.Status == instance.StatusUnknown {
			if err == nil {
				// Zombie container!
				_ = e.Runtime.Stop(e.ctx, containerName)
				_ = e.Runtime.Remove(e.ctx, containerName)
			}
			e.Ports.Release(inst.ID)
			continue
		}
		if inst.Status == instance.StatusCrashed {
			if err == nil && state == runtime.StateRunning {
				// CP thinks it's crashed (e.g. timeout), but it's still running. Fencing requires we kill it.
				_ = e.Runtime.Stop(e.ctx, containerName)
			}
			e.Ports.Release(inst.ID)
			// Leave the container around for manual docker logs debugging in V1
			continue
		}

		if inst.Status != instance.StatusStarting && inst.Status != instance.StatusRunning && inst.Status != instance.StatusStopping {
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
			} else if inst.Status == instance.StatusStarting {
				targetStatus = instance.StatusRunning
				targetHealth = instance.HealthHealthy
			}
		case runtime.StateExited:
			if inst.Status == instance.StatusStopping {
				_ = e.Runtime.Remove(e.ctx, containerName)
				targetStatus = instance.StatusStopped
			} else {
				targetStatus = instance.StatusCrashed
			}
		}

		if targetStatus != "" && targetStatus != inst.Status {
			_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, targetStatus, targetHealth, containerName, nil)
		}
	}
}
