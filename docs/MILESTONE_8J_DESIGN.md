# Milestone 8J Design: Resource Limits & Capacity-Aware Scheduling

## 1. Problem
Currently, the RunStack scheduler distributes Instances and Jobs across `ONLINE` nodes using a naive round-robin algorithm. It completely ignores both the application's resource requirements and the node's physical hardware capacity. Furthermore, the Agent launches Docker containers with unbounded resource usage. This allows a single memory-leaking application to trigger the Linux OOM killer, crashing the Agent, other tenant applications, or the node itself, fundamentally violating the reliability expectations of a PaaS.

## 2. User Value
- **Stability**: Prevents noisy-neighbor scenarios and cascading node failures.
- **Predictability**: Guarantees applications get the CPU and RAM they require to function correctly.
- **Efficiency**: Packs nodes tightly without exceeding safety thresholds.
- **Multi-tenant Safety**: Ensures one application cannot monopolize the node's CPU, starving others.

## 3. Existing Architecture Affected
- `internal/application/application.go` (`AppSpec` additions)
- `internal/scheduler/instance_scheduler.go` (Filtering and scoring)
- `internal/scheduler/scheduler.go` (Job scheduling filtering)
- `internal/runtime/docker/docker.go` (Runtime flag injection)
- `internal/api/` (Validation of resource requests)

## 4. Domain Model Changes
Introduce a `Resources` struct into `application.AppSpec`:
```go
type ResourceRequirements struct {
    CPU    float64 `json:"cpu,omitempty"`    // e.g., 0.5 for half a core
    Memory int     `json:"memory,omitempty"` // e.g., 512 for 512MB
}

type AppSpec struct {
    ...
    Resources *ResourceRequirements `json:"resources,omitempty"`
}
```

## 5. Control Plane Changes
The Control Plane must dynamically calculate the **Available Capacity** of a Node.
Since we have no database, this is computed entirely in-memory on the fly (or via a cached index):
`AvailableMemory = Node.Capabilities.TotalMemoryBytes - Sum(Active Instances Memory)`
`AvailableCPU = Node.CPUCores - Sum(Active Instances CPU)`
Only instances in `ASSIGNED`, `STARTING`, `RUNNING`, or `UNKNOWN` state count towards allocation.

## 6. Agent Changes
The Agent receives the `Resources` block as part of the `resolvedSpec` during the `Claim` payload. It passes this down to the `ContainerRuntime`.

## 7. Runtime Changes
`internal/runtime/docker/docker.go` must translate the `Resources` block into Docker flags:
- `CPU`: `--cpus=<value>`
- `Memory`: `--memory=<value>m`

## 8. API Changes
- `POST /api/v1/apps` and `PUT /api/v1/apps/{id}` will accept the new `Resources` block.
- `GET /api/v1/nodes` should optionally expose current allocation utilization to help users understand cluster capacity.

## 9. CLI Changes
- `runstack app create --cpu 1.0 --memory 512`
- `runstack deploy --cpu 2.0 --memory 1024`
- `runstack nodes` output updated to show `Allocated / Total` for CPU and Memory.

## 10. Scheduler/Reconciler Interaction
The `InstanceScheduler` and `JobScheduler` must transition from pure round-robin to:
1. **Filter**: Reject nodes where `AvailableMemory < RequiredMemory` or `AvailableCPU < RequiredCPU`.
2. **Sort**: Sort remaining nodes to balance load (e.g., least allocated node first) or maintain the round-robin cursor across the *filtered* subset.
If no nodes have capacity, the Instance/Job remains `PENDING`.

## 11. Failure Semantics
If an application exceeds its memory limit, the Docker runtime (cgroups) will OOM kill it. The container transitions to `StateExited` with OOM exit codes. The Agent detects this, marks it `CRASHED`, and the Control Plane increments the crash-loop counter. This behaves perfectly with existing 8C recovery semantics.

## 12. Security Model
Enforcing container limits hardens the runtime security boundary, preventing CPU exhaustion DoS attacks and memory starvation attacks from malicious or buggy tenant code.

## 13. Concurrency Model
The Scheduler already holds `s.mu.Lock()` and iterates over immutable registry snapshots. Calculating capacity dynamically during the scheduling loop is thread-safe and requires no new locks, preserving the lock-free reads elsewhere.

## 14. Resource Lifecycle
Allocations are inherently tied to the Instance lifecycle. When an Instance transitions to `STOPPED` or `CRASHED` (e.g., via node timeout), it no longer counts towards node allocation, instantly freeing capacity for the Scheduler.

## 15. Observability
Logs and events must reflect scheduling failures due to capacity constraints (e.g., `slog.Warn("Insufficient capacity for instance", "app", appID)`).

## 16. Rollout Interaction
During a rollout, both `v1` and `v2` instances exist simultaneously. The `maxSurge` setting requires *extra* node capacity. If the cluster is exactly 100% full, a surge rollout will stall in `PENDING`. Users must manage capacity or use `maxUnavailable > 0` to free capacity before surging. The architecture handles this naturally.

## 17. Rollback Interaction
Rollback immediately supersedes the bad deployment. The bad instances are marked `STOPPING` and release capacity, allowing the old deployment instances to be scheduled.

## 18. Migration Impact
Existing deployments without `Resources` blocks should default to a safe baseline (e.g., 0.1 CPU, 128MB RAM) or remain unbounded (though unbounded is dangerous). Unbounded is recommended for backward compatibility unless explicitly overridden.

## 19. Test Strategy
Unit tests for `InstanceScheduler` must mock nodes with limited capacity and verify that `PENDING` instances are only assigned to nodes that can fit them, and remain `PENDING` if the cluster is full.

## 20. Integration Tests
Fake runtime tests verifying that multiple large instances correctly exhaust a fake node's capacity, forcing subsequent instances to remain `PENDING`.

## 21. Non-Goals
- Real-time auto-scaling (HPA) based on metrics.
- CPU/Memory overcommit ratios (strict enforcement only for V1).
- Live-migration of instances across nodes.

## 22. V1 Constraints
Maintains the "No Database" constraint. All capacity calculations are computed dynamically in-memory from the Instance Registry during the scheduling tick.

## 23. Known Limitations
- The scheduler calculates requested/allocated capacity, not *actual* utilization. If an app requests 1GB but uses 10MB, 1GB is still reserved.
- In-memory capacity calculation across thousands of instances is technically $O(N \times M)$ on every scheduler tick, but well within acceptable limits for a V1 in-memory PaaS.

## 24. Exact Files Likely to Change
- `internal/application/application.go`
- `internal/application/validation.go`
- `internal/scheduler/instance_scheduler.go`
- `internal/scheduler/job_scheduler.go`
- `internal/scheduler/node_capacity.go` (new)
- `internal/runtime/docker/docker.go`
- `internal/runtime/runtime.go`
- `cmd/runstack/main.go` (CLI flags)

## 25. Recommended Implementation Order
1. Extend `AppSpec` with `Resources` and validate.
2. Implement `NodeCapacity` calculator.
3. Update `InstanceScheduler` and `JobScheduler`.
4. Update `DockerRuntime` to inject `--cpus` and `--memory`.
5. Update CLI commands and outputs.
