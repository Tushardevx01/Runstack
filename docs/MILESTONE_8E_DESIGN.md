# Milestone 8E: Traffic Routing & Service Exposure

## Objective
Transition RunStack from a capacity-aware deployment engine to a traffic-aware PaaS. This milestone introduces a dedicated Service and Routing layer to handle external traffic exposure, host-port allocation, and zero-downtime traffic migration natively tied into the existing Rollout Controller's health gating.

## Core Architectural Decisions

### 1. Service as a First-Class Entity
Routing is decoupled from the `Application`. A new `Service` domain entity will be introduced:
```go
type Service struct {
    ID            string
    ApplicationID string
    Domain        string   // e.g., "api.runstack.local"
    Port          int      // Public-facing port
    TargetPort    int      // The internal Application port to route traffic to
    Protocol      string   // HTTP, TCP, etc.
}
```
A single Application can have multiple Services (e.g., public web interface vs private admin API).

### 2. Instance Endpoint Model & Dynamic Host-Port Ownership
To allow multiple instances of the same Application to run on a single Node without port collisions, RunStack will introduce explicit Port Allocation ownership at the Node layer.
- **Port Allocator:** The Node Registry/Agent will maintain an explicit port allocator map (`HostPort -> InstanceID`). 
- **Collision Safety:** When claiming an Instance, the Agent explicitly allocates a free ephemeral port from its local pool and assigns it. 
- **Release:** When the Instance is STOPPED or CRASHED, the port is explicitly released back to the Node's pool.
- **Observation:** `InstanceStatus` will include the allocated `NodeIP` and `HostPort` as its routing endpoint. 

### 3. Routing Desired vs Observed State
A new **Routing Reconciler** will be introduced to bridge the Service state with the underlying Proxy implementation.
- **Desired State:** `Service -> [List of Eligible Instance Endpoints]`
- **Observed State:** The currently active routing targets inside the Reverse Proxy's memory.
- The Reconciler computes the diff between Desired and Observed endpoints and idempotently pushes updates to the Proxy Provider.

### 4. Healthy-Instance Eligibility
The Routing Reconciler uses strict gating rules. An instance is ONLY added to the Desired Routing State if:
- `Status == StatusRunning`
- `Health == HealthHealthy`
- It belongs to the `ActiveDeploymentID` (or is a surviving previous deployment during a rollback/rollout phase).
- Its `ExecutionID` is current and matches the registry.

### 5. Drain/Remove Semantics
Removing an endpoint from traffic is not a hard kill. 
- **State Introduction:** Introduce a `DRAINING` state or a boolean `Draining: true` flag. 
- When an Instance needs to be scaled down by the Instance Reconciler, the Routing Reconciler first removes it from the Proxy's active backend pool.
- The Proxy stops sending *new* requests to the endpoint.
- After a configurable `DrainTimeout` (e.g., 10 seconds), the Instance Reconciler is permitted to actually issue the `StatusStopped` command to the Agent.

### 6. Single Routing Authority
There will be exactly one source of truth for routing:
```text
Instance Registry & Service Registry
       ↓
Routing Reconciler
       ↓
Proxy Provider Interface
       ↓
Reverse Proxy (e.g. built-in httputil.ReverseProxy)
```
Agents, Instance Executors, and Rollout Controllers NEVER modify proxy configuration directly. They only update their own domain states, which the Routing Reconciler passively observes.

### 7. Proxy Failure Isolation
Routing failures and application failures are strictly isolated domains.
- If the Proxy Provider fails to update its routing table for Endpoint A, Endpoint A's `InstanceStatus` remains `HEALTHY`.
- The Routing Reconciler simply logs the error and retries the routing table sync on the next tick.
- Network routing issues never artificially degrade the Application's internal health counters.

### 8. Routing Reconciliation Idempotency
The Routing Reconciler runs on a continuous tick (like the Instance Reconciler). 
On every tick, it:
1. Queries all Services.
2. Queries all eligible Instances per Service.
3. Computes the exact required endpoints.
4. Pushes the exact required endpoint state to the Proxy.
There are no incremental state patches (`add endpoint X`, `remove endpoint Y`) passed between layers, preventing drift.

### 9. Rollback + Rollout Interaction
The 8D Rollout Controller dictates capacity counts, and the Instance Reconciler creates/stops instances.
- **During a Rollout:** Both `v1` and `v2` deployments will concurrently have instances that meet the Health/Eligibility rules. The Routing Reconciler will natively route to a blend of both (Zero-Downtime Traffic Migration).
- **During a Rollback:** The `ActiveDeploymentID` flips back to `v1`. The Rollout Controller scales `v1` back up. As `v1` instances become healthy, they re-enter the Service routing table. Simultaneously, `v2` instances are placed into the `DRAINING` state, removed from traffic, and eventually terminated. 
