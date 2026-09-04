CREATE TABLE nodes (
    id VARCHAR(255) PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL,
    ip_address VARCHAR(255) NOT NULL,
    cpu_cores INT NOT NULL,
    os VARCHAR(255) NOT NULL,
    architecture VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    last_heartbeat TIMESTAMP WITH TIME ZONE NOT NULL,
    offline_since TIMESTAMP WITH TIME ZONE,
    capabilities JSONB NOT NULL,
    token VARCHAR(255) NOT NULL
);

CREATE UNIQUE INDEX idx_nodes_token ON nodes(token);

CREATE TABLE applications (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    spec JSONB NOT NULL,
    status VARCHAR(50) NOT NULL,
    active_deployment_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE deployments (
    id VARCHAR(255) PRIMARY KEY,
    application_id VARCHAR(255) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    version INT NOT NULL,
    spec_snapshot JSONB NOT NULL,
    hash VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    rollout_status VARCHAR(50) NOT NULL,
    desired_replicas INT NOT NULL DEFAULT 0,
    updated_replicas INT NOT NULL DEFAULT 0,
    ready_replicas INT NOT NULL DEFAULT 0,
    unavailable_replicas INT NOT NULL DEFAULT 0,
    blocked_reason TEXT,
    consecutive_crashes INT NOT NULL DEFAULT 0,
    degraded BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE instances (
    id VARCHAR(255) PRIMARY KEY,
    application_id VARCHAR(255) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    deployment_id VARCHAR(255) NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    node_id VARCHAR(255) REFERENCES nodes(id) ON DELETE SET NULL,
    execution_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    health VARCHAR(50) NOT NULL,
    draining BOOLEAN NOT NULL DEFAULT FALSE,
    drain_started_at TIMESTAMP WITH TIME ZONE,
    container_id VARCHAR(255),
    ports JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE,
    stopped_at TIMESTAMP WITH TIME ZONE,
    unknown_since TIMESTAMP WITH TIME ZONE
);

CREATE TABLE jobs (
    id VARCHAR(255) PRIMARY KEY,
    application_id VARCHAR(255) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    spec JSONB NOT NULL,
    status VARCHAR(50) NOT NULL,
    assigned_node_id VARCHAR(255) REFERENCES nodes(id) ON DELETE SET NULL,
    execution_id VARCHAR(255) NOT NULL,
    result JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    error TEXT
);

CREATE TABLE secrets (
    id VARCHAR(255) PRIMARY KEY,
    application_id VARCHAR(255) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    value TEXT NOT NULL, -- At-rest encryption deferred
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    UNIQUE (application_id, name)
);

CREATE TABLE domains (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    application_id VARCHAR(255) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    tls BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(50) NOT NULL
);

CREATE TABLE ingresses (
    id VARCHAR(255) PRIMARY KEY,
    domain_id VARCHAR(255) NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    service_id VARCHAR(255) NOT NULL,
    path VARCHAR(255) NOT NULL DEFAULT '/'
);

CREATE TABLE routes (
    id VARCHAR(255) PRIMARY KEY,
    application_id VARCHAR(255) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    target_port INT NOT NULL,
    protocol VARCHAR(10) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Note: ActiveDeploymentID FK is added after Deployments are created
ALTER TABLE applications ADD CONSTRAINT fk_applications_active_deployment 
    FOREIGN KEY (active_deployment_id) REFERENCES deployments(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED;
