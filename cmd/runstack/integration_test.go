package main

import (
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/scheduler"
)

func TestIntegration_NodeAwareRecovery(t *testing.T) {
	nodeReg := node.NewRegistry()
	jobReg := job.NewRegistry()

	sched := scheduler.New(nodeReg, jobReg)
	sched.ExecutionTimeout = 2 * time.Hour
	sched.NodeGracePeriod = 200 * time.Millisecond // very short for test

	// 1. Start Control Plane (simulate)
	// 2. Start Agent A (simulate registration)
	n := nodeReg.Register(node.Node{ID: "agent-a"})

	// 3. Create a job
	j := jobReg.Create("job-123", "echo hello")

	// 4. Scheduler assigns it
	sched.SchedulePendingJobs()
	jAssigned, _ := jobReg.Get(j.ID)
	if jAssigned.Status != job.StatusAssigned || jAssigned.AssignedNodeID != n.ID {
		t.Fatalf("expected ASSIGNED to agent-a")
	}

	// 5. Agent claims it
	jobReg.Claim(j.ID, n.ID)

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
		if e.Type == job.EventRecovered && e.NodeID == "agent-a" {
			hasRecovered = true
		}
	}
	if !hasRecovered {
		t.Fatalf("expected EventRecovered for agent-a")
	}

	// 11. Restart Agent A
	nodeReg.Heartbeat("agent-a", nil)

	// 13. Verify job can be reassigned
	sched.SchedulePendingJobs()
	jReassigned, _ := jobReg.Get(j.ID)
	if jReassigned.Status != job.StatusAssigned || jReassigned.AssignedNodeID != "agent-a" {
		t.Fatalf("expected job to be reassigned to agent-a")
	}

	// 14. Verify old execution cannot report result
	_, err := jobReg.ReportResult(j.ID, "agent-a", job.JobResult{ExitCode: 0})
	if err == nil {
		t.Fatalf("expected old execution result to be rejected (wrong state or assignment)")
	}
}
