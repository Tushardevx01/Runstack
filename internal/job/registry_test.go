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
	j := r.Create("test-job", "echo test", 0, 0, 0)
	if j.Name != "test-job" || j.Status != StatusPending {
		t.Errorf("unexpected job creation state")
	}
}

func TestRegistry_Update(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test-job", "echo test", 0, 0, 0)

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
			j := r.Create("concurrent-job", "echo", 0, 0, 0)

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
	j := r.Create("test", "echo 1", 0, 0, 0)

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

	j := r.Create("test-job", "echo test", 0, 0, 0)
	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, "node-1")
	execID := claimed.ExecutionID

	res := JobResult{
		ExitCode: 0,
		Stdout:   "test",
	}

	_, err := r.ReportResult(j.ID, "node-2", execID, res)
	if err == nil {
		t.Fatalf("expected error reporting from wrong node")
	}

	finished, err := r.ReportResult(j.ID, "node-1", execID, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if finished.Status != StatusSucceeded {
		t.Fatalf("expected status %s, got %s", StatusSucceeded, finished.Status)
	}
	if finished.Result.Stdout != "test" {
		t.Fatalf("expected stdout 'test', got %s", finished.Result.Stdout)
	}
	if finished.CompletedAt == nil {
		t.Fatalf("expected CompletedAt to be set")
	}

	// Test failing result
	j2 := r.Create("test-fail", "exit 1", 0, 0, 0)
	r.Update(j2.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	claimed2, _ := r.Claim(j2.ID, "node-1")
	execID2 := claimed2.ExecutionID

	failed, _ := r.ReportResult(j2.ID, "node-1", execID2, JobResult{ExitCode: 1, Error: "fail"})
	if failed.Status != StatusFailed {
		t.Fatalf("expected status %s, got %s", StatusFailed, failed.Status)
	}
}

func TestRegistry_EventHistory(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test-events", "echo 1", 0, 0, 0)

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
	j, _ = r.Claim(j.ID, nodeID)
	events, _ = r.GetEvents(j.ID)
	if len(events) != 3 || events[2].Type != EventClaimed {
		t.Fatalf("expected CLAIMED event, got %v", events)
	}

	// 3. Success
	_, _ = r.ReportResult(j.ID, nodeID, j.ExecutionID, JobResult{ExitCode: 0, Stdout: "done"})
	events, _ = r.GetEvents(j.ID)
	if len(events) != 4 || events[3].Type != EventSucceeded {
		t.Fatalf("expected SUCCEEDED event, got %v", events)
	}

	// 4. Idempotent result should NOT duplicate event
	_, _ = r.ReportResult(j.ID, nodeID, j.ExecutionID, JobResult{ExitCode: 0, Stdout: "done"})
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
	j := r.Create("test-events", "echo 1", 0, 0, 0)

	statusAssigned := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	j, _ = r.Claim(j.ID, nodeID)

	r.ReportResult(j.ID, nodeID, j.ExecutionID, JobResult{ExitCode: 1, Stdout: "fail"})

	events, _ := r.GetEvents(j.ID)
	if len(events) != 4 || events[3].Type != EventFailed {
		t.Fatalf("expected FAILED event, got %v", events)
	}
}

func TestRegistry_RecoverExecutionTimeouts(t *testing.T) {
	r := NewRegistry()
	j1 := r.Create("job1", "echo 1", 1, 0, 0)
	j2 := r.Create("job2", "echo 2", 1, 0, 0)
	j3 := r.Create("job3", "echo 3", 1, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j1.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	r.Update(j2.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	r.Update(j3.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	j1, _ = r.Claim(j1.ID, nodeID)
	j2, _ = r.Claim(j2.ID, nodeID)
	j3, _ = r.Claim(j3.ID, nodeID)

	// Make j1 very stale
	staleTime := time.Now().UTC().Add(-60 * time.Second)
	r.mu.Lock()
	r.jobs[j1.ID].StartedAt = &staleTime
	r.mu.Unlock()

	// j2 is fresh, StartedAt is now (default from Claim)
	// j3 will be completed
	r.ReportResult(j3.ID, nodeID, j3.ExecutionID, JobResult{ExitCode: 0, Stdout: "done"})

	recovered := r.RecoverExecutionTimeouts(30 * time.Second)
	if recovered != 1 {
		t.Fatalf("expected 1 job recovered, got %d", recovered)
	}

	// j1 should be PENDING
	j1After, _ := r.Get(j1.ID)
	if j1After.Status != StatusPending {
		t.Fatalf("expected j1 PENDING, got %s", j1After.Status)
	}
	if j1After.AssignedNodeID != "" || j1After.StartedAt != nil || j1After.ExecutionID != "" {
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
	j1 := r.Create("job1", "echo 1", 1, 0, 0)

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
	jAssigned := r.Create("jobAssigned", "echo a", 0, 0, 0)
	jFailed := r.Create("jobFailed", "echo f", 0, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(jAssigned.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	r.Update(jFailed.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	jFailed, _ = r.Claim(jFailed.ID, nodeID)
	r.ReportResult(jFailed.ID, nodeID, jFailed.ExecutionID, JobResult{ExitCode: 1, Stdout: ""})

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
		j := r.Create(fmt.Sprintf("job-%d", i), "echo", 1, 0, 0)
		statusAssigned := StatusAssigned
		r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
		j, _ = r.Claim(j.ID, nodeID)

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
	j1 := r.Create("job1", "echo 1", 1, 0, 0) // assigned and running on node-1
	j2 := r.Create("job2", "echo 2", 1, 0, 0) // assigned on node-1, not running
	j3 := r.Create("job3", "echo 3", 1, 0, 0) // assigned and running on node-2

	node1 := "node-1"
	node2 := "node-2"
	statusAssigned := StatusAssigned

	r.Update(j1.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &node1})
	r.Update(j2.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &node1})
	r.Update(j3.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &node2})

	j1, _ = r.Claim(j1.ID, node1)
	j3, _ = r.Claim(j3.ID, node2)

	// Recover node-1 jobs
	recovered := r.RecoverNodeJobs(node1, "node offline")
	if recovered != 2 {
		t.Fatalf("expected 2 jobs recovered, got %d", recovered)
	}

	j1After, _ := r.Get(j1.ID)
	if j1After.Status != StatusPending || j1After.AssignedNodeID != "" || j1After.StartedAt != nil || j1After.ExecutionID != "" {
		t.Fatalf("j1 not correctly recovered: %+v", j1After)
	}

	j2After, _ := r.Get(j2.ID)
	if j2After.Status != StatusPending || j2After.AssignedNodeID != "" || j2After.ExecutionID != "" {
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

func TestRegistry_RetryBudget_MaxRetries0(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job", "exit 1", 0, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	j, _ = r.Claim(j.ID, nodeID)
	if j.Attempts != 1 {
		t.Fatalf("expected Attempts = 1, got %d", j.Attempts)
	}

	j, _ = r.ReportResult(j.ID, nodeID, j.ExecutionID, JobResult{ExitCode: 1})
	if j.Status != StatusFailed {
		t.Fatalf("expected FAILED, got %s", j.Status)
	}
}

func TestRegistry_RetryBudget_MaxRetries1(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job", "exit 1", 1, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	// Attempt 1
	j, _ = r.Claim(j.ID, nodeID)
	if j.Attempts != 1 {
		t.Fatalf("expected Attempts = 1, got %d", j.Attempts)
	}
	j, _ = r.ReportResult(j.ID, nodeID, j.ExecutionID, JobResult{ExitCode: 1})
	if j.Status != StatusPending {
		t.Fatalf("expected PENDING, got %s", j.Status)
	}

	// Attempt 2
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	j, _ = r.Claim(j.ID, nodeID)
	if j.Attempts != 2 {
		t.Fatalf("expected Attempts = 2, got %d", j.Attempts)
	}
	j, _ = r.ReportResult(j.ID, nodeID, j.ExecutionID, JobResult{ExitCode: 1})
	if j.Status != StatusFailed {
		t.Fatalf("expected FAILED, got %s", j.Status)
	}
}

func TestRegistry_StaleResult_NoMutation(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job", "exit 1", 1, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	// Attempt 1
	claimed1, _ := r.Claim(j.ID, nodeID)

	// Recover it (simulate timeout)
	r.RecoverExecutionTimeouts(0 * time.Second)

	// Attempt 2
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	claimed2, _ := r.Claim(j.ID, nodeID)

	// Stale result for Attempt 1
	_, err := r.ReportResult(j.ID, nodeID, claimed1.ExecutionID, JobResult{ExitCode: 0})
	if err == nil {
		t.Fatalf("expected stale result to be rejected")
	}

	jAfter, _ := r.Get(j.ID)
	if jAfter.Attempts != 2 {
		t.Fatalf("stale result should not affect Attempts")
	}

	// Valid result for Attempt 2
	_, err = r.ReportResult(j.ID, nodeID, claimed2.ExecutionID, JobResult{ExitCode: 0})
	if err != nil {
		t.Fatalf("valid result failed: %v", err)
	}
}

func TestRegistry_TerminalFencing(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job", "exit 1", 0, 0, 0) // MaxRetries=0 => 1 attempt max

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	// Claim
	claimed, _ := r.Claim(j.ID, nodeID)

	// CP times out job -> StatusFailed (Attempts=1 > MaxRetries=0)
	r.RecoverExecutionTimeouts(0 * time.Second)

	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusFailed {
		t.Fatalf("expected FAILED, got %s", jAfter.Status)
	}

	// 1. Same node + same execID + same result -> 200 OK idempotent
	_, err := r.ReportResult(j.ID, nodeID, claimed.ExecutionID, JobResult{ExitCode: -1})
	if err != nil {
		t.Fatalf("expected idempotent success for matching result, got: %v", err)
	}

	// 2. Same node + same execID + contradictory result -> 409 Conflict
	_, err = r.ReportResult(j.ID, nodeID, claimed.ExecutionID, JobResult{ExitCode: 0})
	if err == nil {
		t.Fatalf("expected conflict for contradictory result")
	}
	if err.Error() != "contradictory execution result" {
		t.Fatalf("expected contradictory error, got: %v", err)
	}

	// 3. Terminal SUCCEEDED
	j2 := r.Create("job2", "echo", 0, 0, 0)
	r.Update(j2.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})
	claimed2, _ := r.Claim(j2.ID, nodeID)
	r.ReportResult(j2.ID, nodeID, claimed2.ExecutionID, JobResult{ExitCode: 0})

	_, err = r.ReportResult(j2.ID, nodeID, claimed2.ExecutionID, JobResult{ExitCode: 0})
	if err != nil {
		t.Fatalf("expected idempotent success for identical success result")
	}

	_, err = r.ReportResult(j2.ID, nodeID, claimed2.ExecutionID, JobResult{ExitCode: 1})
	if err == nil || err.Error() != "contradictory execution result" {
		t.Fatalf("expected contradictory error for SUCCEEDED job receiving failure")
	}
}

func TestRegistry_TerminalFencing_WrongNodeID(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job", "exit 1", 0, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, nodeID)
	r.RecoverExecutionTimeouts(0 * time.Second)

	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusFailed {
		t.Fatalf("expected FAILED, got %s", jAfter.Status)
	}

	_, err := r.ReportResult(j.ID, "node-wrong", claimed.ExecutionID, JobResult{ExitCode: -1})
	if err == nil || err.Error() != "stale execution result" {
		t.Fatalf("expected stale execution result error for wrong nodeID, got: %v", err)
	}
	_ = claimed // used above
}

func TestRegistry_TerminalFencing_WrongExecutionID(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job", "exit 1", 0, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	_, _ = r.Claim(j.ID, nodeID)
	r.RecoverExecutionTimeouts(0 * time.Second)

	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusFailed {
		t.Fatalf("expected FAILED, got %s", jAfter.Status)
	}

	_, err := r.ReportResult(j.ID, nodeID, "exec-wrong", JobResult{ExitCode: -1})
	if err == nil || err.Error() != "stale execution result" {
		t.Fatalf("expected stale execution result error for wrong executionID, got: %v", err)
	}
}

func TestRegistry_TerminalFencing_NoMutation(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job", "exit 1", 0, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	_, _ = r.Claim(j.ID, nodeID)
	r.RecoverExecutionTimeouts(0 * time.Second)

	// Capture state before rejected result
	jBefore, _ := r.Get(j.ID)
	eventsBefore, _ := r.GetEvents(j.ID)

	// Attempt with wrong executionID - should be rejected
	_, err := r.ReportResult(j.ID, nodeID, "exec-wrong", JobResult{ExitCode: -1})
	if err == nil {
		t.Fatalf("expected error for wrong executionID")
	}

	// Verify no mutation
	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != jBefore.Status {
		t.Fatalf("status was mutated by rejected result")
	}
	if jAfter.Attempts != jBefore.Attempts {
		t.Fatalf("attempts was mutated by rejected result")
	}
	if jAfter.ExecutionID != jBefore.ExecutionID {
		t.Fatalf("executionID was mutated by rejected result")
	}

	// Verify no new events
	eventsAfter, _ := r.GetEvents(j.ID)
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("events were appended by rejected result")
	}
}

func TestRegistry_TerminalFencing_ConcurrentReportResult(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job-concurrent", "echo", 0, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, nodeID)
	execID := claimed.ExecutionID

	// Multiple goroutines try to report the same result concurrently.
	// Exactly one should succeed; others should get idempotent success or conflict.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ReportResult(j.ID, nodeID, execID, JobResult{ExitCode: 0, Stdout: "ok"})
		}()
	}
	wg.Wait()

	// Job should be exactly SUCCEEDED with one event
	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusSucceeded {
		t.Fatalf("expected SUCCEEDED, got %s", jAfter.Status)
	}

	events, _ := r.GetEvents(j.ID)
	succeededCount := 0
	for _, e := range events {
		if e.Type == EventSucceeded {
			succeededCount++
		}
	}
	if succeededCount != 1 {
		t.Fatalf("expected exactly 1 SUCCEEDED event, got %d", succeededCount)
	}
}

func TestRegistry_TerminalFencing_ConcurrentContradictoryResults(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job-concurrent-contradict", "echo", 0, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, nodeID)
	execID := claimed.ExecutionID

	// First report: succeed
	_, err := r.ReportResult(j.ID, nodeID, execID, JobResult{ExitCode: 0})
	if err != nil {
		t.Fatalf("first report failed: %v", err)
	}

	// Now concurrently try contradictory results (ExitCode 1) and idempotent (ExitCode 0)
	// All should either return idempotent success or contradictory error.
	// None should mutate state.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ReportResult(j.ID, nodeID, execID, JobResult{ExitCode: 1, Error: "fail"})
		}()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ReportResult(j.ID, nodeID, execID, JobResult{ExitCode: 0, Stdout: "ok"})
		}()
	}
	wg.Wait()

	// State must be unchanged: still SUCCEEDED with ExitCode 0
	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusSucceeded {
		t.Fatalf("expected SUCCEEDED, got %s", jAfter.Status)
	}
	if jAfter.Result == nil || jAfter.Result.ExitCode != 0 {
		t.Fatalf("expected ExitCode 0, got %v", jAfter.Result)
	}

	// No additional events beyond the original SUCCEEDED
	events, _ := r.GetEvents(j.ID)
	succeededCount := 0
	for _, e := range events {
		if e.Type == EventSucceeded {
			succeededCount++
		}
	}
	if succeededCount != 1 {
		t.Fatalf("expected exactly 1 SUCCEEDED event, got %d", succeededCount)
	}
}

