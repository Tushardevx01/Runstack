package scheduler

import (
	"sort"
	"sync"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
)

type DeploymentTarget struct {
	DeploymentID string
	DesiredCount int
}

type AppTarget struct {
	AppID              string
	ActiveDeploymentID string
	Targets            []DeploymentTarget
}

type RolloutController struct {
	appReg  *application.Registry
	depReg  *deployment.Registry
	instReg *instance.Registry
	mu      sync.Mutex
}

func NewRolloutController(appReg *application.Registry, depReg *deployment.Registry, instReg *instance.Registry) *RolloutController {
	return &RolloutController{
		appReg:  appReg,
		depReg:  depReg,
		instReg: instReg,
	}
}

// ComputeTargets calculates the desired instances for all deployments for each application.
func (c *RolloutController) ComputeTargets() []AppTarget {
	c.mu.Lock()
	defer c.mu.Unlock()

	apps := c.appReg.List()
	allInstances := c.instReg.List()
	allDeployments := c.depReg.List() // Using List instead of fetching one by one

	var results []AppTarget

	for _, app := range apps {
		if app.ActiveDeploymentID == "" {
			continue
		}

		target := c.computeAppTarget(app, allInstances, allDeployments)
		if target != nil {
			results = append(results, *target)
		}
	}

	return results
}

func (c *RolloutController) computeAppTarget(app application.Application, allInstances []instance.Instance, allDeployments []deployment.Deployment) *AppTarget {
	var activeDep deployment.Deployment
	var activeFound bool

	// Collect all deployments for this app
	var appDeps []deployment.Deployment
	for _, d := range allDeployments {
		if d.ApplicationID == app.ID {
			appDeps = append(appDeps, d)
			if d.ID == app.ActiveDeploymentID {
				activeDep = d
				activeFound = true
			}
		}
	}

	if !activeFound {
		return nil
	}

	// Filter instances for this app
	var appInsts []instance.Instance
	for _, inst := range allInstances {
		if inst.ApplicationID == app.ID {
			appInsts = append(appInsts, inst)
		}
	}

	// Immediate strategy
	if activeDep.SpecSnapshot.Strategy == nil || activeDep.SpecSnapshot.Strategy.Type == application.RolloutStrategyImmediate {
		return c.computeImmediateTarget(app, activeDep, appDeps)
	}

	// RollingUpdate strategy
	return c.computeRollingTarget(app, activeDep, appDeps, appInsts)
}

func (c *RolloutController) computeImmediateTarget(app application.Application, activeDep deployment.Deployment, appDeps []deployment.Deployment) *AppTarget {
	target := AppTarget{
		AppID:              app.ID,
		ActiveDeploymentID: activeDep.ID,
	}

	for _, dep := range appDeps {
		if dep.ID == activeDep.ID {
			desiredCount := dep.SpecSnapshot.Replicas
			if activeDep.Degraded {
				// Count viable
				viable := 0
				for _, i := range c.instReg.List() {
					if i.DeploymentID == dep.ID && c.isViable(i.Status) {
						viable++
					}
				}
				desiredCount = viable
				if dep.RolloutStatus != deployment.RolloutFailed {
					c.depReg.UpdateRolloutStatusOnly(dep.ID, deployment.RolloutFailed, "Deployment degraded (crash loop)")
				}
			} else if dep.RolloutStatus != deployment.RolloutCompleted {
				c.depReg.UpdateRolloutStatusOnly(dep.ID, deployment.RolloutCompleted, "")
			}
			target.Targets = append(target.Targets, DeploymentTarget{
				DeploymentID: dep.ID,
				DesiredCount: desiredCount,
			})
		} else {
			target.Targets = append(target.Targets, DeploymentTarget{
				DeploymentID: dep.ID,
				DesiredCount: 0,
			})
			if dep.RolloutStatus != deployment.RolloutRolledBack && dep.RolloutStatus != deployment.RolloutCompleted && dep.Status != deployment.StatusSuperseded {
				// If old deployment was progressing, it's now superseded
				c.depReg.UpdateRolloutStatusOnly(dep.ID, deployment.RolloutRolledBack, "Superseded by immediate rollout")
			}
		}
	}

	return &target
}

func (c *RolloutController) isViable(status instance.InstanceStatus) bool {
	switch status {
	case instance.StatusPending, instance.StatusAssigned, instance.StatusStarting, instance.StatusRunning, instance.StatusUnknown:
		return true
	default:
		return false
	}
}

