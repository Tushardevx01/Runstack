package job

import (
	"fmt"
	"sync"
	"testing"
	"time"
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

func TestRegistry_EventHistory(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test-events", "echo 1")

	events, err := r.GetEvents(j.ID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(events) != 1 || events[0].Type != EventCreated {
		t.Fatalf("expected 1 CREATED event, got %v", events)
	}

	// 1. Assignment
	statusAssigned := StatusAssigned
	nodeID := "node-xyz"
	_, _ = r.Update(j.ID, UpdateParams{
		Status:         &statusAssigned,
		AssignedNodeID: &nodeID,
	})

	events, _ = r.GetEvents(j.ID)
	if len(events) != 2 || events[1].Type != EventAssigned {
		t.Fatalf("expected ASSIGNED event, got %v", events)
	}
	if events[1].From != StatusPending || events[1].To != StatusAssigned || events[1].NodeID != nodeID {
		t.Fatalf("invalid ASSIGNED event contents: %v", events[1])
	}

	// 2. Claim
	_, _ = r.Claim(j.ID, nodeID)
	events, _ = r.GetEvents(j.ID)
	if len(events) != 3 || events[2].Type != EventClaimed {
		t.Fatalf("expected CLAIMED event, got %v", events)
	}

	// 3. Success
	_, _ = r.ReportResult(j.ID, nodeID, JobResult{ExitCode: 0, Stdout: "done"})
	events, _ = r.GetEvents(j.ID)
	if len(events) != 4 || events[3].Type != EventSucceeded {
		t.Fatalf("expected SUCCEEDED event, got %v", events)
	}

	// 4. Idempotent result should NOT duplicate event
	_, _ = r.ReportResult(j.ID, nodeID, JobResult{ExitCode: 0, Stdout: "done"})
	events, _ = r.GetEvents(j.ID)
	if len(events) != 4 {
		t.Fatalf("expected exactly 4 events after idempotent result, got %d", len(events))
	}

	// 5. Immutability
	events[0].Type = EventFailed // Mutate caller copy
	eventsAgain, _ := r.GetEvents(j.ID)
	if eventsAgain[0].Type == EventFailed {
		t.Fatalf("registry event history was mutated by caller")
	}
}

func TestRegistry_EventHistory_Failure(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test-events", "echo 1")

	statusAssigned := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	r.Claim(j.ID, nodeID)

	r.ReportResult(j.ID, nodeID, JobResult{ExitCode: 1, Stdout: "fail"})

	events, _ := r.GetEvents(j.ID)
	if len(events) != 4 || events[3].Type != EventFailed {
		t.Fatalf("expected FAILED event, got %v", events)
	}
}

func TestRegistry_RecoverExecutionTimeouts(t *testing.T) {
	r := NewRegistry()
	j1 := r.Create("job1", "echo 1")
	j2 := r.Create("job2", "echo 2")
	j3 := r.Create("job3", "echo 3")

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j1.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	r.Update(j2.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	r.Update(j3.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	r.Claim(j1.ID, nodeID)
	r.Claim(j2.ID, nodeID)
	r.Claim(j3.ID, nodeID)

	// Make j1 very stale
	staleTime := time.Now().UTC().Add(-60 * time.Second)
	r.mu.Lock()
	r.jobs[j1.ID].StartedAt = &staleTime
	r.mu.Unlock()

	// j2 is fresh, StartedAt is now (default from Claim)
	// j3 will be completed
	r.ReportResult(j3.ID, nodeID, JobResult{ExitCode: 0, Stdout: "done"})

	recovered := r.RecoverExecutionTimeouts(30 * time.Second)
	if recovered != 1 {
		t.Fatalf("expected 1 job recovered, got %d", recovered)
	}

	// j1 should be PENDING
	j1After, _ := r.Get(j1.ID)
	if j1After.Status != StatusPending {
		t.Fatalf("expected j1 PENDING, got %s", j1After.Status)
	}
	if j1After.AssignedNodeID != "" || j1After.StartedAt != nil {
		t.Fatalf("expected j1 node and startedAt cleared")
	}
	events1, _ := r.GetEvents(j1.ID)
	lastEvent := events1[len(events1)-1]
	if lastEvent.Type != EventRecovered {
		t.Fatalf("expected last event RECOVERED, got %s", lastEvent.Type)
	}
	if lastEvent.From != StatusRunning || lastEvent.To != StatusPending || lastEvent.NodeID != nodeID {
		t.Fatalf("invalid RECOVERED event fields: %v", lastEvent)
	}

	// j2 should still be RUNNING
	j2After, _ := r.Get(j2.ID)
	if j2After.Status != StatusRunning {
		t.Fatalf("expected j2 RUNNING, got %s", j2After.Status)
	}

	// j3 should still be SUCCEEDED
	j3After, _ := r.Get(j3.ID)
	if j3After.Status != StatusSucceeded {
		t.Fatalf("expected j3 SUCCEEDED, got %s", j3After.Status)
	}

	// Recover again should yield 0
	recovered2 := r.RecoverExecutionTimeouts(30 * time.Second)
	if recovered2 != 0 {
		t.Fatalf("expected 0 jobs recovered, got %d", recovered2)
	}
	events1Again, _ := r.GetEvents(j1.ID)
	if len(events1Again) != len(events1) {
		t.Fatalf("duplicate RECOVERED events appended")
	}
}

func TestRegistry_RecoverExecutionTimeouts_MissingStartedAt(t *testing.T) {
	r := NewRegistry()
	j1 := r.Create("job1", "echo 1")

	r.mu.Lock()
	r.jobs[j1.ID].Status = StatusRunning
	r.jobs[j1.ID].StartedAt = nil
	r.mu.Unlock()

	// Should not panic, should recover 0
	recovered := r.RecoverExecutionTimeouts(1 * time.Second)
	if recovered != 0 {
		t.Fatalf("expected 0 recovered for missing StartedAt, got %d", recovered)
	}
}

func TestRegistry_RecoverExecutionTimeouts_WrongState(t *testing.T) {
	r := NewRegistry()
	jAssigned := r.Create("jobAssigned", "echo a")
	jFailed := r.Create("jobFailed", "echo f")

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(jAssigned.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	r.Update(jFailed.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	r.Claim(jFailed.ID, nodeID)
	r.ReportResult(jFailed.ID, nodeID, JobResult{ExitCode: 1, Stdout: ""})

	// Recover should skip them
	recovered := r.RecoverExecutionTimeouts(0 * time.Second)
	if recovered != 0 {
		t.Fatalf("expected 0 recovered for wrong state, got %d", recovered)
	}
}

func TestRegistry_RecoverExecutionTimeouts_Concurrency(t *testing.T) {
	r := NewRegistry()
	nodeID := "node-1"
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		j := r.Create(fmt.Sprintf("job-%d", i), "echo")
		statusAssigned := StatusAssigned
		r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
		r.Claim(j.ID, nodeID)

		// Force them to be slightly stale
		staleTime := time.Now().UTC().Add(-2 * time.Second)
		r.mu.Lock()
		r.jobs[j.ID].StartedAt = &staleTime
		r.mu.Unlock()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.RecoverExecutionTimeouts(1 * time.Second)
		}()
	}
	wg.Wait()
}

func TestRegistry_RecoverNodeJobs(t *testing.T) {
	r := NewRegistry()
	j1 := r.Create("job1", "echo 1") // assigned and running on node-1
	j2 := r.Create("job2", "echo 2") // assigned on node-1, not running
	j3 := r.Create("job3", "echo 3") // assigned and running on node-2

	node1 := "node-1"
	node2 := "node-2"
	statusAssigned := StatusAssigned

	r.Update(j1.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &node1})
	r.Update(j2.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &node1})
	r.Update(j3.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &node2})

	r.Claim(j1.ID, node1)
	r.Claim(j3.ID, node2)

	// Recover node-1 jobs
	recovered := r.RecoverNodeJobs(node1, "node offline")
	if recovered != 2 {
		t.Fatalf("expected 2 jobs recovered, got %d", recovered)
	}

	j1After, _ := r.Get(j1.ID)
	if j1After.Status != StatusPending || j1After.AssignedNodeID != "" || j1After.StartedAt != nil {
		t.Fatalf("j1 not correctly recovered: %+v", j1After)
	}

	j2After, _ := r.Get(j2.ID)
	if j2After.Status != StatusPending || j2After.AssignedNodeID != "" {
		t.Fatalf("j2 not correctly recovered: %+v", j2After)
	}

	j3After, _ := r.Get(j3.ID)
	if j3After.Status != StatusRunning || j3After.AssignedNodeID != node2 {
		t.Fatalf("j3 was incorrectly recovered: %+v", j3After)
	}

	// Idempotency check
	recoveredAgain := r.RecoverNodeJobs(node1, "node offline")
	if recoveredAgain != 0 {
		t.Fatalf("expected 0 recovered on retry, got %d", recoveredAgain)
	}
}