func TestRegistry_TerminalFencing_ContradictoryNoMutation(t *testing.T) {
	r := NewRegistry()
	j := r.Create("job", "exit 1", 0, 0, 0)

	nodeID := "node-1"
	statusAssigned := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &statusAssigned, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, nodeID)
	r.RecoverExecutionTimeouts(0 * time.Second)

	// Capture state before contradictory result
	jBefore, _ := r.Get(j.ID)
	eventsBefore, _ := r.GetEvents(j.ID)

	// Attempt with contradictory exitCode - should be rejected
	_, err := r.ReportResult(j.ID, nodeID, claimed.ExecutionID, JobResult{ExitCode: 0})
	if err == nil || err.Error() != "contradictory execution result" {
		t.Fatalf("expected contradictory execution result error, got: %v", err)
	}

	// Verify no mutation
	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != jBefore.Status {
		t.Fatalf("status was mutated by rejected result")
	}
	if jAfter.Attempts != jBefore.Attempts {
		t.Fatalf("attempts was mutated by rejected result")
	}

	// Verify no new events
	eventsAfter, _ := r.GetEvents(j.ID)
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("events were appended by rejected result")
	}
}

func TestRegistry_Update_CannotOverwriteStartedAtOnRunning(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	s := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, nodeID)
	if claimed.Status != StatusRunning {
		t.Fatalf("expected RUNNING, got %s", claimed.Status)
	}

	futureTime := time.Now().UTC().Add(1 * time.Hour)
	_, err := r.Update(j.ID, UpdateParams{StartedAt: &futureTime})
	if err == nil {
		t.Fatal("expected error blocking StartedAt overwrite on RUNNING job")
	}

	jAfter, _ := r.Get(j.ID)
	if jAfter.StartedAt.Equal(futureTime) {
		t.Fatal("StartedAt should not have been overwritten")
	}
}