func (c *RolloutController) computeRollingTarget(app application.Application, activeDep deployment.Deployment, appDeps []deployment.Deployment, appInsts []instance.Instance) *AppTarget {
	desired := activeDep.SpecSnapshot.Replicas
	surge := activeDep.SpecSnapshot.Strategy.MaxSurge
	unavailable := activeDep.SpecSnapshot.Strategy.MaxUnavailable

	// Track counts
	var activeViable, activeReady int
	var oldViable, oldReady int

	// Count instances
	for _, inst := range appInsts {
		viable := c.isViable(inst.Status)
		ready := (inst.Status == instance.StatusRunning && inst.Health == instance.HealthHealthy)

		if inst.DeploymentID == activeDep.ID {
			if viable {
				activeViable++
			}
			if ready {
				activeReady++
			}
		} else {
			if viable {
				oldViable++
			}
			if ready {
				oldReady++
			}
		}
	}

	totalViable := activeViable + oldViable
	totalReady := activeReady + oldReady

	// Metrics update
	c.depReg.UpdateRolloutStatus(activeDep.ID, activeDep.RolloutStatus, desired, activeViable, activeReady, desired-totalReady, activeDep.BlockedReason)

	// If degraded, stop progressing
	if activeDep.Degraded {
		if activeDep.RolloutStatus != deployment.RolloutFailed {
			c.depReg.UpdateRolloutStatusOnly(activeDep.ID, deployment.RolloutFailed, "Deployment degraded (crash loop)")
		}

		// Target is to keep whatever we have, or maybe scale active to 0?
		// "preserve surviving v1 capacity" -> v1 desired is whatever it is, v2 is whatever it is.
		target := AppTarget{
			AppID:              app.ID,
			ActiveDeploymentID: activeDep.ID,
		}

		for _, dep := range appDeps {
			if dep.ID == activeDep.ID {
				target.Targets = append(target.Targets, DeploymentTarget{DeploymentID: dep.ID, DesiredCount: activeViable})
			} else {
				// Old deployments keep their viable capacity
				// Let's count per-old-deployment viable
				depViable := 0
				for _, i := range appInsts {
					if i.DeploymentID == dep.ID && c.isViable(i.Status) {
						depViable++
					}
				}
				target.Targets = append(target.Targets, DeploymentTarget{DeploymentID: dep.ID, DesiredCount: depViable})
			}
		}

		return &target
	}

	// Active deployment target calculations (scale up)
	// Max allowed total instances across all versions
	maxAllowedTotal := desired + surge

	// Min required ready instances
	minAvailable := desired - unavailable
	if minAvailable < 0 {
		minAvailable = 0
	}

	// 1. Scale down old deployments if we exceed maxAllowedTotal
	oldTargetTotal := oldViable
	if totalViable > maxAllowedTotal {
		excess := totalViable - maxAllowedTotal
		if excess > oldTargetTotal {
			excess = oldTargetTotal
		}
		oldTargetTotal -= excess
	}

	// 2. Scale down old deployments if they are no longer needed
	// We only need old deployments to satisfy minAvailable.
	// How many old instances do we NEED to keep?
	// We need: minAvailable - activeReady. (But we shouldn't ask for more than oldViable).
	requiredOld := minAvailable - activeReady
	if requiredOld < 0 {
		requiredOld = 0
	}

	// Maximum old instances we SHOULD have:
	// If active is fully scaled up (activeViable == desired), we want 0 old instances (subject to minAvailable).
	// If active is still scaling up, we want to scale down old instances only to make room for new active instances (to stay under maxAllowedTotal), OR if we just have excess old instances.
	// To make room for active up to `desired`, we should cap old instances at `maxAllowedTotal - desired`.
	// But if `maxAllowedTotal - desired` is less than `requiredOld`, we cap at `requiredOld`.
	maxOld := maxAllowedTotal - desired

	// Wait, if activeViable > desired? We already handled newActiveTarget above.
	// If active is struggling to scale up, capping old at `maxAllowedTotal - desired` means we immediately drop old down to that cap.
	// That's exactly what we want! (Subject to requiredOld for safety).
	if maxOld < requiredOld {
		maxOld = requiredOld // safety bound
	}

	if oldTargetTotal > maxOld {
		oldTargetTotal = maxOld
	}

	// 3. Scale up active deployment
	// We can scale up active up to `desired`.
	// But we cannot exceed `maxAllowedTotal - oldTargetTotal` (to respect surge after accounting for old instances we decided to keep).
	newActiveTarget := desired
	allowedActive := maxAllowedTotal - oldTargetTotal
	if newActiveTarget > allowedActive {
		newActiveTarget = allowedActive
	}
	if newActiveTarget < activeViable {
		newActiveTarget = activeViable // Don't scale down active deployment during rollout!
	}

	// Assign old targets
	target := AppTarget{
		AppID:              app.ID,
		ActiveDeploymentID: activeDep.ID,
	}

	target.Targets = append(target.Targets, DeploymentTarget{
		DeploymentID: activeDep.ID,
		DesiredCount: newActiveTarget,
	})

	// Distribute oldTargetTotal among old deployments (arbitrarily, just sorting by ID for determinism)
	var oldDeps []deployment.Deployment
	for _, dep := range appDeps {
		if dep.ID != activeDep.ID {
			oldDeps = append(oldDeps, dep)
		}
	}
	sort.Slice(oldDeps, func(i, j int) bool {
		return oldDeps[i].ID < oldDeps[j].ID
	})

	remainingOld := oldTargetTotal
	for _, dep := range oldDeps {
		depViable := 0
		for _, i := range appInsts {
			if i.DeploymentID == dep.ID && c.isViable(i.Status) {
				depViable++
			}
		}

		assign := depViable
		if assign > remainingOld {
			assign = remainingOld
		}
		remainingOld -= assign

		target.Targets = append(target.Targets, DeploymentTarget{
			DeploymentID: dep.ID,
			DesiredCount: assign,
		})

		// Status update
		if assign == 0 && depViable == 0 && dep.RolloutStatus != deployment.RolloutRolledBack && dep.Status != deployment.StatusSuperseded {
			c.depReg.UpdateRolloutStatusOnly(dep.ID, deployment.RolloutRolledBack, "Scaled to 0 by rollout")
		}
	}

	// Update active rollout status
	if activeDep.RolloutStatus == deployment.RolloutPending {
		c.depReg.UpdateRolloutStatusOnly(activeDep.ID, deployment.RolloutProgressing, "")
	} else if activeReady == desired && oldViable == 0 {
		c.depReg.UpdateRolloutStatusOnly(activeDep.ID, deployment.RolloutCompleted, "")
	} else if newActiveTarget == activeViable && activeViable < desired && oldTargetTotal == requiredOld {
		// Rollout is paused/waiting
		// Not necessarily paused, just progressing slowly due to health checks.
	}

	return &target
}
