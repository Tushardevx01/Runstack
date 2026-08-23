package executor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/runtime"
)

// InstanceExecutor manages long-running Instances on the Agent.
type InstanceExecutor struct {
	NodeID    string
	APIClient *api.Client
	Runtime   runtime.ContainerRuntime
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewInstanceExecutor(nodeID string, apiClient *api.Client, cr runtime.ContainerRuntime) *InstanceExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	return &InstanceExecutor{
		NodeID:    nodeID,
		APIClient: apiClient,
		Runtime:   cr,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (e *InstanceExecutor) Start() {
	slog.Info("InstanceExecutor started")
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
			spec.Ports = append(spec.Ports, runtime.PortMapping{
				Internal: p.ContainerPort,
				External: p.HostPort,
			})
		}

		info, err := e.Runtime.Start(e.ctx, spec)
		if err != nil {
			_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, claimResp.ExecutionID, instance.StatusCrashed, "")
			continue
		}

		status := instance.StatusRunning
		if info.State == runtime.StateExited {
			status = instance.StatusCrashed
		}

		_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, claimResp.ExecutionID, status, info.ContainerID)
	}
}

func (e *InstanceExecutor) monitorActive() {
	instances, err := e.APIClient.ListInstances(e.NodeID, "")
	if err != nil {
		return
	}

	for _, inst := range instances {
		if inst.Status != instance.StatusStarting && inst.Status != instance.StatusRunning && inst.Status != instance.StatusStopping {
			continue
		}
		if inst.ExecutionID == "" {
			continue
		}

		containerName := "runstack-" + inst.ID
		if inst.ContainerID != "" {
			containerName = inst.ContainerID
		}

		state, err := e.Runtime.Status(e.ctx, containerName)

		if err != nil {
			if errors.Is(err, runtime.ErrContainerNotFound) {
				if inst.Status == instance.StatusStopping {
					_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusStopped, containerName)
				} else {
					_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusCrashed, containerName)
				}
			} else if errors.Is(err, runtime.ErrContainerConflict) {
				// stale execution
			}
			continue
		}

		var targetStatus instance.InstanceStatus
		switch state {
		case runtime.StateRunning:
			if inst.Status == instance.StatusStopping {
				_ = e.Runtime.Stop(e.ctx, containerName)
				_ = e.Runtime.Remove(e.ctx, containerName)
				targetStatus = instance.StatusStopped
			} else if inst.Status == instance.StatusStarting {
				targetStatus = instance.StatusRunning
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
			_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, targetStatus, containerName)
		}
	}
}