func TestRegistry_Update_CannotManipulateFailedJob(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	s := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, nodeID)
	r.ReportResult(j.ID, nodeID, claimed.ExecutionID, JobResult{ExitCode: 1, Error: "fail"})

	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusFailed {
		t.Fatalf("expected FAILED, got %s", jAfter.Status)
	}

	newNode := "node-3"
	_, err := r.Update(j.ID, UpdateParams{AssignedNodeID: &newNode})
	if err == nil {
		t.Fatal("expected error blocking AssignedNodeID overwrite on FAILED job")
	}

	newResult := &JobResult{ExitCode: 0}
	_, err = r.Update(j.ID, UpdateParams{Result: newResult})
	if err == nil {
		t.Fatal("expected error blocking Result overwrite on FAILED job")
	}

	newStatus := StatusPending
	_, err = r.Update(j.ID, UpdateParams{Status: &newStatus})
	if err == nil {
		t.Fatal("expected error blocking terminal state change via Update")
	}
}

func TestRegistry_Update_AssignedToPendingBlocked(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	s := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})

	pending := StatusPending
	_, err := r.Update(j.ID, UpdateParams{Status: &pending})
	if err == nil {
		t.Fatal("expected error blocking ASSIGNED→PENDING via Update")
	}
}

func TestRegistry_Update_CannotOverwriteAssignedNodeOnRunning(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	s := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, nodeID)
	if claimed.Status != StatusRunning {
		t.Fatalf("expected RUNNING, got %s", claimed.Status)
	}

	newNode := "node-2"
	_, err := r.Update(j.ID, UpdateParams{AssignedNodeID: &newNode})
	if err == nil {
		t.Fatal("expected error blocking AssignedNodeID overwrite on RUNNING job")
	}

	jAfter, _ := r.Get(j.ID)
	if jAfter.AssignedNodeID != "node-1" {
		t.Fatalf("AssignedNodeID should not have changed, got %s", jAfter.AssignedNodeID)
	}
}

