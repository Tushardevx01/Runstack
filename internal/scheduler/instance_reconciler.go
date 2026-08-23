package scheduler

import (
	"log/slog"
	"sort"
	"sync"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
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

	apps := r.appReg.List()

	for _, app := range apps {
		if app.ActiveDeploymentID == "" {
			continue
		}

		// 2. Fetch the active deployment
		dep, err := r.depReg.Get(app.ActiveDeploymentID)
		if err != nil {
			slog.Error("Reconciler: failed to get active deployment", "app_id", app.ID, "dep_id", app.ActiveDeploymentID, "error", err)
			continue
		}

		desiredCount := dep.SpecSnapshot.Replicas
		allInstances := r.instReg.List()

		// 3. Stop instances from SUPERSEDED deployments (Immediate Cutover for V1)
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

		// 4. Count viable instances for the ACTIVE deployment
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
			slog.Info("Reconciler: scaling up", "app_id", app.ID, "missing", toCreate)
			for i := 0; i < toCreate; i++ {
				_, err := r.instReg.Create(app.ID, dep.ID)
				if err != nil {
					slog.Error("Reconciler: failed to create instance", "error", err)
				}
			}
		}

		// 6. Scale DOWN
		if actualCount > desiredCount {
			toStop := actualCount - desiredCount
			slog.Info("Reconciler: scaling down", "app_id", app.ID, "excess", toStop)

			// Deterministically sort to ensure we don't randomly kill different instances on retry if failed
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

func (r *InstanceReconciler) recoverOrphanedInstances() {
	nodes := r.nodeReg.List()
	onlineNodes := make(map[string]bool)
	for _, n := range nodes {
		if n.Status == node.StatusOnline {
			onlineNodes[n.ID] = true
		}
	}

	allInstances := r.instReg.List()
	for _, inst := range allInstances {
		if inst.NodeID != "" && !onlineNodes[inst.NodeID] {
			if r.isViable(inst.Status) {
				slog.Info("Reconciler: instance orphaned, marking crashed", "instance_id", inst.ID, "node_id", inst.NodeID)
				// Transition to CRASHED, keeping NodeID history
				r.instReg.UpdateState(inst.ID, instance.StatusCrashed, inst.NodeID, inst.ContainerID)
			}
		}
	}
}

func (r *InstanceReconciler) isViable(status instance.InstanceStatus) bool {
	switch status {
	case instance.StatusPending, instance.StatusAssigned, instance.StatusStarting, instance.StatusRunning:
		return true
	case instance.StatusCrashed, instance.StatusStopped:
		return false
	default:
		return false
	}
}
