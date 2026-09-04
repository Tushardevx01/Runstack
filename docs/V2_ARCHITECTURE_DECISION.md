# RunStack V2: Architecture Decision Record

## Critical Context
RunStack V1 is officially frozen at Milestone 8O (`63dd559`). V1 successfully proved the domain model (Application → Deployment → Instance) and developer workflow (`runstack.yaml` → `apply`) using a strictly in-memory, single-binary architecture with a trusted Control Plane and thin, poll-based Node Agents.

However, V1 is fundamentally limited by its lack of durable state. The 9 internal `sync.RWMutex`-backed registries are lost on restart, lack cross-registry transactions (creating TOCTOU races in scheduling and application updates), and completely preclude a High Availability (HA) Control Plane.

This document establishes the foundational state and consistency models required for RunStack V2.

---

## 1. State Model Candidates

Evaluation of durable state backends against actual RunStack V1 access patterns:

### A. Embedded SQLite
* **Migration**: Straightforward mapping of current structs to tables.
* **Concurrency**: Great for single-node (WAL mode), but multi-control-plane concurrent writes require external coordination (e.g., LiteFS, rqlite) which introduces complex filesystem/consensus dependencies.
* **Transactions**: Full ACID, solving the V1 cross-registry `AppRegistry.Update` + `DeploymentRegistry.UpdateState` gap.
* **Suitability**: Excellent for a single-node V2, but fundamentally fights the requirement for a true active-active or active-standby HA Control Plane without heavy add-ons.

### B. PostgreSQL
* **Concurrency & ACID**: Industry standard. Provides robust row-level locking (`SELECT FOR UPDATE SKIP LOCKED`), which perfectly replaces V1's `scheduler.mu` to prevent concurrent scheduling races across multiple CP nodes.
* **HA Options**: Highly mature external ecosystem (Patroni, RDS, Cloud SQL). Pushes HA complexity out of the RunStack binary.
* **Migrations**: Requires a standard schema migration tool (e.g., golang-migrate).
* **Suitability**: Solves V1's TOCTOU capacity calculation issues, provides transactional safety for App+Deployment+Instance rollouts, and allows multiple Control Planes to operate statelessly over a shared truth.

### C. etcd
* **Coordination**: Purpose-built for distributed state and watches.
* **Fit for RunStack**: Poor. RunStack's state is heavily relational (Apps have Deployments, Deployments have Instances, Capacity is an aggregate of Instances and Jobs). etcd is a KV store. Rebuilding V1's relational models into etcd KV paths would require massive application-side join logic and index management, mimicking Kubernetes but without the K8s API machinery.

### D. Raft-based custom/Hashicorp storage
* **Model**: Embeds a Raft consensus node in every Control Plane.
* **Implementation Complexity**: Extremely high. Every V1 registry mutation (9 registries) must be modeled as a deterministic Raft log command and applied to a Replicated State Machine. 
* **Operational Burden**: Operator now has to manage quorum, snapshotting, and log compaction inside RunStack.
* **Suitability**: Unjustified engineering cost for a PaaS. We are building a PaaS, not a database.

### E. CockroachDB / TiDB
* **Model**: Distributed SQL.
* **Resource Cost**: Very heavy footprint. Demands multiple nodes, significant memory, and careful clock synchronization.
* **Suitability**: Massive overkill. RunStack V1 fits its entire state in a few megabytes of RAM; scaling to a distributed SQL cluster is entirely disproportionate to the workload.

### Conclusion: Candidate Comparison

| Criteria                 | SQLite | PostgreSQL | etcd | Raft | CockroachDB/TiDB |
| ------------------------ | ------ | ---------- | ---- | ---- | ---------------- |
| Persistence              | Yes    | Yes        | Yes  | Yes  | Yes              |
| Consistency              | Strong | Strong     | Strong| Strong| Strong         |
| HA                       | Poor*  | Excellent  | Excellent| Good | Excellent    |
| Operational complexity   | Low    | Medium     | High | High | Very High        |
| Migration complexity     | Low    | Medium     | High | High | High             |
| Controller compatibility | Good   | Excellent  | Poor | Poor | Good             |
| Scheduling compatibility | Good   | Excellent  | Poor | Poor | Good             |
| TLS/secret lifecycle     | Good   | Excellent  | Good | Good | Good             |
| Backup/recovery          | Easy   | Easy       | Medium| Hard| Medium           |
| Long-term V2 fit         | Limited| Perfect    | Poor | Poor | Overkill         |

