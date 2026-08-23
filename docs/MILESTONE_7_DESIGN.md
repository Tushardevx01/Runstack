# Milestone 7: Application Model & Deployment Specification

This document details the shift of RunStack from a batch-job execution engine to a long-running, self-hosted PaaS.

**Core Principle:** Deployment is immutable. Instance is runtime/observed state. Application is desired state. Jobs remain separate from long-running Instances. Container runtime integration comes later.

## 1. Domain Separation

RunStack will implement a strict separation of concerns for PaaS workloads:

*   **Application:** "WHAT should exist?" (Desired State)
*   **Deployment:** "WHAT exact version should run?" (Immutable Snapshot)
*   **Instance:** "WHAT is actually running on THIS node?" (Runtime/Observed State)

This model explicitly separates the desired configuration from the runtime reality, enabling robust reconciliation. **This system must be kept entirely separate from the existing Job system**, which remains focused on short-lived execution tasks.

## 2. Entity Specifications

### `Application` (Desired State)
Represents the user's intent. Changes to this entity trigger new Deployments.
*   **ID** (`string`): Unique UUID
*   **Name** (`string`): Unique, human-readable name (e.g., `api`)
*   **Spec** (`AppSpec`): The desired configuration.
    *   **Image** (`string`)
    *   **Command** (`[]string`)
    *   **Args** (`[]string`)
    *   **Environment** (`map[string]string`)
    *   **Ports** (`[]PortMapping`)
    *   **Replicas** (`int`)
*   **ActiveDeploymentID** (`string`): Pointer to the currently desired deployment.
*   **Status** (`AppStatus`): Aggregate state.
*   **CreatedAt**, **UpdatedAt**

*(Note: Advanced features like autoscaling, probes, rolling strategies, volumes, secrets, and quotas are explicitly excluded from V1).*

### `Deployment` (Immutable Version/Snapshot)
A read-only, point-in-time snapshot of the `ApplicationSpec`. **Once created, a Deployment is never mutated.** Any change to the Application results in a completely new Deployment record.
*   **ID** (`string`): Unique UUID
*   **ApplicationID** (`string`): Reference to parent.
*   **Version** (`int`): Sequential rollout number (`v1`, `v2`, etc.).
*   **SpecSnapshot** (`AppSpec`): Complete, immutable copy of the configuration.
*   **Status** (`DeploymentStatus`): E.g., `ROLLING_OUT`, `SUCCESS`, `FAILED`.
*   **CreatedAt**

### `Instance` (Observed/Runtime State)
The control-plane representation of a desired or running replica. It maps a `Deployment` to a specific `Node`. **It is not a raw Docker container abstraction.**
*   **ID** (`string`): Unique UUID
*   **ApplicationID** (`string`): Reference to parent Application.
*   **DeploymentID** (`string`): Reference to parent Deployment (giving the instance its immutable identity/configuration).
*   **NodeID** (`string`): Assigned execution node.
*   **Status** (`InstanceStatus`): State machine tracking (`PENDING`, `ASSIGNED`, `STARTING`, `RUNNING`, `CRASHED`, `STOPPED`).
*   **ContainerID** (`string`): ID returned by the agent (future).
*   **Health** (`string`)
*   **CreatedAt**, **StartedAt**, **StoppedAt**

## 3. The Architecture

```text
                        RunStack PaaS
                             │
                 ┌───────────┴───────────┐
                 │                       │
          Application Layer        Infrastructure
                 │                       │
                 ▼                       ▼
           Application             Node Registry
                 │                       │
                 ▼                       │
            Deployment                  Node
                 │
                 ▼
              Instance
                 │
                 ▼
              Scheduler
                 │
                 ▼
               Agent
                 │
                 ▼
          Docker / Podman
```

**Domain Relationship:**
```text
Application 1 ──────┬──── Deployment 1 ──── Instances
                    ├──── Deployment 2 ──── Instances
                    └──── Deployment 3 ──── Instances
```

## 4. Implementation Boundary (Milestone 7)

Milestone 7 is strictly scoped to **domain modeling and control-plane state**.
*   **NO** container execution.
*   **NO** Docker or Podman API calls.
*   **NO** agent-side container orchestration.

The goal is to implement:
1.  Application CRUD APIs and registries.
2.  Deployment creation logic (immutability enforcement).
3.  Instance generation (desired state).
4.  Instance scheduling/assignment to Nodes.

Milestone 8 will introduce the Agent and Container Runtime layers.
