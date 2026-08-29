# Milestone 8I: Application Health Probes & Readiness (Revised)

## Motivation
Currently, instances report `instance.HealthUnknown` which breaks routing (since `RoutingReconciler` strictly routes to `HEALTHY`). It also means rollouts proceed based purely on process liveness rather than actual application readiness. Milestone 8I introduces dedicated Health Probes (Readiness and Liveness) to safely gate traffic and manage container lifecycles.

## 1. Separate Readiness and Liveness
We explicitly separate two concerns:
- **Readiness:** Determines whether the instance may receive traffic. A failed readiness probe changes Health to `UNHEALTHY` (removing it from routing) but does **NOT** terminate the container. It can recover back to `HEALTHY`.
- **Liveness:** Determines whether the running process is stuck and should be replaced. A failed liveness probe changes Status to `CRASHED` (terminating the container) and increments the deployment's `ConsecutiveCrashes` circuit breaker.

## 2. AppSpec Configuration
`AppSpec` will include optional `ReadinessProbe` and `LivenessProbe` configurations:
```go
type Probe struct {
    Type             string // "HTTP" or "TCP"
    Path             string // e.g. "/healthz" (for HTTP only)
    Port             int    // Container internal port to probe
    InitialDelaySecs int    // Wait time before first probe
    PeriodSecs       int    // Interval between probes
    TimeoutSecs      int    // Timeout per probe attempt
    SuccessThreshold int    // Consecutive successes required
    FailureThreshold int    // Consecutive failures required
}
```

### No-Probe Default
If no `ReadinessProbe` is configured, an instance transitions from `UNKNOWN` to `HEALTHY` immediately upon entering `RUNNING`.
If no `LivenessProbe` is configured, the container relies purely on process liveness (exit code).

## 3. Startup & Health State Machine
**Status** (what the process is doing) and **Health** (whether it is usable) remain independent.
- **Startup:** When the container starts, it enters `Status = RUNNING` and `Health = UNKNOWN`. During `InitialDelaySecs`, it remains `UNKNOWN` and is **not routable**.
- **Healthy:** After `SuccessThreshold` *consecutive* successful readiness probes, `Health` becomes `HEALTHY`.
- **Unhealthy:** After `FailureThreshold` *consecutive* failed readiness probes, `Health` becomes `UNHEALTHY`. A single success resets the failure counter, and vice versa.
- **Liveness Failure:** After `FailureThreshold` consecutive failed liveness probes, the Agent kills the container. `Status` becomes `CRASHED`, and `Health` becomes `UNKNOWN`.

## 4. Probe Safety & Semantics
- **Target Safety:** The Agent strictly probes `127.0.0.1:<MappedHostPort>`. It translates the configured internal `Port` to the actual allocated host port for that specific instance. No arbitrary external URLs or cross-container network probing is permitted (prevents SSRF/scanning).
- **HTTP Probe:** Performs an HTTP GET. Success is `200 <= status < 400`. Timeouts or connection refused count as failures. Redirects will not be followed externally.
- **TCP Probe:** Attempts a TCP connection to the host port. Connection established = success.
- **Timeout:** Each attempt uses an explicit `context.WithTimeout` bounded by `TimeoutSecs`.

## 5. System Integration
- **Routing:** `RoutingReconciler` continues to route strictly to `RUNNING + HEALTHY + !Draining`. Readiness failures seamlessly remove endpoints.
- **Rollouts:** `InstanceReconciler` waits for new instances to become `HEALTHY` before draining old instances. A deployment stuck in `UNHEALTHY` simply halts the rollout without crashing (unless liveness also fails).
- **Crash-Loop (8C):** Only `CRASHED` status increments `ConsecutiveCrashes`. Readiness failures (`UNHEALTHY`) do not trigger the circuit breaker.
- **Fencing:** Health updates use `InstanceID + NodeID + ExecutionID`. Stale executions cannot falsely mark an instance `HEALTHY`.

## 6. Agent Implementation Safety
- One bounded prober goroutine per instance running on the Agent.
- Goroutines are strictly cancelled via context when the instance stops, crashes, or is removed.
- Prevents goroutine leaks and overlapping probe loops.

### Known Limitations
- **Agent Restart Probes:** Upon restart, the Agent does not recreate in-memory health probe loops for existing `RUNNING` instances due to lack of persisted `AppSpec`.
