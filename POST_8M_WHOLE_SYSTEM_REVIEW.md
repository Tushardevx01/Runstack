# POST-8M WHOLE-SYSTEM + PRODUCT REVIEW

## 1. Executive Summary
RunStack has evolved from a primitive task queue (Milestone 1) into a robust, declarative, stateless container orchestration platform (Milestone 8M). It successfully enforces a "No Database" constraint while delivering production-grade features: zero-downtime rollouts, auto-TLS, custom ingress, capacity-aware scheduling, and declarative GitOps reconciliation. 

**Is RunStack now a genuinely usable self-hosted V1 PaaS?**
**YES.** For stateless containerized workloads pulling from standard registries, RunStack provides a complete, secure, and resilient deployment pipeline.

**The Single Largest Remaining Blocker:**
**Operational Visibility & Application UX.** The platform has transitioned to a declarative Application model, but the CLI remains stuck in the imperative Job era (`runstack jobs`, `runstack nodes`). Operators cannot easily visualize `Application` rollout status, instance placement, degraded health, or cluster resource utilization (`runstack top`) without manually scraping the API.

## 2. Current Capability Map
*   **Execution:** Node-aware capacity scheduling, Docker lifecycle management.
*   **Resilience:** Instance crash loop detection, auto-recovery, node-drain recovery.
*   **Traffic:** TCP proxy routing, SNI routing, Auto-TLS (`autocert`), zero-downtime transition.
*   **Security:** Bearer token authentication, NodeToken agent identity, injected secrets (no plaintext leaks).
*   **Deployment:** Declarative manifests (`runstack.yaml`), idempotent apply, immutable Deployment tracking, probes.
*   **Operations:** Cross-application isolation, drift pruning, live log streaming.

## 3. Developer Journey
**The Golden Path:**
1. `runstack context add` (Authenticate)
2. `runstack secret set my-app DB_PASS ...` (Secure provisioning)
3. `runstack validate` (Local schema check)
4. `runstack apply -f runstack.yaml` (Idempotent submission)
5. Control plane resolves dependencies, requests TLS, and generates an immutable Deployment.
6. `RolloutReconciler` schedules Instances to Agent nodes based on CPU/Mem capacity.
7. Agent pulls image, starts container, monitors Readiness/Liveness.
8. `RoutingReconciler` maps healthy ports; `IngressRegistry` answers to `example.com`.
9. `runstack logs -a my-app` (Watch traffic).

*Friction Point:* Between steps 5 and 7, the operator is blind. There is no `runstack app list` or `runstack app status` to watch the rollout transition from `PROGRESSING` to `COMPLETED`.

## 4. Security Assessment
*   **Privilege Escalation:** Prevented. CLI actions require the Operator Bearer token. Agents are restricted to NodeTokens and can only pull their assigned jobs.
*   **Cross-Application Access:** Enforced. `ApplyHandler` strictly filters domains and secrets by `app.ID`. Manifests cannot hijack domains owned by other apps.
*   **Secret Leakage:** Secure. Secrets are stored in CP memory and injected via the Agent API into container memory. They are explicitly excluded from deployment hashes and diffs.
*   **Certificate Abuse:** Mitigated. `autocert` SNI interception only requests Let's Encrypt challenges for domains explicitly registered and owned by an Application.
*   **Manifest Abuse:** Mitigated. `yaml.v3` `KnownFields(true)` prevents parser pollution.

## 5. Reliability Assessment
*   **CP Restart:** Fully recoverable. In-memory state wipes, but `runstack apply` safely and idempotently rebuilds App, Secret validation, Domains, TLS, and Deployments.
*   **Agent/Node Failure:** Detected via missed heartbeats. `InstanceReconciler` marks instances `UNKNOWN` -> `CRASHED` and reschedules them to healthy nodes.
*   **Rollout Failure:** If an image is broken, readiness probes fail. `RolloutReconciler` pauses the rollout, preventing the destruction of the previous stable deployment.
*   **Partial Apply Failure:** Handled seamlessly. Because Deployments are matched by SHA256 hash, an interrupted apply can be retried and will seamlessly adopt the orphaned deployment state.

## 6. Resource Assessment
*   **Capacity Scheduling:** Node memory and CPU are tracked. The scheduler correctly rejects placements that exceed node capacity.
*   **OOMs:** Handled via Docker. Agent detects container exit, reports back to CP, triggers crash loop backoff.
*   *Gap:* No visibility into *actual* consumption vs *requested* limits.

## 7. Networking/TLS Assessment
*   **Ingress:** Path-based mapping (`/`) to Service targets works reliably.
*   **TLS:** Handled entirely in-memory via `autocert`. 
*   *Volatility:* Certificates disappear on CP restart. This is an accepted V1 tradeoff to avoid a database, bounded by Let's Encrypt rate limits and `autocert` deduplication caching.