func TestRegistry_Update_CannotOverwriteFieldsOnSucceeded(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	s := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})

	claimed, _ := r.Claim(j.ID, nodeID)
	r.ReportResult(j.ID, nodeID, claimed.ExecutionID, JobResult{ExitCode: 0})

	newNode := "node-3"
	_, err := r.Update(j.ID, UpdateParams{AssignedNodeID: &newNode})
	if err == nil {
		t.Fatal("expected error blocking AssignedNodeID overwrite on SUCCEEDED job")
	}

	newResult := &JobResult{ExitCode: 1}
	_, err = r.Update(j.ID, UpdateParams{Result: newResult})
	if err == nil {
		t.Fatal("expected error blocking Result overwrite on SUCCEEDED job")
	}
}

// ========================================================================
// ADVERSARIAL AUDIT REGRESSION TESTS
// ========================================================================

func TestRegistry_Update_PendingToFailedBlocked(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 1, 0, 0)

	// An adversary must not be able to PATCH a PENDING job to FAILED,
	// bypassing the entire execution lifecycle.
	failed := StatusFailed
	_, err := r.Update(j.ID, UpdateParams{Status: &failed})
	if err == nil {
		t.Fatal("expected error blocking PENDING→FAILED via Update")
	}

	// Verify job remains PENDING
	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusPending {
		t.Fatalf("expected job to remain PENDING, got %s", jAfter.Status)
	}
}

