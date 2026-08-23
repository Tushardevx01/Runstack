package docker

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/runtime"
)

func isDockerAvailable() bool {
	err := exec.Command("docker", "info").Run()
	return err == nil
}

func TestDockerRuntime_IdempotentStartStopRemove(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	d := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := runtime.ContainerSpec{
		InstanceID:  "test-inst-123",
		ExecutionID: "test-exec-456",
		Image:       "alpine:latest",
		Command:     []string{"sleep", "60"}, // run a long-running process
	}

	// Clean up before test just in case
	d.Remove(ctx, containerName(spec.InstanceID))

	// 1. Start container
	info1, err := d.Start(ctx, spec)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if info1.ContainerID == "" {
		t.Fatalf("expected ContainerID")
	}
	if info1.State != runtime.StateRunning {
		t.Fatalf("expected state running, got %s", info1.State)
	}

	// 2. Start same instance again (Idempotency)
	info2, err := d.Start(ctx, spec)
	if err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	if info2.ContainerID != info1.ContainerID {
		t.Fatalf("expected same container ID, got %s vs %s", info1.ContainerID, info2.ContainerID)
	}

	// 3. Status check
	state, err := d.Status(ctx, containerName(spec.InstanceID))
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if state != runtime.StateRunning {
		t.Fatalf("expected state running, got %s", state)
	}

	// 4. Stop
	if err := d.Stop(ctx, containerName(spec.InstanceID)); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 5. Stop again (Idempotency)
	if err := d.Stop(ctx, containerName(spec.InstanceID)); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}

	// 6. Status check after stop
	state, err = d.Status(ctx, containerName(spec.InstanceID))
	if err != nil {
		t.Fatalf("Status failed after stop: %v", err)
	}
	if state != runtime.StateExited {
		t.Fatalf("expected state exited, got %s", state)
	}

	// 7. Remove
	if err := d.Remove(ctx, containerName(spec.InstanceID)); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// 8. Remove again (Idempotency)
	if err := d.Remove(ctx, containerName(spec.InstanceID)); err != nil {
		t.Fatalf("second Remove failed: %v", err)
	}

	// 9. Status check after remove
	_, err = d.Status(ctx, containerName(spec.InstanceID))
	if err == nil {
		t.Fatalf("expected error querying removed container")
	}
	if err != runtime.ErrContainerNotFound {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestDockerRuntime_IdentityConflict(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	d := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec1 := runtime.ContainerSpec{
		InstanceID:  "test-conflict-abc",
		ExecutionID: "test-exec-1",
		Image:       "alpine:latest",
		Command:     []string{"sleep", "5"},
	}

	d.Remove(ctx, containerName(spec1.InstanceID))
	defer d.Remove(ctx, containerName(spec1.InstanceID))

	_, err := d.Start(ctx, spec1)
	if err != nil {
		t.Fatalf("Start 1 failed: %v", err)
	}

	// Now try to start the same instance but with a different ExecutionID
	spec2 := spec1
	spec2.ExecutionID = "test-exec-2"

	_, err = d.Start(ctx, spec2)
	if err == nil {
		t.Fatalf("expected ErrContainerConflict, got nil")
	}
	if err != runtime.ErrContainerConflict {
		t.Fatalf("expected ErrContainerConflict, got %v", err)
	}
}

func TestDockerRuntime_InvalidSpec(t *testing.T) {
	d := New()
	ctx := context.Background()

	// Missing Image
	spec := runtime.ContainerSpec{
		InstanceID:  "inst-1",
		ExecutionID: "exec-1",
	}

	_, err := d.Start(ctx, spec)
	if err != runtime.ErrInvalidSpec {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}
}

func TestDockerRuntime_CommandSafety(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration test")
	}

	d := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use an Alpine image and try to create a file with a shell metacharacter injection
	// If it was interpreted by a shell, it would execute touch /tmp/pwned
	// Since it's passed directly to exec, the echo command will literally print "hello; touch /tmp/pwned"
	spec := runtime.ContainerSpec{
		InstanceID:  "test-security-abc",
		ExecutionID: "test-exec-sec",
		Image:       "alpine:latest",
		// This should NOT result in a file being created, because echo is run directly
		Command: []string{"echo", "hello;", "touch", "/tmp/pwned"},
	}

	d.Remove(ctx, containerName(spec.InstanceID))
	defer d.Remove(ctx, containerName(spec.InstanceID))

	_, err := d.Start(ctx, spec)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait a moment for it to exit
	time.Sleep(2 * time.Second)

	// Verify the container exited cleanly
	state, err := d.Status(ctx, containerName(spec.InstanceID))
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if state != runtime.StateExited {
		t.Fatalf("expected state exited, got %s", state)
	}
}
