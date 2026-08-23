# RunStack HTTP API

All endpoints are hosted by the Control Plane, which defaults to `http://localhost:8080`.

## Health & Status

- **`GET /health`**
  Returns `{ "status": "ok" }`.

- **`GET /api/v1/status`**
  Returns `{ "status": "running" }`.

## Nodes

- **`POST /api/v1/nodes/register`**
  Registers a new agent node with its system capabilities (OS, CPU, Memory, Containers).

- **`POST /api/v1/nodes/{id}/heartbeat`**
  Agents periodically ping this to prevent being marked `OFFLINE`. Can include updated capability metrics.

- **`GET /api/v1/nodes`**
  Returns a list of all registered nodes and their statuses.

- **`GET /api/v1/nodes/{id}`**
  Returns details for a specific node.

## Jobs

- **`POST /api/v1/jobs`**
  Creates a new job. Begins in the `PENDING` state.
  ```json
  {
    "name": "my-job",
    "command": "echo hello_world"
  }
  ```

- **`GET /api/v1/jobs`**
  Lists all jobs. 
  Supports query parameters for agents polling for work:
  - `?assignedNodeId={node_id}`
  - `?status={assigned}`

- **`GET /api/v1/jobs/{id}`**
  Retrieves full job details, including timestamps, assignments, and execution results.

- **`GET /api/v1/jobs/{id}/events`**
  Returns the in-memory chronological event history for a job. Note: Events disappear if the Control Plane restarts.

- **`PATCH /api/v1/jobs/{id}`**
  Manually update job properties. (Used sparingly, largely superseded by internal registry mechanics).

- **`POST /api/v1/jobs/{id}/claim`**
  Agent endpoint to claim an `ASSIGNED` job atomically. Transitions state to `RUNNING`.
  ```json
  {
    "nodeId": "node-hostname"
  }
  ```

- **`POST /api/v1/jobs/{id}/result`**
  Agent endpoint to report execution completion. Idempotent. Transitions state to `SUCCEEDED` or `FAILED`.
  ```json
  {
    "nodeId": "node-hostname",
    "result": {
      "exitCode": 0,
      "stdout": "hello_world\n",
      "stderr": "",
      "error": ""
    }
  }
  ```
