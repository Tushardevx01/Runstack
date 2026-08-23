# Milestone 8: Container Runtime & Instance Execution Design

This document details how the RunStack Execution Plane will transition Control Plane `Instances` into physical, running containers across distributed nodes, abstracting the underlying runtime (Docker or Podman).

---

## 1. Instance Execution Identity & Fencing

Similar to the Job execution model, `NodeID` alone is insufficient to guarantee execution ownership. Instances will use an **Instance Execution Identity** (`ExecutionID`).

*   When an Agent successfully calls `POST /api/v1/instances/{id}/claim`, the Control Plane generates a unique `ExecutionID` and transitions the Instance to `STARTING`.
*   All subsequent state reports from the Agent must include this `ExecutionID`.
*   If the Agent disconnects and the Control Plane reassigns the Instance to another Agent (or Reconciles a replacement), the old Agent's `ExecutionID` is invalidated.
*   **Split-brain prevention**: The Control Plane will reject any status updates containing a stale `ExecutionID`, ensuring out-of-sync Agents cannot incorrectly mutate state (e.g., resurrecting a `CRASHED` instance).

---

## 2. The Container Runtime Abstraction

To ensure RunStack is not tightly coupled to Docker, the Agent will interact with a `ContainerRuntime` interface.

### The Interface

```go
package runtime

type ContainerState string

const (
	StateRunning ContainerState = "running"
	StateExited  ContainerState = "exited"
	StateUnknown ContainerState = "unknown"
)

type ContainerSpec struct {
	InstanceID  string
	Image       string
	Command     []string
	Args        []string
	Environment map[string]string
	Ports       []PortMapping
}

type ContainerRuntime interface {
	// Start is strictly idempotent. It creates/starts the container and returns its ID.
	Start(spec ContainerSpec) (containerID string, err error)

	// Stop gracefully terminates a container.
	Stop(containerID string) error

	// Status returns the current runtime state of the container.
	Status(containerID string) (ContainerState, error)

	// Remove cleans up the container resources.
	Remove(containerID string) error
}
```

### Deterministic Container Identity & Idempotent Operations
Operations like `Start` must be completely idempotent.
*   The underlying container name must be deterministically derived from the Instance ID (e.g., `runstack-<instance-id>`).
*   If `Start()` is called multiple times due to a network retry, it will resolve to the same underlying container object rather than spawning duplicates.

### Runtime Selection Policy
Runtime selection will be driven by explicit configuration and Node Capabilities, rather than blind fallbacks.
*   **Policy options**: `auto` (default), `docker`, `podman`.
*   The Agent inspects the node capabilities on startup.
*   It initializes the configured runtime (or highest priority available if `auto`).
*   This specific runtime identity is reported in the `NodeRegistry` capabilities.

### Preventing Shell Injection
`Command` and `Args` are strictly handled as `[]string` arrays and environment variables as structured key-value pairs. Implementations will pass these directly to the runtime API or CLI `exec` array, completely bypassing shell evaluation.

---

## 3. Instance State Machine & Control Plane Authority

The Control Plane is the absolute authority on state. The Agent merely reports **observations**.

### The Transition Flow
1.  **Agent Observation**: The Agent observes `StateExited` with `ExitCode=1`.
2.  **Report**: The Agent sends `POST /api/v1/instances/{id}/status` containing:
    *   `ExecutionID`
    *   `ObservedState=exited`
3.  **CP Decision**: The Control Plane validates the `ExecutionID` and determines the domain transition (e.g., `RUNNING` -> `CRASHED`).

### State Matrix
*   **`PENDING`**: The Reconciler determined capacity is missing.
*   **`ASSIGNED`**: The InstanceScheduler has mapped it to a Node.
*   **`STARTING`**: The Agent has claimed it. A valid `ExecutionID` exists.
*   **`RUNNING`**: The container process is active.
*   **`CRASHED`**: The container exited unexpectedly, failed to start, or the Node went offline. Terminal.
*   **`STOPPED`**: The Control Plane requested shutdown (e.g., rollout), and the container is gone. Terminal.

**Allowed Domain Transitions**:
*   `PENDING` -> `ASSIGNED` (Scheduler)
*   `ASSIGNED` -> `STARTING` (Agent Claim)
*   `STARTING` -> `RUNNING` (Agent Observation)
*   `STARTING` -> `CRASHED` (Agent Observation or Node Offline)
*   `RUNNING` -> `CRASHED` (Agent Observation or Node Offline)
*   `STARTING` / `RUNNING` -> `STOPPED` (Reconciler Scale Down)
*   *(Note: `CRASHED` and `STOPPED` are strictly terminal. No resurrection is permitted.)*

### Strict Ownership of Protected Fields
Fields like `NodeID`, `DeploymentID`, `ExecutionID`, and `ContainerID` belong exclusively to the domain layer. Clients cannot arbitrarily mutate them via generic `PATCH / PUT` API updates.
`ContainerID` is set by the Control Plane solely in response to a successful Agent `Start` observation.

---

## 4. Monitoring, Graceful Shutdown, & Failure Semantics

### Agent Monitor Loop
The Agent runs an `InstanceMonitor` loop that tracks instances it owns (via matching `ExecutionID`). It polls `ContainerRuntime.Status()` and pushes observations to the Control Plane.
Health checks (healthy/unhealthy application status) are excluded from Milestone 8 to maintain focus on raw execution state.

### Explicit Runtime Failure Semantics
*   **Stop Failure**: If the Reconciler marks an instance as `STOPPED` but the Agent's `Stop()` operation repeatedly fails, the Agent will report this failure. The CP will maintain the instance in a terminal error state rather than assuming it vanished, allowing administrative intervention.
*   **Start Failure**: If image pull fails, the Agent reports a terminal error, resulting in a `CRASHED` transition. The Reconciler provisions a new `PENDING` replacement.

### Deployment Rollout
When an Application is updated, the Reconciler marks instances from the `SUPERSEDED` deployment as `STOPPED` in the Control Plane.
The Agent detects this desired state, calls `ContainerRuntime.Stop()` (delegating the specific termination behavior—e.g., SIGTERM then SIGKILL—to the Docker/Podman adapter), and calls `ContainerRuntime.Remove()`.