*\* HA SQLite requires complex VFS layers or consensus wrappers.*

---

## 2. Consistency Requirements

Based on a detailed audit of V1, different subsystems require different consistency models.

### Strong Consistency / Serializable Transactions
* **App & Deployment Lifecycle**: V1 `AppService.UpdateApp` uses three separate locks, causing a split-state vulnerability. V2 requires transactional atomicity when superseding a Deployment and updating the Application's `ActiveDeploymentID`.
* **Job & Instance Scheduling**: V1 calculates capacity via `CapacityCalculator.CalculateAll` and then assigns instances. In HA, two CP nodes could read the same capacity snapshot and double-book a Node. V2 requires `SELECT FOR UPDATE` or strict serializable transactions to decrement capacity and assign work atomically.
* **ExecutionID Fencing**: V1 relies on in-memory atomic updates to ensure an Agent claim matches the `ExecutionID`. V2 requires strict Compare-And-Swap (CAS) semantics via DB constraints to reject stale Agent updates.
* **Node Registration**: The V1 O(n) token scan must be replaced with a uniquely indexed database lookup.

### Eventual Consistency
* **Traffic Routing (`HTTPProxy`)**: The V1 `RoutingReconciler` already operates on a 1-second poll loop, generating route tables and swapping them via lock-free `atomic.Pointer`. This is fundamentally eventually consistent and highly performant. V2 can retain this exactly as-is; the proxy simply reads the latest DB state eventually.
* **Health Probes & Crash Recovery**: Reconcilers already operate on timers and thresholds (`ConsecutiveCrashes`). Eventual consistency is perfectly acceptable here.

**Minimum Required Consistency Model:** **Read-Committed with Row-Level Explicit Locking** for all mutating controller/scheduler operations.

---

## 3. HA Control Plane

Using the chosen relational state model, the Control Plane (CP) becomes a stateless API and coordination layer.

### 2-Node Topology (Active/Passive Reconcilers)
* **Split-Brain Risk**: If CP A and B are partitioned from each other but both can reach the Agents and Database, they must not both run the `InstanceReconciler` or `Scheduler`.
* **Fencing**: The Database acts as the arbiter. CPs use an advisory lock (or a `leader_election` table with a heartbeat lease) to elect a single leader for background scheduling/reconciliation. 
* **API Traffic**: Both CP A and B can actively serve Operator API traffic (`runstack apply`, `runstack logs`) and Agent traffic (`/claim`, `/status`).

### 3-Node Topology
* **Quorum**: Managed entirely by the database layer (e.g., Patroni/Postgres quorum). 
* **Leadership**: One CP acquires the Postgres advisory lock to become the active Scheduler/Reconciler. If it crashes or partitions, the lock drops, and another CP claims it.
* **Network Partition**: If CP C is isolated from the DB, its API requests fail, and it cannot acquire the leader lock. Agents reporting to CP C will receive 500s and retry against CP A/B (assuming an external LB). 

### Fencing and Duplicate Prevention
If a stale CP controller pauses, loses leadership, and wakes up, its write attempts to the DB will either fail (because the `ExecutionID` or `Status` changed underneath it) or be rejected by standard transactional concurrency controls. The Database guarantees isolation.

---

## 4. Registry Migration Boundary

The V1 `sync.RWMutex` + `map[string]T` registries must be entirely dismantled. 

