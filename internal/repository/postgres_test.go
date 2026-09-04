package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/db"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/repository"
)

func setupTestDB(t *testing.T) (*db.Database, func()) {
	url := os.Getenv("RUNSTACK_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:password@localhost:5432/runstack_test?sslmode=disable"
	}

	cfg := &db.Config{URL: url, MaxOpenConns: 5}
	database, err := db.Connect(cfg)
	if err != nil {
		t.Skipf("Skipping integration test; postgres not available: %v", err)
	}

	ctx := context.Background()
	// Run down then up to ensure clean state
	database.MigrateDown(ctx)
	if err := database.MigrateUp(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Clean tables before tests
	database.Exec("TRUNCATE applications, deployments, instances, jobs, nodes, secrets, domains, ingresses, routes RESTART IDENTITY CASCADE")

	return database, func() {
		database.Close()
	}
}

func TestPostgresRepository_NodeAuth(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewPostgresRepository(database.DB)
	ctx := context.Background()

	n := &node.Node{
		ID:            "node-1",
		Hostname:      "worker-1",
		IPAddress:     "10.0.0.1",
		CPUCores:      4,
		OS:            "linux",
		Architecture:  "amd64",
		Status:        "online",
		LastHeartbeat: time.Now(),
		Token:         "secret-token-123",
	}

	if err := repo.RegisterNode(ctx, n); err != nil {
		t.Fatalf("Failed to register node: %v", err)
	}

	fetched, err := repo.GetNodeByToken(ctx, "secret-token-123")
	if err != nil {
		t.Fatalf("Failed to get node by token: %v", err)
	}
	if fetched.ID != "node-1" {
		t.Errorf("Expected node-1, got %s", fetched.ID)
	}

	_, err = repo.GetNodeByToken(ctx, "invalid-token")
	if err != repository.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestPostgresRepository_TransactionsAndRollback(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := database.RunInTx(ctx, func(tx *sql.Tx) error {
		repo := repository.NewPostgresRepository(tx)
		n := &node.Node{
			ID:            "node-tx",
			Hostname:      "tx-worker",
			IPAddress:     "10.0.0.2",
			Status:        "online",
			Token:         "tx-token",
			LastHeartbeat: time.Now(),
			OS:            "linux",
			Architecture:  "amd64",
			CPUCores:      2,
		}
		if err := repo.RegisterNode(ctx, n); err != nil {
			return err
		}
		// Force rollback
		return errors.New("simulated failure")
	})

	if err == nil {
		t.Fatal("Expected error from simulated failure")
	}

	// Verify rollback
	repo := repository.NewPostgresRepository(database.DB)
	_, err = repo.GetNodeByToken(ctx, "tx-token")
	if err != repository.ErrNotFound {
		t.Errorf("Expected ErrNotFound (rolled back), got %v", err)
	}
}

func TestPostgresRepository_ExecutionIDFencing(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewPostgresRepository(database.DB)
	ctx := context.Background()

	// Setup dummy app and dep to satisfy foreign keys
	database.ExecContext(ctx, `INSERT INTO applications (id, name, spec, status, created_at, updated_at) VALUES ('app-1', 'app-1', '{}', 'ACTIVE', NOW(), NOW())`)
	database.ExecContext(ctx, `INSERT INTO deployments (id, application_id, version, spec_snapshot, hash, status, rollout_status, created_at) VALUES ('dep-1', 'app-1', 1, '{}', 'hash', 'ACTIVE', 'COMPLETED', NOW())`)

	inst := &instance.Instance{
		ID:            "inst-1",
		ApplicationID: "app-1",
		DeploymentID:  "dep-1",
		ExecutionID:   "exec-current",
		Status:        instance.StatusStarting,
		Health:        instance.HealthUnknown,
		CreatedAt:     time.Now(),
	}

	if err := repo.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	// 1. Stale ExecutionID should fail
	err := repo.UpdateInstanceStatusWithFencing(ctx, "inst-1", "exec-old", instance.StatusRunning)
	if err != repository.ErrStaleUpdate {
		t.Errorf("Expected ErrStaleUpdate, got %v", err)
	}

	// 2. Missing Instance should fail with ErrNotFound
	err = repo.UpdateInstanceStatusWithFencing(ctx, "inst-missing", "exec-current", instance.StatusRunning)
	if err != repository.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	// 3. Current ExecutionID should succeed
	err = repo.UpdateInstanceStatusWithFencing(ctx, "inst-1", "exec-current", instance.StatusRunning)
	if err != nil {
		t.Errorf("Expected success, got %v", err)
	}
}

func TestPostgresRepository_RowLocking(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Setup data
	database.ExecContext(ctx, `INSERT INTO applications (id, name, spec, status, created_at, updated_at) VALUES ('app-1', 'app-1', '{}', 'ACTIVE', NOW(), NOW())`)
	database.ExecContext(ctx, `INSERT INTO deployments (id, application_id, version, spec_snapshot, hash, status, rollout_status, created_at) VALUES ('dep-1', 'app-1', 1, '{}', 'hash', 'ACTIVE', 'COMPLETED', NOW())`)

	inst := &instance.Instance{
		ID:            "inst-lock",
		ApplicationID: "app-1",
		DeploymentID:  "dep-1",
		ExecutionID:   "exec-lock",
		Status:        instance.StatusPending,
		CreatedAt:     time.Now(),
	}
	repo := repository.NewPostgresRepository(database.DB)
	if err := repo.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	// Start Tx 1
	tx1, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	repo1 := repository.NewPostgresRepository(tx1)

	// Lock row in Tx 1
	_, err = repo1.LockInstanceForUpdate(ctx, "inst-lock")
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	// Try to update same row from separate connection (should block/timeout if we had a short context, but we will test by ensuring Tx1 commit allows Tx2)
	// We'll just verify the lock holds until tx1.Commit()
	done := make(chan bool)
	go func() {
		tx2, _ := database.BeginTx(context.Background(), nil)
		repo2 := repository.NewPostgresRepository(tx2)
		repo2.LockInstanceForUpdate(context.Background(), "inst-lock")
		tx2.Commit()
		done <- true
	}()

	select {
	case <-done:
		t.Fatal("Tx2 acquired lock before Tx1 committed - FOR UPDATE failed")
	case <-time.After(500 * time.Millisecond):
		// Expected: Tx2 is blocked waiting for Tx1
	}

	tx1.Commit()

	select {
	case <-done:
		// Expected: Tx2 unblocked and completed
	case <-time.After(1 * time.Second):
		t.Fatal("Tx2 did not unblock after Tx1 commit")
	}
}
