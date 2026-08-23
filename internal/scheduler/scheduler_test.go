package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
)

func TestScheduler_NoJobs(t *testing.T) {
	nR := node.NewRegistry()
	jR := job.NewRegistry()
	s := New(nR, jR)

	nR.Register(node.Node{ID: "node-1", Status: node.StatusOnline})

	if err := s.SchedulePendingJobs(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(jR.List()) != 0 {
		t.Errorf("expected 0 jobs")
	}
}

func TestScheduler_NoNodes(t *testing.T) {
	nR := node.NewRegistry()
	jR := job.NewRegistry()
	s := New(nR, jR)

	j := jR.Create("test", "echo 1")

	if err := s.SchedulePendingJobs(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jAfter, _ := jR.Get(j.ID)
	if jAfter.Status != job.StatusPending {
		t.Errorf("expected job to remain PENDING")
	}
}

func TestScheduler_PendingJob_NoOnlineNode(t *testing.T) {
	nR := node.NewRegistry()
	jR := job.NewRegistry()
	s := New(nR, jR)

	j := jR.Create("test", "echo 1")
	nR.Register(node.Node{ID: "node-1", Status: node.StatusOffline})

	// Override last heartbeat to ensure it's marked offline in our tests if needed
	// Actually we can just wait or set it directly. Wait, Register sets it to online.
	// Oh, Register() forces Status = StatusOnline. Let's use MarkOfflineNodes to force it.
	nR.MarkOfflineNodes(-1 * time.Second) // Forces all to offline

	if err := s.SchedulePendingJobs(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jAfter, _ := jR.Get(j.ID)
	if jAfter.Status != job.StatusPending {
		t.Errorf("expected job to remain PENDING")
	}
}

func TestScheduler_PendingJob_OneOnlineNode(t *testing.T) {
	nR := node.NewRegistry()
	jR := job.NewRegistry()
	s := New(nR, jR)

	j := jR.Create("test", "echo 1")
	nR.Register(node.Node{ID: "node-1"})

	if err := s.SchedulePendingJobs(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jAfter, _ := jR.Get(j.ID)
	if jAfter.Status != job.StatusAssigned {
		t.Errorf("expected job to become ASSIGNED")
	}
	if jAfter.AssignedNodeID != "node-1" {
		t.Errorf("expected job assigned to node-1")
	}
	if jAfter.StartedAt != nil || jAfter.CompletedAt != nil || jAfter.Result != "" {
		t.Errorf("expected execution fields to remain unset")
	}
}

func TestScheduler_DeterministicSelection(t *testing.T) {
	nR := node.NewRegistry()
	jR := job.NewRegistry()
	s := New(nR, jR)

	nR.Register(node.Node{ID: "node-C"})
	nR.Register(node.Node{ID: "node-A"})
	nR.Register(node.Node{ID: "node-B"})

	j1 := jR.Create("test-1", "echo 1")
	j2 := jR.Create("test-2", "echo 2")

	s.SchedulePendingJobs()

	j1After, _ := jR.Get(j1.ID)
	j2After, _ := jR.Get(j2.ID)

	if j1After.AssignedNodeID != "node-A" || j2After.AssignedNodeID != "node-A" {
		t.Errorf("expected deterministic assignment to node-A, got %s and %s", j1After.AssignedNodeID, j2After.AssignedNodeID)
	}
}

func TestScheduler_IgnoreAssignedJobs(t *testing.T) {
	nR := node.NewRegistry()
	jR := job.NewRegistry()
	s := New(nR, jR)

	nR.Register(node.Node{ID: "node-A"})

	j := jR.Create("test", "echo 1")

	status := job.StatusAssigned
	nodeID := "node-other"
	jR.Update(j.ID, job.UpdateParams{
		Status:         &status,
		AssignedNodeID: &nodeID,
	})

	s.SchedulePendingJobs()

	jAfter, _ := jR.Get(j.ID)
	if jAfter.AssignedNodeID != "node-other" {
		t.Errorf("expected job to remain assigned to node-other")
	}
}

func TestScheduler_Concurrent(t *testing.T) {
	nR := node.NewRegistry()
	jR := job.NewRegistry()
	s := New(nR, jR)

	nR.Register(node.Node{ID: "node-A"})

	// Create 100 jobs
	for i := 0; i < 100; i++ {
		jR.Create("test", "echo 1")
	}

	// Schedule concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.SchedulePendingJobs()
		}()
	}
	wg.Wait()

	// Ensure all 100 are ASSIGNED exactly once (no errors / conflicts should leak out)
	for _, j := range jR.List() {
		if j.Status != job.StatusAssigned {
			t.Errorf("expected job %s to be ASSIGNED, got %s", j.ID, j.Status)
		}
	}
}
