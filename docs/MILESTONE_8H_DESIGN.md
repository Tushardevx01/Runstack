# Milestone 8H Design: Secrets Management

## 1. Overview
Allow production applications to reference secrets without storing plaintext values in `runstack.json`, immutable Deployment snapshots, or version control. 

## 2. Core Architecture
- **Secret Domain:** A first-class `Secret` entity exists in the Control Plane memory.
- **Scoping:** Secrets are **Application-scoped**. A secret created for Application A cannot be referenced by Application B.
- **Immutability:** Secrets are separated from `AppSpec`. `AppSpec.Environment` stores *references* (e.g., `secret:db-password`), not plaintext.
- **Resolution:** Secrets are resolved precisely at the moment the Agent claims an instance. The Control Plane replaces the references with plaintext values in the `AppSpec` payload sent to the Agent, which immediately passes them to the Container Runtime.
- **V1 Constraint:** Secrets are held in plaintext **only in process memory**. No database, no persistent storage, no distributed keystore. If the Control Plane restarts, all secrets are lost and must be recreated before the next deployment.

## 3. Secret Entity
```go
type Secret struct {
    ID            string    `json:"id"`
    Name          string    `json:"name"`
    ApplicationID string    `json:"application_id"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
    // Plaintext is kept in memory but NEVER serialized to JSON.
}
```

## 4. API Definition
*   `POST /api/v1/secrets`
    *   Payload: `{"name": "...", "application_id": "...", "value": "..."}`
    *   Response: Returns the `Secret` struct *without* the `value` field.
*   `GET /api/v1/secrets?application_id=...`
    *   Response: Array of `Secret` structs (metadata only).
*   `DELETE /api/v1/secrets/{id}`

*No endpoint is provided to retrieve the plaintext value.*

## 5. CLI Commands
*   `runstack secret set <app_id> <name> <value>`
*   `runstack secret ls <app_id>`
*   `runstack secret rm <id>`
*(Values provided via arguments for V1. In future, stdin can be supported to bypass shell history).*

## 6. Deployment & Resolution Flow
1. **Creation:** User creates secret via `runstack secret set`.
2. **Configuration:** `runstack.json` specifies `"DB_PASS": "secret:db-password"`.
3. **Deployment:** `runstack deploy` sends the reference. The Control Plane verifies the secret exists and belongs to the App. It stores the *reference* in the immutable Deployment snapshot.
4. **Claim:** The Agent claims an Instance.
5. **Resolution:** The Control Plane intercepts the `AppSpec` being returned to the Agent, scans `Environment`, and replaces `secret:db-password` with the plaintext value.
6. **Execution:** The Agent passes the environment map directly to the Docker Runtime via `-e`.
7. **Failure:** If a referenced secret is missing during Claim (e.g. it was deleted), the Claim is rejected, and the Instance transitions to `CRASHED`.

## 7. Versioning & Immutability
- Secrets are overwritten in place in memory.
- Updating a secret **does not** retroactively mutate existing running instances.
- To apply a new secret value, a developer must explicitly trigger `runstack deploy`. No automatic rolling restart is performed.

## 8. Safety & Trust Boundary
- **V1 Trust Model:** The Control Plane is unauthenticated. Any operator with network access to `:8080` is trusted to deploy applications and manage secrets.
- **Log Safety:** `runstack logs` reads stdout/stderr. Environment variables are not logged by the platform. (If the application prints its own environment, it will be visible).
- **Docker Metadata:** Plaintext secrets are injected via Docker `-e` arguments. They are *not* added as Docker labels. Note: `docker inspect` will expose the container environment; this is an accepted Docker constraint.
- **Serialization:** Plaintext values are isolated in the registry and injected just-in-time. They are strictly excluded from API responses and Event histories.
