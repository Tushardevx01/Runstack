# RunStack HTTP API

All endpoints are hosted by the Control Plane, which defaults to `http://localhost:8080`.

## Health & Status

- **`GET /health`**
  Returns `{ "status": "ok" }`.

- **`GET /api/v1/status`**
  Returns `{ "status": "running" }`.

## Nodes

- **`POST /api/v1/nodes/register`**
  Registers a new agent node with its system capabilities.

- **`POST /api/v1/nodes/{id}/heartbeat`**
  Agents periodically ping this to prevent being marked `OFFLINE`.

- **`GET /api/v1/nodes`**
  Returns a list of all registered nodes and their statuses.

- **`GET /api/v1/nodes/{id}`**
  Returns details for a specific node.

## Jobs

- **`POST /api/v1/jobs`**
  Creates a new job. Begins in the `PENDING` state.

- **`GET /api/v1/jobs`**
  Lists all jobs. 

- **`GET /api/v1/jobs/{id}`**
  Retrieves full job details.

- **`GET /api/v1/jobs/{id}/events`**
  Returns chronological event history for a job.

- **`POST /api/v1/jobs/{id}/claim`**
  Agent endpoint to claim an assigned job.

- **`POST /api/v1/jobs/{id}/result`**
  Agent endpoint to report execution completion.

## Applications & Deployments

- **`POST /api/v1/apps`**
  Creates a new Application.

- **`POST /api/v1/apps/{id}/deploy`**
  Builds, pushes, and updates an Application's active deployment via immutable image digests.

- **`POST /api/v1/apps/{id}/rollback`**
  Reverts the active deployment to the previous state.

- **`GET /api/v1/apps/{id}/logs`**
  Proxies bounded logs from the active instances securely.

## Instances

- **`GET /api/v1/instances`**
  List instances (supports `node_id` and `status` query params).

- **`POST /api/v1/instances/{id}/claim`**
  Agent claim an assigned instance.

- **`POST /api/v1/instances/{id}/status`**
  Agent pushes runtime state and health.

## Service & Routing (Milestone 8G)

- **`POST /api/v1/domains`**
  Register a new custom domain owned by a specific Application.

- **`GET /api/v1/domains`**
  List registered custom domains (supports `application_id` query param).

- **`DELETE /api/v1/domains/{id}`**
  Delete a custom domain.

- **`POST /api/v1/services`**
  Create a new internal routing service mapping an Application to a target port.

- **`GET /api/v1/services`**
  List all routing services.

- **`PUT /api/v1/services/{id}`**
  Update an existing routing service's target port.

- **`DELETE /api/v1/services/{id}`**
  Delete a routing service.

- **`POST /api/v1/ingresses`**
  Create an Ingress mapping a Domain to a Service (enforces Application ownership).

- **`GET /api/v1/ingresses`**
  List registered Ingress mappings.

- **`DELETE /api/v1/ingresses/{id}`**
  Delete an Ingress mapping.

## Secrets (Milestone 8H - Upcoming)

- **`POST /api/v1/secrets`**
  Create a new secret (metadata returned, plaintext value safely consumed but not returned).

- **`GET /api/v1/secrets`**
  List application secrets (metadata only, no values).

- **`DELETE /api/v1/secrets/{id}`**
  Delete a secret.

