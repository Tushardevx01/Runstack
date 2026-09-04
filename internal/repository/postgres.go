package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Tushardevx01/runstack/internal/db"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
)

var (
	ErrNotFound    = errors.New("record not found")
	ErrStaleUpdate = errors.New("stale execution id update rejected")
)

type PostgresRepository struct {
	q db.Queryer
}

func NewPostgresRepository(q db.Queryer) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) GetNodeByToken(ctx context.Context, token string) (*node.Node, error) {
	row := r.q.QueryRowContext(ctx, "SELECT id, hostname, ip_address, status, token FROM nodes WHERE token = $1", token)

	var n node.Node
	err := row.Scan(&n.ID, &n.Hostname, &n.IPAddress, &n.Status, &n.Token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &n, nil
}

func (r *PostgresRepository) RegisterNode(ctx context.Context, n *node.Node) error {
	capsJSON, _ := json.Marshal(n.Capabilities)
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO nodes (id, hostname, ip_address, cpu_cores, os, architecture, status, last_heartbeat, capabilities, token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			ip_address = EXCLUDED.ip_address,
			status = EXCLUDED.status,
			last_heartbeat = EXCLUDED.last_heartbeat,
			capabilities = EXCLUDED.capabilities
	`, n.ID, n.Hostname, n.IPAddress, n.CPUCores, n.OS, n.Architecture, n.Status, n.LastHeartbeat, capsJSON, n.Token)
	return err
}

func (r *PostgresRepository) UpdateInstanceStatusWithFencing(ctx context.Context, id, executionID string, status instance.InstanceStatus) error {
	res, err := r.q.ExecContext(ctx, `
		UPDATE instances 
		SET status = $1 
		WHERE id = $2 AND execution_id = $3
	`, status, id, executionID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		// Differentiate between Missing Instance vs Stale Execution
		var dummy string
		err := r.q.QueryRowContext(ctx, "SELECT id FROM instances WHERE id = $1", id).Scan(&dummy)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return ErrStaleUpdate
	}
	return nil
}

func (r *PostgresRepository) CreateInstance(ctx context.Context, inst *instance.Instance) error {
	portsJSON, _ := json.Marshal(inst.Ports)
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO instances (id, application_id, deployment_id, node_id, execution_id, status, health, draining, container_id, ports, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, inst.ID, inst.ApplicationID, inst.DeploymentID, sql.NullString{String: inst.NodeID, Valid: inst.NodeID != ""}, inst.ExecutionID, inst.Status, inst.Health, inst.Draining, sql.NullString{String: inst.ContainerID, Valid: inst.ContainerID != ""}, portsJSON, inst.CreatedAt)
	return err
}

// LockInstanceForUpdate demonstrates Explicit Row Locking (SELECT ... FOR UPDATE).
func (r *PostgresRepository) LockInstanceForUpdate(ctx context.Context, id string) (*instance.Instance, error) {
	row := r.q.QueryRowContext(ctx, "SELECT id, status, execution_id FROM instances WHERE id = $1 FOR UPDATE", id)

	var inst instance.Instance
	err := row.Scan(&inst.ID, &inst.Status, &inst.ExecutionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &inst, nil
}
