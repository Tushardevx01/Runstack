package scheduler

import (
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"testing"
)

func TestInstanceScheduler_DeterministicRoundRobin(t *testing.T) {
	nodeReg := node.NewRegistry()
	instReg := instance.NewRegistry()

	nodeReg.Register(node.Node{ID: "node-C"})
	nodeReg.Register(node.Node{ID: "node-A"})
	nodeReg.Register(node.Node{ID: "node-B"})

	s := NewInstanceScheduler(nodeReg, instReg)

	i1, _ := instReg.Create("app1", "dep1")
	i2, _ := instReg.Create("app1", "dep1")

	s.SchedulePendingInstances()

	got1, _ := instReg.Get(i1.ID)
	got2, _ := instReg.Get(i2.ID)

	// Since they are sorted by ID, we don't strictly know which one goes to A or B
	// without checking IDs. But we know they should go to A and B.
	assigned := map[string]int{}
	assigned[got1.NodeID]++
	assigned[got2.NodeID]++

	if assigned["node-A"] != 1 || assigned["node-B"] != 1 {
		t.Errorf("expected round robin assignment to node-A and node-B, got %v", assigned)
	}
}

func TestInstanceScheduler_NoOnlineNodes(t *testing.T) {
	nodeReg := node.NewRegistry()
	instReg := instance.NewRegistry()

	s := NewInstanceScheduler(nodeReg, instReg)

	i1, _ := instReg.Create("app1", "dep1")

	err := s.SchedulePendingInstances()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got1, _ := instReg.Get(i1.ID)
	if got1.Status != instance.StatusPending {
		t.Errorf("expected PENDING, got %s", got1.Status)
	}
}
