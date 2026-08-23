# Step 8C Design Document: Instance Health, Reconciliation & Automatic Recovery

## 1. Problem Statement
RunStack must automatically maintain the desired state of long-running applications. The core challenge is robustly handling failure (crashes, unresponsiveness) and automatically replacing instances to meet the desired replica count, while strictly avoiding endless, uncontrolled restart loops ("CrashLooping").

## 2. Conceptual Separation
RunStack explicitly separates four concerns:
*   **Status**: Runtime process lifecycle (`STARTING`, `RUNNING`, `STOPPED`, `CRASHED`, `UNKNOWN`).
*   **Health**: Application readiness (`HEALTHY`, `UNHEALTHY`, `UNKNOWN`).
*   **Node State**: Infrastructure availability (`ONLINE`, `OFFLINE`).
*   **Desired State**: What the `Deployment` wants (`Replicas: 3`).

A completely valid state matrix might be: `Deployment desired: RUNNING | Node: ONLINE | Instance: RUNNING | Health: UNHEALTHY`.

## 3. Core Principles
1.  **Immutability over Mutation**: A `CRASHED` instance is terminal. We do not restart failed instances; the Reconciler provisions brand new `PENDING` replacement instances.
2.  **Circuit Breaking**: Infinite restart loops will destroy a cluster. We must employ crash loop limits at the Deployment layer.

## 4. Key Design Decisions

### Source of Truth for Health
The Agent's `InstanceExecutor` evaluates application health locally (e.g., Docker native health checks, simple HTTP/TCP probes). The Agent reports both `Status` and `Health` to the Control Plane via `POST /api/v1/instances/{id}/status`.

### Unknown State & Node Loss
A Node going `OFFLINE` does not mean the application crashed; it means the Control Plane lost observation.
1.  If a node drops offline (or the runtime goes unresponsive), the instance becomes `UNKNOWN`.
2.  The Control Plane records `UnknownSince`.
3.  If `time.Now() > UnknownSince + InstanceUnknownTimeout` (e.g., 5 minutes), the Control Plane concludes the instance is lost and transitions it to `CRASHED` to trigger a replacement.
4.  `UNKNOWN` and `UnknownSince` are fully exposed via the API and CLI to distinguish between infrastructure partition and application failure.

### Crash-Loop Breaker Semantics
Instances use a **Continuous Recovery Model with Circuit Breaking**. To prevent infinite loops, `ConsecutiveCrashes` is tracked strictly on the **Deployment** version.

*   **Increment Rules**:
    *   **Process crash**: Container unexpectedly exits (`CRASHED`) within `HealthyRecoveryWindow` (e.g., 60 seconds). -> *Increments*
    *   **Runtime error**: Container fails to start or image is missing. -> *Increments*
    *   **Health failure**: Instance is `RUNNING` but remains `UNHEALTHY` past a threshold, causing the Reconciler to terminate it. -> *Increments*
    *   **Node loss**: Instance times out from `UNKNOWN` state due to infrastructure partition. -> *Does NOT increment*.
*   **Degraded State**: If `ConsecutiveCrashes` reaches the `MaxCrashLoopThreshold` (e.g., 5), the Deployment transitions to `DEGRADED`.
*   **While Degraded**: The Reconciler **pauses** all replacement provisioning for this specific deployment to protect cluster resources.
*   **Reset & Recovery**:
    *   If an instance achieves `Status: RUNNING` and `Health: HEALTHY` for the duration of the `HealthyRecoveryWindow`, the Deployment's `ConsecutiveCrashes` resets to 0.
    *   To exit a `DEGRADED` state, an operator must intervene (e.g., manual API override) or deploy a new `Deployment` version. A new deployment version starts with 0 crashes, preventing a broken `v1` from poisoning a healthy `v2`.

### Replaced versus Restarted
Instances are **replaced**, never restarted.
If `runstack-abc` crashes, it transitions to `CRASHED` (terminal). The Reconciler creates `runstack-def`.

### Old InstanceExecutor vs Replacement Instance
If an old Node returns from an `UNKNOWN` partition, its Agent will attempt to report status for the old instances, be rejected via `ExecutionID` fencing, and locally clean up the orphaned containers.

## 5. Architectural Boundaries

**InstanceExecutor (Agent):**
- Probes container health locally.
- Pushes `Health` and `Status` together.

**InstanceReconciler (CP):**
- Manages `UNKNOWN -> CRASHED` timeouts.
- Checks `Active Instances` vs `Desired Replicas`.
- Evaluates `ConsecutiveCrashes`.
- Provisions replacements if healthy, or marks Deployment `DEGRADED` if looping.

## 6. Definition of Done
*   Instances track `Health`, `Status`, and `UnknownSince`.
*   Reconciler replaces `CRASHED` instances automatically.
*   Deployments track `ConsecutiveCrashes` and enter `DEGRADED` state to break infinite crash loops based on precise increment rules.

READY FOR IMPLEMENTATION
