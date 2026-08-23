package instance

import (
	"testing"
)

func TestRegistry_CreateGet(t *testing.T) {
	r := NewRegistry()
	inst, err := r.Create("app-1", "dep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inst.Status != StatusPending {
		t.Errorf("expected PENDING, got %s", inst.Status)
	}

	got, err := r.Get(inst.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != inst.ID {
		t.Errorf("expected %s, got %s", inst.ID, got.ID)
	}
}

func TestRegistry_Immutability(t *testing.T) {
	r := NewRegistry()
	inst, _ := r.Create("app-1", "dep-1")

	// Mutate returned object
	inst.ApplicationID = "hacked"
	inst.Status = StatusRunning

	// Verify internal state is unchanged
	got, _ := r.Get(inst.ID)
	if got.ApplicationID != "app-1" {
		t.Errorf("instance app ID was mutated!")
	}
	if got.Status != StatusPending {
		t.Errorf("instance status was mutated!")
	}
}

func TestRegistry_UpdateState(t *testing.T) {
	r := NewRegistry()
	inst, _ := r.Create("app-1", "dep-1")

	updated, err := r.UpdateState(inst.ID, StatusAssigned, "node-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.NodeID != "node-1" {
		t.Errorf("expected node-1, got %s", updated.NodeID)
	}
	if updated.Status != StatusAssigned {
		t.Errorf("expected ASSIGNED, got %s", updated.Status)
	}

	// Update to running, should set StartedAt
	running, _ := r.UpdateState(inst.ID, StatusRunning, "", "container-1")
	if running.StartedAt == nil {
		t.Errorf("expected StartedAt to be set")
	}

	// Update to stopped, should set StoppedAt
	stopped, _ := r.UpdateState(inst.ID, StatusStopped, "", "")
	if stopped.StoppedAt == nil {
		t.Errorf("expected StoppedAt to be set")
	}
}
