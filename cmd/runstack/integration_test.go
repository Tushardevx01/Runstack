package main

import (
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/scheduler"
)

func TestIntegration_NodeAwareRecovery(t *testing.T) {
	nodeReg := node.NewRegistry()
	jobReg := job.NewRegistry()

	sched := scheduler.New(nodeReg, jobReg, scheduler.NewCapacityCalculator(application.NewRegistry(), deployment.NewRegistry(), instance.NewRegistry(), jobReg))
	sched.ExecutionTimeout = 2 * time.Hour
	sched.NodeGracePeriod = 200 * time.Millisecond // very short for test

	// 1. Start Control Plane (simulate)
	// 2. Start Agent A (simulate registration)
	n := nodeReg.Register(node.Node{ID: "agent-a"}, "")

	// 3. Create a job
	j := jobReg.Create("job-123", "echo hello", 1, 0, 0)

	// 4. Scheduler assigns it
	sched.SchedulePendingJobs()
	jAssigned, _ := jobReg.Get(j.ID)
	if jAssigned.Status != job.StatusAssigned || jAssigned.AssignedNodeID != n.ID {
		t.Fatalf("expected ASSIGNED to agent-a")
	}

	// 5. Agent claims it
	claimed, _ := jobReg.Claim(j.ID, n.ID)
	execA := claimed.ExecutionID

	// 6. Kill Agent A / Wait for OFFLINE
	// Simulate time passing by manually triggering offline via a very small timeout
	time.Sleep(10 * time.Millisecond)
	nodeReg.MarkOfflineNodes(1 * time.Millisecond)

	// Verify node is offline
	nOffline, _ := nodeReg.Get(n.ID)
	if nOffline.Status != node.StatusOffline {
		t.Fatalf("expected agent-a to be OFFLINE")
	}

	// 7. Scheduler runs within grace period (should not recover)
	sched.SchedulePendingJobs()
	jStillRunning, _ := jobReg.Get(j.ID)
	if jStillRunning.Status != job.StatusRunning {
		t.Fatalf("job should still be RUNNING within grace period")
	}

	// 8. Wait until NodeGracePeriod expires
	time.Sleep(250 * time.Millisecond)

	// 9. Scheduler runs and recovers
	sched.SchedulePendingJobs()
	jRecovered, _ := jobReg.Get(j.ID)
	// it recovers to PENDING and then Step 5 assigns it... wait! Node A is offline, there are no other nodes!
	// So it will stay PENDING.
	if jRecovered.Status != job.StatusPending {
		t.Fatalf("job should be PENDING, got %s", jRecovered.Status)
	}

	// 10. Verify EventRecovered exists
	events, _ := jobReg.GetEvents(j.ID)
	hasRecovered := false
	for _, e := range events {
		if e.Type == job.EventRecovered && e.NodeID == "agent-a" && e.ExecutionID == execA {
			hasRecovered = true
		}
	}
	if !hasRecovered {
		t.Fatalf("expected EventRecovered for agent-a with execution ID")
	}

	// 11. Restart Agent A or bring up Agent B
	nodeReg.Register(node.Node{ID: "agent-b"}, "")

	// 12. Verify job can be reassigned to Agent B
	sched.SchedulePendingJobs()
	jReassigned, _ := jobReg.Get(j.ID)
	if jReassigned.Status != job.StatusAssigned || jReassigned.AssignedNodeID != "agent-b" {
		t.Fatalf("expected job to be reassigned to agent-b")
	}

	// 13. Agent B claims it
	claimedB, _ := jobReg.Claim(j.ID, "agent-b")
	execB := claimedB.ExecutionID

	// 14. Verify Agent A's old execution cannot report result
	_, err := jobReg.ReportResult(j.ID, "agent-a", execA, job.JobResult{ExitCode: 0})
	if err == nil {
		t.Fatalf("expected old execution result from agent-a to be rejected")
	}

	// 15. Verify Agent B can report its execution
	_, err = jobReg.ReportResult(j.ID, "agent-b", execB, job.JobResult{ExitCode: 0})
	if err != nil {
		t.Fatalf("expected Agent B result to succeed, got %v", err)
	}
}

func TestIntegration_ExecutionFencing_And_Retry(t *testing.T) {
	nodeReg := node.NewRegistry()
	jobReg := job.NewRegistry()

	sched := scheduler.New(nodeReg, jobReg, scheduler.NewCapacityCalculator(application.NewRegistry(), deployment.NewRegistry(), instance.NewRegistry(), jobReg))

	// Register Agent A and Agent B
	nodeReg.Register(node.Node{ID: "agent-a"}, "")
	nodeReg.Register(node.Node{ID: "agent-b"}, "")

	// Create job with MaxRetries = 1
	j := jobReg.Create("job-retry", "exit 1", 1, 0, 0)

	// Schedule
	sched.SchedulePendingJobs()
	jAssigned, _ := jobReg.Get(j.ID)
	if jAssigned.Status != job.StatusAssigned {
		t.Fatalf("expected ASSIGNED")
	}
	assignedNode := jAssigned.AssignedNodeID

	// Agent A claims
	claimedA, err := jobReg.Claim(j.ID, assignedNode)
	if err != nil {
		t.Fatalf("failed to claim: %v", err)
	}
	execA := claimedA.ExecutionID

	if claimedA.Attempts != 1 {
		t.Fatalf("expected Attempts = 1, got %d", claimedA.Attempts)
	}

	// Agent A reports failure
	reportedA, err := jobReg.ReportResult(j.ID, assignedNode, execA, job.JobResult{ExitCode: 1})
	if err != nil {
		t.Fatalf("failed to report result: %v", err)
	}

	// Should be PENDING
	if reportedA.Status != job.StatusPending {
		t.Fatalf("expected PENDING after first failure due to retry, got %s", reportedA.Status)
	}
	if reportedA.Attempts != 1 {
		t.Fatalf("expected Attempts to remain 1 after reporting, got %d", reportedA.Attempts)
	}

	// Schedule again
	sched.SchedulePendingJobs()
	jAssigned2, _ := jobReg.Get(j.ID)
	if jAssigned2.Status != job.StatusAssigned {
		t.Fatalf("expected ASSIGNED again")
	}
	assignedNode2 := jAssigned2.AssignedNodeID

	// Agent B claims
	claimedB, err := jobReg.Claim(j.ID, assignedNode2)
	if err != nil {
		t.Fatalf("failed to claim: %v", err)
	}
	execB := claimedB.ExecutionID

	if claimedB.Attempts != 2 {
		t.Fatalf("expected Attempts = 2, got %d", claimedB.Attempts)
	}

	// Agent B reports failure
	reportedB, err := jobReg.ReportResult(j.ID, assignedNode2, execB, job.JobResult{ExitCode: 1})
	if err != nil {
		t.Fatalf("failed to report result: %v", err)
	}

	// Should be FAILED
	if reportedB.Status != job.StatusFailed {
		t.Fatalf("expected FAILED after retries exhausted, got %s", reportedB.Status)
	}
	if reportedB.Attempts != 2 {
		t.Fatalf("expected Attempts = 2, got %d", reportedB.Attempts)
	}

	// Third claim should be impossible
	sched.SchedulePendingJobs()
	jAssigned3, _ := jobReg.Get(j.ID)
	if jAssigned3.Status != job.StatusFailed {
		t.Fatalf("job should remain FAILED, got %s", jAssigned3.Status)
	}
}