func TestRegistry_Update_AssignedToFailedBlocked(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	s := StatusAssigned
	nodeID := "node-1"
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})

	// An adversary must not be able to PATCH an ASSIGNED job to FAILED.
	failed := StatusFailed
	_, err := r.Update(j.ID, UpdateParams{Status: &failed})
	if err == nil {
		t.Fatal("expected error blocking ASSIGNED→FAILED via Update")
	}

	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusAssigned {
		t.Fatalf("expected job to remain ASSIGNED, got %s", jAfter.Status)
	}
}

func TestRegistry_Update_PendingCannotSetAssignedNodeID(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	// Setting AssignedNodeID on a PENDING job without a status change
	// creates an impossible state invariant (PENDING with assigned node).
	nodeID := "node-evil"
	_, err := r.Update(j.ID, UpdateParams{AssignedNodeID: &nodeID})
	if err == nil {
		t.Fatal("expected error blocking AssignedNodeID on PENDING job without status transition")
	}

	jAfter, _ := r.Get(j.ID)
	if jAfter.AssignedNodeID != "" {
		t.Fatalf("expected AssignedNodeID to remain empty, got %s", jAfter.AssignedNodeID)
	}
}

func TestRegistry_Update_PendingCannotSetStartedAt(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	now := time.Now().UTC()
	_, err := r.Update(j.ID, UpdateParams{StartedAt: &now})
	if err == nil {
		t.Fatal("expected error blocking StartedAt on PENDING job without status transition")
	}

	jAfter, _ := r.Get(j.ID)
	if jAfter.StartedAt != nil {
		t.Fatal("expected StartedAt to remain nil")
	}
}

