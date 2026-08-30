# Milestone 8K: Authentication & Remote CLI Contexts

## 1. Primary Goal
Make RunStack safe to operate over a remote network. The Control Plane API is currently trusted by network reachability, exposing the cluster to Remote Code Execution (RCE) if bound to a public IP. 8K implements static Bearer token authentication to ensure only authenticated operators and legitimate Agents can perform authorized actions.

## 2. Authentication Model
RunStack will use a **Static Bearer Token** model, strictly adhering to the in-memory, no-database V1 constraints. 
*   **Tokens:** High-entropy cryptographic strings (e.g., 32-byte hex).
*   **Transport:** Passed via HTTP header: `Authorization: Bearer <token>`.
*   **Volatility:** Since the Control Plane (CP) is stateless, valid tokens must be injected at CP startup (e.g., via ENV vars `RUNSTACK_OPERATOR_TOKEN` and `RUNSTACK_AGENT_TOKEN`, or a CP config file). If the CP restarts, the same tokens must be provided to retain CLI/Agent connectivity.

## 3. Trust Model & Roles
Do not implement full RBAC. Implement a strict, disjoint two-role trust model:
1.  **USER / OPERATOR:** Has administrative control over Applications, Deployments, Secrets, Routes, and Logs.
2.  **AGENT:** Strictly limited to node lifecycle and workload execution reporting.

An Agent token **MUST NOT** authorize Operator API actions. An Operator token **MUST NOT** authorize Agent API actions (to prevent operators from accidentally spoofing node state).

### Role API Boundary
*   **USER / OPERATOR endpoints:** `/api/v1/apps*`, `/api/v1/deployments*`, `/api/v1/routes*`, `/api/v1/secrets*`, `/api/v1/nodes` (read-only list).
*   **AGENT endpoints:** `/api/v1/nodes/register`, `/api/v1/nodes/*/heartbeat`, `/api/v1/instances/*/claim`, `/api/v1/instances/*/status`, `/api/v1/jobs/*/claim`, `/api/v1/jobs/*/result`.

## 4. HTTP Status Semantics
*   **401 Unauthorized:** Missing token, malformed `Authorization` header, or invalid token.
*   **403 Forbidden:** Valid identity but disallowed operation (e.g., Agent calling `/api/v1/apps`).
*   **404/409:** Existing domain semantics are preserved.

## 5. Authentication Middleware
Authentication is centralized via HTTP middleware wrapping the API router.
*   **Architecture:** `HTTP Request → Auth Middleware → Context Injection (Identity) → Handler → Domain Logic`.
*   The middleware checks the bearer token against the CP's configured Operator/Agent tokens and rejects unauthorized requests immediately. Handlers assume requests are authenticated but must still enforce ownership.

## 6. Resource & Agent Ownership (Auth ≠ Ownership)
Authentication proves *who* is calling. It does not replace ownership checks.
*   **Application Ownership:** Existing checks (`Application → Deployment → Instance → Secret`) remain. A valid operator token simply permits the request to reach the handler.
*   **Agent Identity Binding:** When an Agent token is used for an endpoint (e.g., `ReportInstanceStatus`), the CP must validate that the `NodeID` in the request matches the node to which the `InstanceID` is actually assigned. An Agent cannot claim or report on another Agent's instances.

## 7. CLI Contexts
The CLI will support remote CP URLs and manage tokens securely via a config file.
*   **File:** `~/.runstack/config` (YAML or JSON).
*   **Structure:** Supports multiple named contexts, storing `name`, `endpoint`, and `token`.
*   **Commands:**
    *   `runstack context list`
    *   `runstack context add <name> --endpoint <url> --token <token>`
    *   `runstack context use <name>`
*   Commands like `runstack deploy` or `runstack logs` will transparently resolve the endpoint and token from the active context.

## 8. Config File Security & Token Handling
*   **Permissions:** `~/.runstack/config` MUST be created with `0600` (owner read/write only).
*   **No Echoing:** Tokens are never printed in console output, diagnostic logs, or error messages.
*   **Generation:** `runstack auth token create` will generate secure random tokens.
*   **Missing Config:** The CLI gracefully returns a clear error if the config is missing or the context is unset, prompting the user to run `runstack context add`.

## 9. Remote Control Plane
The CLI and Agent must stop defaulting exclusively to `http://localhost:8080`.
*   Support for `https://` schemes.
*   Agent configuration uses `RUNSTACK_CP_URL`, `RUNSTACK_AGENT_TOKEN`, and `RUNSTACK_NODE_ID` via Environment Variables to avoid leaking secrets in `ps` output or Docker labels.

## 10. Centralized API Client
The internal API client (used by CLI and Agent) will be refactored to abstract HTTP requests, automatically injecting the `Authorization: Bearer <token>` header, preserving existing timeouts, retries, and context cancellations, and handling `401`/`403` responses clearly.

## 11. Security Threat Model Handled
*   **Token Replay / MITM:** Remote deployments must use HTTPS (enforced by reverse proxies sitting in front of the CP).
*   **Cross-role Usage:** Middleware strictly separates Agent/Operator tokens.
*   **Agent Spoofing:** CP validates NodeID bindings for instance/job assignments.
*   **Log/Secret Exposure:** Handlers enforce application ownership; Auth doesn't weaken this.
*   **Default Bypass:** No unauthenticated fallbacks. Missing token always yields `401`. Local development mode (e.g., running CP with a `--dev-insecure` flag) must be explicitly opt-in and emit prominent warnings.

## 12. Backward Compatibility
This is a breaking security change. Old CLIs and Agents making unauthenticated requests will immediately fail with `401 Unauthorized`. This is deliberate.

## 13. Test Strategy
*   **Auth Middleware:** Unit tests for missing token, invalid token, valid Operator token, valid Agent token.
*   **Cross-Role Rejection:** Agent token accessing Operator endpoint (`403`); Operator token accessing Agent endpoint (`403`).
*   **Agent Impersonation:** Agent A attempting to update Agent B's instance (`403` or `409/conflict`).
*   **Config Security:** Verify `0600` creation of `~/.runstack/config`. Verify context switching.
*   **Race Conditions:** Concurrent token validation and authenticated requests.
