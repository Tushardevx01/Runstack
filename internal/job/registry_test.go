package job

import (
	"sync"
	"testing"
)

func TestJob_GenerateID(t *testing.T) {
	id1 := GenerateID()
	id2 := GenerateID()
	if id1 == id2 {
		t.Errorf("expected unique IDs")
	}
}

func TestJob_Transitions(t *testing.T) {
	if !isValidTransition(StatusPending, StatusAssigned) {
		t.Errorf("expected PENDING -> ASSIGNED to be valid")
	}
	if isValidTransition(StatusPending, StatusRunning) {
		t.Errorf("expected PENDING -> RUNNING to be invalid")
	}
}

func TestRegistry_Create(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test-job", "echo test")
	if j.Name != "test-job" || j.Status != StatusPending {
		t.Errorf("unexpected job creation state")
	}
}

func TestRegistry_Update(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test-job", "echo test")

	newStatus := StatusAssigned
	nodeID := "node-1"

	updated, err := r.Update(j.ID, UpdateParams{
		Status:         &newStatus,
		AssignedNodeID: &nodeID,
	})

	if err != nil {
		t.Fatal(err)
	}

	if updated.Status != StatusAssigned || updated.AssignedNodeID != "node-1" {
		t.Errorf("unexpected updated state: %+v", updated)
	}

	invalidStatus := StatusSucceeded
	_, err = r.Update(j.ID, UpdateParams{
		Status: &invalidStatus,
	})
	if err != ErrInvalidTransition {
		t.Errorf("expected invalid transition error, got %v", err)
	}
}

func TestRegistry_Concurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j := r.Create("concurrent-job", "echo")

			s := StatusAssigned
			r.Update(j.ID, UpdateParams{Status: &s})

			r.Get(j.ID)
			r.List()
		}()
	}
	wg.Wait()
	if len(r.List()) != 100 {
		t.Errorf("expected 100 jobs")
	}
}