func TestRegistry_Update_PendingCannotSetResult(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	result := &JobResult{ExitCode: 0, Stdout: "hacked"}
	_, err := r.Update(j.ID, UpdateParams{Result: result})
	if err == nil {
		t.Fatal("expected error blocking Result on PENDING job without status transition")
	}

	jAfter, _ := r.Get(j.ID)
	if jAfter.Result != nil {
		t.Fatal("expected Result to remain nil")
	}
}

func TestRegistry_ReportResult_EmptyNodeID(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	nodeID := "node-1"
	s := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})
	claimed, _ := r.Claim(j.ID, nodeID)

	// Empty nodeID should be rejected as stale (not matching assigned node).
	_, err := r.ReportResult(j.ID, "", claimed.ExecutionID, JobResult{ExitCode: 0})
	if err == nil {
		t.Fatal("expected error for empty nodeID in ReportResult")
	}

	// Job should remain RUNNING.
	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusRunning {
		t.Fatalf("expected job to remain RUNNING, got %s", jAfter.Status)
	}
}

func TestRegistry_ReportResult_EmptyExecutionID(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 0, 0, 0)

	nodeID := "node-1"
	s := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})
	r.Claim(j.ID, nodeID)

	// Empty executionID should be rejected.
	_, err := r.ReportResult(j.ID, nodeID, "", JobResult{ExitCode: 0})
	if err == nil {
		t.Fatal("expected error for empty executionID in ReportResult")
	}

	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusRunning {
		t.Fatalf("expected job to remain RUNNING, got %s", jAfter.Status)
	}
}

