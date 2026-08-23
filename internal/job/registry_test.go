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

func TestRegistry_Claim(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo 1")

	// Cannot claim PENDING job
	_, err := r.Claim(j.ID, "node-1")
	if err != ErrInvalidTransition {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Update to ASSIGNED
	status := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &status, AssignedNodeID: &nodeID})

	// Claim with wrong node
	_, err = r.Claim(j.ID, "node-2")
	if err == nil {
		t.Errorf("expected error when claiming with wrong node")
	}

	// Successful claim
	claimed, err := r.Claim(j.ID, "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimed.Status != StatusRunning || claimed.StartedAt == nil {
		t.Errorf("expected status RUNNING and StartedAt set")
	}
}

func TestRegistry_ReportResult(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo 1")

	status := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &status, AssignedNodeID: &nodeID})
	r.Claim(j.ID, "node-1")

	res := JobResult{ExitCode: 0, Stdout: "done"}

	// Report with wrong node
	_, err := r.ReportResult(j.ID, "node-2", res)
	if err == nil {
		t.Errorf("expected error when reporting with wrong node")
	}

	// Successful report
	finished, err := r.ReportResult(j.ID, "node-1", res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finished.Status != StatusSucceeded || finished.CompletedAt == nil || finished.Result.Stdout != "done" {
		t.Errorf("expected status SUCCEEDED and Result set, got %+v", finished)
	}

	// Failure report
	j2 := r.Create("fail", "exit 1")
	r.Update(j2.ID, UpdateParams{Status: &status, AssignedNodeID: &nodeID})
	r.Claim(j2.ID, "node-1")

	failed, _ := r.ReportResult(j2.ID, "node-1", JobResult{ExitCode: 1, Error: "fail"})
	if failed.Status != StatusFailed {
		t.Errorf("expected status FAILED")
	}
}
