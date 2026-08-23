# Step 8B Design Document: Instance Lifecycle Execution

## 1. Current Architecture
RunStack currently separates the Control Plane (Application, Deployment, Instance domain state) from the Execution Plane (Agent, Job execution, Container Runtime). Step 8A established a safe, idempotent `ContainerRuntime` interface with a Docker adapter, bounded by strict execution identity (`ExecutionID`).

## 2. Problem Statement
The Control Plane currently manages `Instance` capacity abstractly. We must now connect this abstract state to the physical `ContainerRuntime` on the Agent, ensuring that container lifecycle divergence (crashes, missing containers, network partitions) is handled cleanly, securely, and idempotently without leaking runtime details into the domain logic.

## 3. Goals
- Define a strict lifecycle state machine for Instances.
- Establish an `InstanceExecutor` within the Agent.
- Create secure API boundaries for claiming and reporting Instance execution.
- Map runtime observations to domain state changes idempotently.
- Handle node/agent failures and container crashes without split-brain execution.

## 4. Non-Goals
- Rolling or Canary deployments.
- Health checks and readiness probes.
- Persistent volumes or networking orchestration.
- Reusing Job execution semantics for long-running Instances.

## 5. Instance Lifecycle State Machine
*   **`PENDING`**: Desired capacity recognized by CP. Unassigned.
*   **`ASSIGNED`**: Mapped to a Node by `InstanceScheduler`.
*   **`STARTING`**: Claimed by Agent. `ExecutionID` generated. Runtime `Start()` invoked.
*   **`RUNNING`**: Agent observed `running` from `ContainerRuntime`.
*   **`STOPPING`**: CP desired state is scaled down or superseded. Agent invokes `Stop()`.
*   **`STOPPED`**: Terminal. Agent confirms container removed/exited.
*   **`CRASHED`**: Terminal. Container exited unexpectedly or Node went offline.

## 6. Runtime State Mapping
The `ContainerRuntime` returns generic states: `running`, `exited`, `unknown`.
*   `running` -> Agent reports `RUNNING`.
*   `exited` -> Agent reports `CRASHED` (if desired state is active) or `STOPPED` (if scaling down).
*   `unknown` -> Agent delays reporting (transient).
*   `missing` (ErrContainerNotFound) -> Agent reports `CRASHED` (if it was previously STARTING/RUNNING).

## 7. Execution Ownership Model
Instances are strictly fenced by a unique `ExecutionID` assigned at claim time.
*   `Instance` domain model adds `ExecutionID string`.
*   The CP rejects any status update with a mismatched `ExecutionID`.
*   If an Agent goes offline, CP node-recovery marks the Instance `CRASHED` and creates a new one. The old Agent's `ExecutionID` is permanently invalidated. If the old Agent returns, its status updates are rejected, prompting it to clean up the orphaned container.

## 8. Agent Workflow
1.  **Poll**: Agent queries `GET /api/v1/instances?node_id=XYZ&status=ASSIGNED`.
2.  **Claim**: Agent calls `POST /api/v1/instances/{id}/claim`. Receives `ExecutionID` and `AppSpec`.
3.  **Start**: Agent invokes `ContainerRuntime.Start()`.
4.  **Monitor**: `InstanceMonitor` loop continually calls `ContainerRuntime.Status()` for claimed instances.
5.  **Report**: Changes in status are pushed via `POST /api/v1/instances/{id}/status`.
6.  **Stop**: If CP returns `409 Conflict` (stale execution) or explicitly signals scale-down, Agent invokes `Stop()` and `Remove()`.

## 9. Control Plane Workflow
1.  **Assign**: `InstanceScheduler` sets `ASSIGNED` + `NodeID`.
2.  **Grant Claim**: `Claim` API validates `ASSIGNED`, sets `ExecutionID`, transitions to `STARTING`.
3.  **Accept Status**: `Status` API validates `ExecutionID`, updates status to `RUNNING`, `CRASHED`, or `STOPPED`.

## 10. Reconciliation Algorithm
*   **Desired: RUNNING | Observed: STOPPED/MISSING/EXITED**: Agent reports `CRASHED`. CP drops it from viable capacity. Reconciler creates new `PENDING` replacement.
*   **Desired: RUNNING | Observed: RUNNING**: Normal operation. No action.
*   **Desired: STOPPED (Superseded) | Observed: RUNNING**: Agent receives scale-down signal, invokes `Stop()`, reports `STOPPED`.

## 11. Idempotency Semantics
*   **Agent retries**: If network fails during Claim or Start, Agent retries.
*   **Start/Stop/Remove**: Handled safely by Step 8A adapter.
*   **CP Status**: `UpdateStatus` is idempotent. Updating `RUNNING` to `RUNNING` is a no-op.

## 12. Failure Handling
*   **Runtime Unavailable/Crash Loop**: `Start()` fails. Agent reports `CRASHED`. CP provisions replacement. (Future: backoff logic).
*   **Agent Crash**: Node goes offline. CP Node-aware recovery marks `CRASHED`. Replacement is provisioned.
*   **Missing Container**: Treated as `CRASHED`.
*   **Duplicate Operation**: Safely absorbed by CP idempotency and runtime adapter.

## 13. Security Model
*   Rely purely on the Step 8A adapter for shell safety and flag injection protection.
*   Agents only receive `ContainerSpec` for Instances explicitly assigned to them.
*   `ExecutionID` prevents Agent B from spoofing Agent A's container status.

## 14. API Changes
*   `POST /api/v1/instances/{id}/claim`
*   `POST /api/v1/instances/{id}/status`
*   *(No changes to existing Job endpoints to avoid domain contamination).*

## 15. Logging/Observability
*   Structured logs (`cp` and `agent`) for every lifecycle transition, embedding `instance_id`, `node_id`, `execution_id`, `deployment_id`.

## 16. Concurrency Model
*   Agent `InstanceExecutor` runs a dedicated goroutine loop distinct from `JobExecutor`.
*   CP `InstanceRegistry` manages locks independently of `JobRegistry`.

## 17. Test Strategy
*   Unit test `InstanceExecutor` with a `FakeContainerRuntime`.
*   Test stale `ExecutionID` rejections.
*   Test `PENDING` -> `ASSIGNED` -> `STARTING` -> `RUNNING` -> `STOPPED` full flow in integration.
*   Test Agent crash recovery.

## 18. Integration Test Strategy
*   Wire CP, Agent, and `FakeContainerRuntime` in `test/integration/`.
*   Verify Reconciler creates replacements when Agent reports `CRASHED`.

## 19. Exact Files Likely to Change
*   `internal/instance/instance.go` (Add ExecutionID, ContainerID)
*   `internal/api/instance.go` (Claim/Status handlers)
*   `cmd/runstack/agent.go` (Wire InstanceExecutor)
*   `internal/executor/instance_executor.go` (New Agent component)

## 20. Recommended Implementation Order
1.  Extend `Instance` domain model with `ExecutionID` and terminal state enums.
2.  Add CP API endpoints for Claim and Status.
3.  Implement `FakeContainerRuntime` for tests.
4.  Implement `InstanceExecutor` in the Agent.
5.  Wire Agent to `cp` and write integration tests.

## 21. Definition of Done
Instances successfully move from `PENDING` to `RUNNING` on an Agent, crashes are accurately reported as `CRASHED`, scale-downs result in `STOPPED`, and no Job logic is entangled.