func TestRegistry_StaleResult_AllPermutations(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 1, 0, 0)

	nodeID := "node-1"
	s := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})
	claimed, _ := r.Claim(j.ID, nodeID)
	execID := claimed.ExecutionID

	tests := []struct {
		name    string
		nodeID  string
		execID  string
		wantErr bool
	}{
		{"correct node + correct exec", "node-1", execID, false},
		{"correct node + wrong exec", "node-1", "exec-wrong", true},
		{"wrong node + correct exec", "node-wrong", execID, true},
		{"wrong node + wrong exec", "node-wrong", "exec-wrong", true},
		{"empty node + correct exec", "", execID, true},
		{"correct node + empty exec", "node-1", "", true},
		{"empty node + empty exec", "", "", true},
	}

	// Report the correct result first
	_, err := r.ReportResult(j.ID, tests[0].nodeID, tests[0].execID, JobResult{ExitCode: 0})
	if err != nil {
		t.Fatalf("first correct report failed: %v", err)
	}

	// Now test all permutations against the terminal SUCCEEDED state
	for _, tt := range tests[1:] {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.ReportResult(j.ID, tt.nodeID, tt.execID, JobResult{ExitCode: 0})
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}

	// Verify no mutation occurred
	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusSucceeded || jAfter.Attempts != 1 {
		t.Fatalf("state mutated by rejected results: status=%s attempts=%d", jAfter.Status, jAfter.Attempts)
	}

	events, _ := r.GetEvents(j.ID)
	succeededCount := 0
	for _, e := range events {
		if e.Type == EventSucceeded {
			succeededCount++
		}
	}
	if succeededCount != 1 {
		t.Fatalf("expected exactly 1 SUCCEEDED event, got %d", succeededCount)
	}
}

func TestRegistry_RecoveryDoesNotIncrementAttempts(t *testing.T) {
	r := NewRegistry()
	j := r.Create("test", "echo", 2, 0, 0)

	nodeID := "node-1"
	s := StatusAssigned
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})
	claimed, _ := r.Claim(j.ID, nodeID)

	if claimed.Attempts != 1 {
		t.Fatalf("expected Attempts=1 after claim, got %d", claimed.Attempts)
	}

	// Recovery should NOT increment attempts
	r.RecoverExecutionTimeouts(0 * time.Second)

	jAfter, _ := r.Get(j.ID)
	if jAfter.Status != StatusPending {
		t.Fatalf("expected PENDING after recovery, got %s", jAfter.Status)
	}
	if jAfter.Attempts != 1 {
		t.Fatalf("recovery should not change Attempts, expected 1 got %d", jAfter.Attempts)
	}

	// Node recovery also should not increment attempts
	r.Update(j.ID, UpdateParams{Status: &s, AssignedNodeID: &nodeID})
	r.Claim(j.ID, nodeID) // Attempts becomes 2

	r.RecoverNodeJobs(nodeID, "test recovery")

	jAfter2, _ := r.Get(j.ID)
	if jAfter2.Attempts != 2 {
		t.Fatalf("node recovery should not change Attempts, expected 2 got %d", jAfter2.Attempts)
	}
}
