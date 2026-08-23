# Milestone 8D: Deployment Rollout Management

## Context
Currently (Step 8C), when a new Deployment is created and set as the active deployment for an Application, the Reconciler immediately stops all instances belonging to the old deployment and provisions new ones. This causes a hard loss of available capacity during the cutover window.

Step 8D introduces **Zero-Capacity-Loss Rolling Updates** and **Rollbacks**. (Note: "Zero-Downtime" is not guaranteed at the PaaS tier until traffic routing/load balancing is explicitly modeled in a future milestone. This milestone guarantees continuous underlying instance capacity).

## Architecture Boundaries

To prevent the `InstanceReconciler` from becoming a monolithic mega-controller, Step 8D introduces a **Deployment Rollout Controller**.

The boundary is strict:
1. **Deployment Rollout Controller**: Responsible for transitioning an Application from `Deployment v1` to `Deployment v2`. It manages `MaxSurge`, `MaxUnavailable`, and determines the exact `desired Instance set` for both the old and new versions at any given tick.
2. **Instance Reconciler**: (Existing) Reads the `desired Instance set` mapped by the Rollout Controller, and guarantees the physical instances match that state.

## Core Concepts

### 1. Rollout Strategies
Applications will support a configurable rollout strategy:
- `Immediate`: Current behavior. Old version is scaled to 0 immediately, new version scales to desired.
- `RollingUpdate`: Incrementally start new instances and shut down old instances, gated by health checks.

### 2. RollingUpdate Semantics (MaxSurge & MaxUnavailable)
- `MaxSurge`: The maximum number of instances that can be created over the desired replica count during an update (e.g., if desired=3 and surge=1, up to 4 total instances can run across all versions).
- `MaxUnavailable`: The maximum number of instances that can be unavailable during the update (e.g., if desired=3 and unavailable=1, we must have at least 2 viable/healthy instances serving).

**Validation Rule (Deadlock Prevention)**:
`MaxSurge` and `MaxUnavailable` cannot both be `0` if `Replicas >= 1`.
If `Replicas = 1`, and `MaxSurge = 0` / `MaxUnavailable = 0`, the rollout would deadlock (cannot create v2 because Surge is 0, cannot remove v1 because Unavailable is 0).
The API will enforce `MaxSurge > 0 || MaxUnavailable > 0`. If left empty, default to `MaxSurge = 25% (min 1)` and `MaxUnavailable = 25% (min 1)`.

### 3. Rollout Lifecycle & Progress Metrics
A rollout is represented by an explicit state machine tracked on the `Deployment`:
- `PENDING`: Initial state when created.
- `PROGRESSING`: Rollout is actively moving capacity to this deployment.
- `PAUSED`: Operator explicitly halted the rollout, or it was paused by an error.
- `FAILED`: The new deployment failed to roll out (e.g., hit the 8C CrashLoop threshold).
- `COMPLETED`: The rollout finished successfully. All desired instances are healthy.
- `ROLLED_BACK`: This deployment was superseded or rolled back from.

**Progress Metrics tracked on Deployment**:
- `DesiredReplicas`: The target total.
- `UpdatedReplicas`: Instances of the *new* version that have been created.
- `ReadyReplicas`: Instances of the *new* version that are `RUNNING` + `HEALTHY`.
- `UnavailableReplicas`: Total desired capacity minus total healthy capacity (across all versions).
- `BlockedReason`: Human-readable reason if the rollout is `FAILED` or `PAUSED`.

### 4. Idempotent Rollout Reconciliation
The Rollout Controller is strictly idempotent. Given `Deployment v1` (old) and `Deployment v2` (new):
1. **Calculate Available Capacity**: Sum of `HEALTHY` instances across v1 and v2.
2. **Calculate Surge**: Allow creating v2 instances up to `Desired + MaxSurge` total across both versions.
3. **Calculate Obsolescence**: If total available capacity allows, scale down v1 instances without dipping below `Desired - MaxUnavailable`.
4. **Emit Targets**: Output `Desired Replicas for v1` and `Desired Replicas for v2`.

The `InstanceReconciler` then blindly converges actual instances to these two integer targets.

### 5. Failed Rollout Behavior
If `Deployment v2` begins a crash loop and triggers the 8C `DEGRADED` circuit breaker:
- `Deployment v2` transitions to Rollout `FAILED`.
- `Deployment v1` remains active and continues to hold whatever instances it currently has.
- **No automatic rollback**. The controller stops acting, preventing an oscillating loop (`v1 → v2 → v1 → v2`). Operator intervention is required.

### 6. Rollback Semantics
Deployments are completely immutable. Rollback behaves as follows:
- **Operation**: The operator calls `POST /api/v1/applications/{id}/rollback?target=v1`.
- **Identity**: The Application's `ActiveDeploymentID` is pointed back to the exact, immutable `v1` DeploymentID. `v1` retains its original `Version`, `SpecSnapshot`, and `CreatedAt`.
- **Eligibility**: You cannot roll back to a deployment that was previously marked `FAILED` or `DEGRADED` without an explicit force flag.
- **Execution**: The Rollout Controller sees `v1` is the active target, and `v2` is the previous. It uses the exact same `RollingUpdate` idempotency algorithm to scale `v1` back up to `Desired` while safely scaling `v2` down to `0`.
- **Partially-Created v2 Instances**: Any v2 instances that were created before the rollback are naturally treated as obsolete capacity by the `InstanceReconciler` and gracefully stopped.

## Out of Scope
- Traffic shaping, Ingress, or Load Balancing (Blue/Green, Canary).
- Automatic Rollbacks on failure.