| V1 Component | Current Storage | V2 Storage | Verdict | Reason |
| ------------ | --------------- | ---------- | ------- | ------ |
| `AppRegistry` | Memory Map | DB Table | **Rebuild** | Requires relational joins to Deployments. |
| `DepRegistry` | Memory Map | DB Table | **Rebuild** | Must transactionally update with App state. |
| `InstRegistry` | Memory Map | DB Table | **Rebuild** | Must support row-level locks for concurrent scheduling. |
| `JobRegistry` | Memory Map | DB Table | **Rebuild** | Needs garbage collection, history retention, locking. |
| `NodeRegistry` | Memory Pointer Map | DB Table | **Rebuild** | O(n) token lookup must become an indexed SQL query. |
| `SecretRegistry` | Memory Map | DB Table | **Adapt** | Move to DB, but requires envelope encryption for at-rest security. |
| `DomainRegistry` | Memory Map | DB Table | **Rebuild** | Relational mapping to Ingress/Routes. |
| `IngressRegistry` | Memory Map | DB Table | **Rebuild** | Relational mapping. |
| `ACMEProvider` | `autocert.memoryCache` | DB Table | **Adapt** | Certs must be stored durably to prevent rate-limiting on CP restart. |
| `LogRingBuffer` | Agent Memory | Agent Memory | **Promote** | Crash logs intentionally remain volatile and Agent-local. |

---

## 5. Component Verdict

* **API Layer / CLI / Manifest Format**: **PROMOTE**. The declarative `runstack.yaml` and standard HTTP API are clean and stateless.
* **Scheduler & Reconciler Loops**: **ADAPT**. The logic of capacity calculation and rollout math remains, but in-memory mutexes (`s.mu`, `r.mu`) must be replaced by Database leader election and row-level locks.
* **HTTP Proxy & Routing**: **PROMOTE**. The `atomic.Pointer` lock-free design is perfect. The `RoutingReconciler` will just query the DB instead of memory.
* **Agent Model**: **PROMOTE**. Agents remain thin, pull-based, and stateless.
* **Execution Fencing**: **ADAPT**. The `ExecutionID` logic moves from in-memory CAS to a SQL `WHERE ExecutionID = ?` constraint.
* **In-Memory Registries**: **RETIRE**. Entirely replaced by a Data Access layer (Repositories).

---

## Final Verdict: CHOSEN DURABLE STATE BACKEND

**RunStack V2 will use PostgreSQL as its exclusive durable state backend.**

### Defense of Choice
1. **Fits Architecture**: RunStack's state is heavily relational. Applications own Deployments; Deployments own Instances; Nodes own capacity. PostgreSQL handles these relationships, cascading deletes, and strict constraints natively, unlike etcd.
2. **V2 Capabilities**: PostgreSQL easily supports Multi-User/RBAC, historical telemetry retention, and persistent volumes via standard SQL schemas.
3. **Consistency & Concurrency**: PostgreSQL's `SELECT FOR UPDATE SKIP LOCKED` is the exact primitive needed to allow multiple Control Plane nodes to safely and concurrently pop PENDING jobs off a queue without double-assigning them, eliminating the V1 TOCTOU bugs.
4. **HA Control Plane**: Postgres supports advisory locks for leader election (allowing 1 active Reconciler / N active API nodes). Actual database HA is delegated to proven external tooling (Cloud SQL, RDS, Patroni), keeping the RunStack binary stateless and focused.
5. **Rejected Alternatives**: 
   * *SQLite* fails the multi-CP HA requirement cleanly.
   * *etcd* is a KV store that fights relational data models and aggregation (capacity math).
   * *Raft* forces us to build a distributed database instead of a PaaS.
   * *CockroachDB* is operational overkill for our scale.

### Implementation Readiness Summary
* **Chosen Backend**: PostgreSQL.
* **Consistency Model**: Read-Committed + Explicit Row Locks.
* **HA Model**: Active/Active for API; Active/Passive (via DB Lock) for Reconcilers.
* **Migration Strategy**: Rip out `sync.RWMutex` registries; replace with `database/sql` Repositories.
* **First Proposed V2 Milestone (Do not implement yet)**: **Milestone 9A: PostgreSQL Foundation & Repository Migration**.
