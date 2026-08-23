package scheduler

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
)

const (
	InstanceUnknownTimeout = 5 * time.Minute // using realistic value, tests can override if needed, or we just use a variable
	HealthyRecoveryWindow  = 60 * time.Second
	MaxCrashLoopThreshold  = 5
)

// Allow tests to override
var (
	ConfigInstanceUnknownTimeout = InstanceUnknownTimeout
	ConfigHealthyRecoveryWindow  = HealthyRecoveryWindow
)

type InstanceReconciler struct {
	appReg  *application.Registry
	depReg  *deployment.Registry
	instReg *instance.Registry
	nodeReg *node.Registry
	mu      sync.Mutex
}

func NewInstanceReconciler(
	appReg *application.Registry,
	depReg *deployment.Registry,
	instReg *instance.Registry,
	nodeReg *node.Registry,
) *InstanceReconciler {
	return &InstanceReconciler{
		appReg:  appReg,
		depReg:  depReg,
		instReg: instReg,
		nodeReg: nodeReg,
	}
}

func (r *InstanceReconciler) Reconcile() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. Recover stale instances (orphaned by offline nodes)
	r.recoverOrphanedInstances()

	// 2. Evaluate Health and Crash Loop resettings
	r.evaluateHealthAndCrashes()

	apps := r.appReg.List()

	for _, app := range apps {
		if app.ActiveDeploymentID == "" {
			continue
		}

		dep, err := r.depReg.Get(app.ActiveDeploymentID)
		if err != nil {
			slog.Error("Reconciler: failed to get active deployment", "app_id", app.ID, "dep_id", app.ActiveDeploymentID, "error", err)
			continue
		}

		desiredCount := dep.SpecSnapshot.Replicas
		allInstances := r.instReg.List()

		for _, inst := range allInstances {
			if inst.ApplicationID != app.ID {
				continue
			}
			if inst.DeploymentID != dep.ID {
				if r.isViable(inst.Status) {
					slog.Info("Reconciler: stopping obsolete instance", "instance_id", inst.ID, "deployment_id", inst.DeploymentID)
					r.instReg.UpdateState(inst.ID, instance.StatusStopped, inst.NodeID, inst.ContainerID)
				}
			}
		}

		var viableInstances []instance.Instance
		for _, inst := range allInstances {
			if inst.ApplicationID == app.ID && inst.DeploymentID == dep.ID {
				if r.isViable(inst.Status) {
					viableInstances = append(viableInstances, inst)
				}
			}
		}

		actualCount := len(viableInstances)

		// 5. Scale UP
		if actualCount < desiredCount {
			toCreate := desiredCount - actualCount

			if dep.Degraded {
				slog.Warn("Reconciler: skipping replacement for degraded deployment", "app_id", app.ID, "dep_id", dep.ID)
			} else {
				slog.Info("Reconciler: scaling up", "app_id", app.ID, "missing", toCreate)
				for i := 0; i < toCreate; i++ {
					_, err := r.instReg.Create(app.ID, dep.ID)
					if err != nil {
						slog.Error("Reconciler: failed to create instance", "error", err)
					}
				}
			}
		}

		// 6. Scale DOWN
		if actualCount > desiredCount {
			toStop := actualCount - desiredCount
			slog.Info("Reconciler: scaling down", "app_id", app.ID, "excess", toStop)

			sort.Slice(viableInstances, func(i, j int) bool {
				return viableInstances[i].ID > viableInstances[j].ID
			})

			for i := 0; i < toStop; i++ {
				inst := viableInstances[i]
				r.instReg.UpdateState(inst.ID, instance.StatusStopped, inst.NodeID, inst.ContainerID)
			}
		}
	}

	return nil
}

func (r *InstanceReconciler) evaluateHealthAndCrashes() {
	allInstances := r.instReg.List()
	now := time.Now().UTC()

	for _, inst := range allInstances {
		if inst.Status == instance.StatusRunning {
			if inst.Health == instance.HealthUnhealthy {
				slog.Info("Reconciler: instance is unhealthy, terminating", "instance_id", inst.ID)
				r.instReg.UpdateState(inst.ID, instance.StatusCrashed, inst.NodeID, inst.ContainerID)
				r.depReg.RecordCrash(inst.DeploymentID, deployment.MaxCrashLoopThreshold)
			} else if inst.Health == instance.HealthHealthy && inst.StartedAt != nil {
				if now.Sub(*inst.StartedAt) >= ConfigHealthyRecoveryWindow {
					r.depReg.ResetCrashCounter(inst.DeploymentID)
				}
			}
		} else if inst.Status == instance.StatusCrashed {
			// If it crashed rapidly (not a health termination, but a runtime crash)
			// Wait, the agent might have just marked it CRASHED. We need to increment the crash counter.
			// How do we know if we already incremented?
			// We can track if it was handled, or just increment whenever a RUNNING instance goes to CRASHED.
			// But Reconciler polls. It's better if the InstanceRegistry or Agent handles the crash counter.
			// The design says: "ConsecutiveCrashes is tracked strictly on the Deployment version".
			// Let's modify UpdateState in Registry to do it? Or have Reconciler do it?
		}
	}
}

func (r *InstanceReconciler) recoverOrphanedInstances() {
	nodes := r.nodeReg.List()
	onlineNodes := make(map[string]bool)
	for _, n := range nodes {
		if n.Status == node.StatusOnline {
			onlineNodes[n.ID] = true
		}
	}

	now := time.Now().UTC()
	allInstances := r.instReg.List()
	for _, inst := range allInstances {
		if inst.NodeID != "" && !onlineNodes[inst.NodeID] {
			if r.isViable(inst.Status) && inst.Status != instance.StatusUnknown {
				slog.Info("Reconciler: instance node offline, marking UNKNOWN", "instance_id", inst.ID, "node_id", inst.NodeID)
				r.instReg.MarkUnknown(inst.ID)
			}
		}

		if inst.Status == instance.StatusUnknown && inst.UnknownSince != nil {
			if now.Sub(*inst.UnknownSince) >= ConfigInstanceUnknownTimeout {
				slog.Info("Reconciler: UNKNOWN instance timed out, marking CRASHED", "instance_id", inst.ID)
				r.instReg.UpdateState(inst.ID, instance.StatusCrashed, inst.NodeID, inst.ContainerID)
				// Do NOT increment crash counter for Node Loss
			}
		}
	}
}

func (r *InstanceReconciler) isViable(status instance.InstanceStatus) bool {
	switch status {
	case instance.StatusPending, instance.StatusAssigned, instance.StatusStarting, instance.StatusRunning, instance.StatusUnknown:
		return true
	case instance.StatusCrashed, instance.StatusStopped:
		return false
	default:
		return false
	}
}