## 8. Declarative/Apply Assessment
*   **Durable State:** `runstack.yaml` successfully replaces the need for a database.
*   **Immutability:** Modifying `image` or `env` reliably produces a different hash, triggering a new Deployment. Modifying `replicas` safely scales the existing deployment.
*   **Pruning:** Removing a domain from the YAML reliably deletes it from the CP, but *only* for that specific Application.
*   **Side-effects:** `diff` and `validate` are strictly read-only or local.

## 9. Observability Assessment
*   **Operator Understanding:** *Poor.* The declarative engine is brilliant, but operators lack the tools to see it. If a rollout blocks due to a crash loop, the operator must guess or manually parse `runstack jobs` / `runstack logs`.
*   **TLS Status:** Invisible to the operator until `curl` succeeds or fails.
*   **Capacity:** `runstack nodes` shows presence, but not remaining allocatable capacity.

## 10. CLI Assessment
*   **Strong:** `apply`, `deploy` (imperative), `secret`, `logs`, `validate`.
*   **Missing / Weak:** 
    *   No `runstack apps` (List declarative apps).
    *   No `runstack app status <name>` (Show rollout, instances, health, domains).
    *   No `runstack top` (Show cluster/node capacity).

## 11. Remaining Gaps & Classification

| Gap | Classification | Reason |
| :--- | :--- | :--- |
| **App Observability UX** | **CRITICAL** | Operators cannot see if `apply` succeeded in runtime without scraping lower-level primitives. |
| **Cluster Metrics (Top)** | **HIGH** | Required to tune `resources` correctly in the manifest. |
| **Internal Service Discovery** | **MEDIUM** | Microservices must route through public ingress; no internal cluster DNS. |
| **ConfigMaps / Volume Mounts** | **MEDIUM** | Hard to pass complex `.yml` configuration files to apps via env vars. |
| **Centralized Logging** | **INTENTIONAL V1 LIMITATION** | Requires ELK/Promtail/Storage, violating the lightweight in-memory goal. |
| **Persistent Volumes** | **INTENTIONAL V1 LIMITATION** | Breaks node-agnostic scheduling and requires complex distributed storage (CSI). |
| **Certificate Persistence** | **INTENTIONAL V1 LIMITATION** | Requires database/disk. In-memory ACME is an accepted tradeoff. |
| **Source-to-Image (Builds)** | **V2 ONLY** | Adding a builder component (like Heroku buildpacks) is too complex for V1. |
| **Multi-user RBAC** | **V2 ONLY** | V1 assumes a single trusted operator/team per cluster. |

## 12. V1 Limitations
RunStack V1 is explicitly bounded:
1. **No Database:** The CP is ephemeral. Restarting the CP temporarily drops routing and requires `runstack apply` to reconstruct desired state.
2. **Stateless Workloads Only:** No persistent volumes. Databases must be hosted externally (RDS, Cloud SQL, etc.).
3. **Bring Your Own Image:** Operators must build and push Docker images to a registry; RunStack only orchestrates them.

## 13. V2 Candidates
*   PostgreSQL / Etcd backed Control Plane (eliminating restart volatility).
*   Source-to-Image build pipelines (`runstack push`).
*   Persistent Volume Claims.
*   Internal Service Mesh / DNS.
*   Horizontal Pod Autoscaling.

## 14. Ranked Priorities
1. **Application Dashboards & CLI UX:** Operators need to see their Apps.
2. **Resource Metrics:** Operators need to see their cluster capacity.
3. **Internal Service Routing:** Apps need to talk to Apps privately.
4. **Config File Injection:** Complex app configuration.

## 15. Recommended Next Milestone
### Milestone 8N: Application Observability & CLI UX

Do not build persistence. Do not build a database. Do not build internal DNS yet.
**Make the PaaS usable by the human operator.** The transition to 8M's declarative model left the CLI behind.

## 16. Exact Proposed Scope (8N)
1. **App Read API:** Add `GET /api/v1/apps` and `GET /api/v1/apps/{id}` endpoints that aggregate Application, Active Deployment, Rollout Status, Domains, and Instance health into a single JSON response.
2. **CLI App Commands:**
    *   `runstack apps`: Table view of apps, replicas (ready/desired), and rollout status.
    *   `runstack app status <name>`: Detailed tree view showing the active deployment, domain mappings, TLS status, and list of running instances with their exact node placements and restart counts.
3. **CLI Capacity Commands:**
    *   Enhance `runstack nodes` or add `runstack top` to show *Allocated vs Total* CPU and Memory per node based on scheduled instances.
4. **Interactive Wait:** Optionally add `runstack apply --wait` to block until the RolloutReconciler reports `COMPLETED`.

## 17. Explicit Non-Goals
*   No historical metrics (Prometheus).
*   No web UI/Dashboard (CLI only).
*   No persistent logging storage.

==================================================
FINAL VERDICT:
**READY FOR 8N DESIGN**
==================================================
