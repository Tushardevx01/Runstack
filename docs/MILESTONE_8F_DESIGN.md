# Milestone 8F: The Developer Experience (CLI & Logs)

This document revises the 8F design to explicitly address the boundaries, idempotency, and security of the developer workflow (Source to URL) and operational visibility (Logs), preserving the strict Control Plane / Agent separation established in milestones 1-8E.

## 1. Deploy Image Flow

### Source-to-Deployment Pipeline
1. **Source Code:** The developer writes code and a `Dockerfile`.
2. **Local Build:** The `runstack deploy` CLI executes a local `docker build` (using the developer's local Docker daemon).
3. **Image Push:** The CLI executes `docker push` to a configured registry.
4. **CP Deployment:** The CLI calls the Control Plane (`POST /api/v1/apps/{id}/deploy`) with the exact pushed image.
5. **Agent Pull:** The Control Plane schedules the `Instance`. The assigned Agent securely pulls the image via the `RuntimeAdapter` and starts the container.

### Configurable Registry & Identity
- **No Hidden Magic:** The image registry must be explicitly configured in `runstack.yaml` (e.g., `image: ghcr.io/my-org/my-app`). RunStack will **not** provide a hidden internal registry.
- **Immutable Deployment Identity:** The CLI will resolve the pushed image to its exact cryptographic digest (`ghcr.io/my-org/my-app@sha256:abcd...`). The `Deployment` spec in RunStack will store this digest—never a mutable tag like `:latest`. This ensures that a Deployment version is 100% immutable and stale tags cannot silently alter running instances.
- **Agent Access:** Agents pull images using their local Docker daemon's existing authentication (e.g., node IAM roles or pre-configured `~/.docker/config.json`). No credentials are passed through the Control Plane API in V1.

### Failure Handling
- **Build/Push Failure:** The CLI fails locally before communicating with the Control Plane. No state is mutated.
- **Image Pull Failure:** The Agent fails to pull the image. The container fails to start, and the Agent reports `CRASHED`.
- **Deployment Safety:** Because the instance is `CRASHED`, the `InstanceReconciler` increments `ConsecutiveCrashes`. Once `MaxCrashLoopThreshold` is reached, the Deployment becomes `DEGRADED`, the `RolloutController` pauses, and the old healthy `v1` instances are protected from being torn down.

## 2. Deploy Idempotency

`runstack deploy` must be perfectly idempotent to prevent accidental duplicate deployments.

### Idempotency Key
When the CLI calls `POST /api/v1/apps/{id}/deploy`, it provides the **Desired AppSpec** (which includes the specific image digest, environment variables, ports, and rollout strategy).

The Control Plane compares this desired spec against the `SpecSnapshot` of the application's current `ActiveDeployment`.
- **If the specs match identically:** The Control Plane returns the existing `ActiveDeploymentID`. No new deployment is created, and no rollout is triggered.
- **If the specs differ (e.g., new digest or modified env var):** A new immutable `Deployment` version is created, and the `RolloutController` begins the transition.

## 3. Control Plane Boundary

The CLI must never bypass the Control Plane to manipulate infrastructure directly.
- The CLI **must not** communicate with Agents.
- The CLI **must not** configure the Proxy or allocate Node ports.
- The CLI **only** interacts with the Control Plane's HTTP API to declare desired state (`AppSpec`) and Service routing rules.

## 4. Log Flow & Modes

### Flow Architecture
The Control Plane acts as a secure proxy to retrieve logs from the Agents.
1. `runstack logs <app-id>` hits the Control Plane.
2. The CP identifies the `ActiveDeployment` and looks up the `InstanceID`s and their assigned `NodeID`s.
3. The CP reads the `IPAddress` of the target Node from the `NodeRegistry`.
4. The CP makes an internal HTTP request to a new **Agent API** (e.g., `GET http://{Node.IPAddress}:8081/api/v1/logs?container={ContainerID}`).
5. The Agent executes `docker logs` via the `RuntimeAdapter` and returns the output to the CP, which streams it back to the CLI.

### Log Modes
To ensure bounded resource usage and prevent connection exhaustion, 8F will support the minimum useful surface:
- `runstack logs <app-id>` (fetches last 100 lines from all running instances)
- `runstack logs <app-id> --instance <id>` (fetches last 100 lines for a specific instance)

**Streaming (`--follow`) is explicitly deferred.** Streaming requires connection lifecycle management, backpressure, and Agent goroutine resource limits to prevent denial-of-service, which falls outside the minimal "first usable PaaS" scope.

## 5. Security Fencing

- **No Arbitrary Container Access:** Users cannot request logs by arbitrary container IDs. They request by `InstanceID`. The Control Plane resolves the `InstanceID` to a specific `NodeID` and `ContainerID`.
- **Cross-Application Prevention:** The Control Plane validates that the requested `InstanceID` strictly belongs to the requested `ApplicationID`.
- **Agent Enforcement:** The Agent API only accepts log requests for containers prefixed with `runstack-` and validates that the container belongs to a known active/crashed instance.
- **No Shell Injection:** The CLI constructs deployment specs via JSON and executes local docker commands via structured `os/exec` arrays, never `sh -c`.

## 6. Product Flow

**Deployment:**
```text
runstack deploy
    ↓
Local docker build & push
    ↓
CP: Validate desired spec vs active deployment
    ↓
CP: Create immutable Deployment (if changed)
    ↓
CP: RolloutController converges instances
    ↓
CP: RoutingReconciler exposes healthy instances
    ↓
CLI: Prints Service URL
```

**Observability:**
```text
runstack logs <app-name>
    ↓
CP validates App ownership
    ↓
CP identifies target Agent Node IP
    ↓
CP dials Agent API for container logs
    ↓
CLI displays securely fetched logs
```

## 7. Constraints Preserved
- No databases (purely in-memory).
- No GitOps controller (build execution remains local to the developer).
- No hidden internal registry (user provides registry).
- Existing Job semantics are strictly untouched.
- Standard Control Plane / Agent / Runtime adapter interfaces are respected.

==================================================
READY FOR 8F IMPLEMENTATION
