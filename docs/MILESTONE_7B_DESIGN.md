# Milestone 7B: Deployment Lifecycle & Instance Reconciliation

This document designs the control-plane reconciliation loop that ensures an Application's actual runtime state matches its desired Deployment configuration.

**Core Rule:** The Instance Reconciler must be strictly idempotent. Running the reconciliation loop once or 1,000 times against the same state must produce the exact same outcome without duplicating side effects.

---

## 1. Deployment Lifecycle States

To manage version rollouts, Deployments will use the following lifecycle semantics:

*   **`ACTIVE`**: The current, desired deployment version for an Application. There is exactly one `ACTIVE` deployment per Application.
*   **`SUPERSEDED`**: An older deployment version that has been replaced by a newer rollout. Its instances should be spun down.
*   **`FAILED`**: A deployment that failed to become healthy (e.g., its instances crash repeatedly). Used for future automated rollback thresholds.
*   **`ROLLED_BACK`**: A deployment that was explicitly deactivated in favor of reverting to a previous version.

When an `Application` is updated, the previous `ACTIVE` deployment is transitioned to `SUPERSEDED`, and a new `ACTIVE` deployment is created.

---

## 2. Defining "Desired" vs "Actual" Capacity

The reconciliation logic fundamentally compares two values:

*   **Desired Capacity**: The `Replicas` value specified in the `ACTIVE` Deployment's `SpecSnapshot`.
*   **Actual Viable Capacity**: The number of Instances belonging to the `ACTIVE` Deployment that are in a "viable" state.
    *   **Viable states**: `PENDING`, `ASSIGNED`, `STARTING`, `RUNNING`.
    *   **Non-viable (terminal) states**: `CRASHED`, `STOPPED`, `TERMINATED`.

**Crucially, a `CRASHED` or `STOPPED` instance does not count toward desired capacity.**

---

## 3. The Idempotent Reconciliation Loop

The Control Plane will introduce a background `InstanceReconciler` loop. It evaluates state independently of user HTTP requests.

### The Algorithm (Per Application):

1.  **Read Target**: Retrieve the Application and identify its `ActiveDeploymentID`.
2.  **Snapshot Actual State**: Retrieve all Instances for this Application across all its deployments.
3.  **Scale Down Old Versions (Immediate Cutover for V1)**:
    *   Identify any "viable" instances belonging to `SUPERSEDED` deployments.
    *   Update their state to `STOPPED` (or `TERMINATING`).
4.  **Evaluate Active Version**:
    *   Count the viable instances belonging to the `ACTIVE` deployment (`actualCount`).
    *   Compare to the desired replicas (`desiredCount`).
5.  **Reconcile Active Version**:
    *   If `actualCount < desiredCount`: Create exactly `(desiredCount - actualCount)` new Instances in `PENDING` state.
    *   If `actualCount > desiredCount`: Select exactly `(actualCount - desiredCount)` viable instances and transition them to `STOPPED`.
    *   If `actualCount == desiredCount`: Do nothing.

Because the loop explicitly counts current viable instances before acting, it is naturally idempotent. If it creates 2 instances and immediately runs again, `actualCount` will equal `desiredCount`, resulting in a no-op.

---

## 4. Node Failures & Stale Recovery

When a Node goes offline, its assigned Jobs are traditionally recovered back to `PENDING`. However, Instances are long-running and mapped to K8s-style Pod semantics.

*   **Immutability of Instances**: Once an Instance fails or its Node dies, that specific Instance ID is permanently dead.
*   **Recovery Flow**:
    1.  The NodeRegistry marks `node-A` as `OFFLINE`.
    2.  The Control Plane's stale recovery loop detects instances assigned to `node-A` that have exceeded the grace period.
    3.  It transitions those Instances to `CRASHED` (or `UNKNOWN`).
    4.  The next tick of the `InstanceReconciler` sees that `actualCount` has dropped below `desiredCount` (because `CRASHED` instances are non-viable).
    5.  The Reconciler generates **brand new** `PENDING` Instances to replace them.

This guarantees a clean audit trail where every Instance ID corresponds to exactly one physical execution attempt.

---

## 5. Preventing Concurrency & Duplicate Creation

*   The `InstanceReconciler` runs as a single, sequential background goroutine (similar to the Job Scheduler).
*   During its reconciliation tick for a specific Application, it evaluates the `InstanceRegistry` state.
*   Because registries use `sync.RWMutex` and the Reconciler sweeps sequentially, there are no race conditions where multiple threads attempt to scale up the same Application simultaneously.
*   Updates via HTTP (`PUT /apps/{id}`) do **not** directly spawn Instances. They only create the new `Deployment` and update the Application. The asynchronous `InstanceReconciler` detects the change on its next tick and orchestrates the instance creation. This entirely removes the risk of duplicated orchestration logic in the API handler.

---

## 6. Implementation Boundary (7B)

*   **Refactor API Handler/Service**: Remove the synchronous instance creation loop from `AppService.CreateApp` and `AppService.UpdateApp`. They should only mutate desired state (Application + Deployment).
*   **Implement `InstanceReconciler`**: Create the background loop that continuously aligns `InstanceRegistry` with `DeploymentRegistry`.
*   **Add Stale Node Recovery**: Implement logic to transition orphaned instances to `CRASHED`.
*   **Zero Container Logic**: No Docker/Podman dependencies will be introduced. The goal is entirely focused on mathematical control-plane reconciliation.
