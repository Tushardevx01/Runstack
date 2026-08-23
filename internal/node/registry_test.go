package node

import (
	"sync"
	"testing"
	"time"
)

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	n := Node{ID: "node-1", Hostname: "host-1", CPUCores: 2}

	registered := r.Register(n)
	if registered.Status != StatusOnline {
		t.Errorf("expected online, got %s", registered.Status)
	}

	fetched, err := r.Get("node-1")
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Hostname != "host-1" {
		t.Errorf("expected host-1, got %s", fetched.Hostname)
	}
}

func TestRegistry_Update(t *testing.T) {
	r := NewRegistry()
	n := Node{ID: "node-1", Hostname: "host-1", CPUCores: 2}
	r.Register(n)

	n.Hostname = "host-2"
	r.Register(n)

	fetched, _ := r.Get("node-1")
	if fetched.Hostname != "host-2" {
		t.Errorf("expected host-2, got %s", fetched.Hostname)
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("unknown")
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	list := r.List()
	if list == nil || len(list) != 0 {
		t.Errorf("expected empty list, got %v", list)
	}

	r.Register(Node{ID: "node-1"})
	r.Register(Node{ID: "node-2"})

	list = r.List()
	if len(list) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(list))
	}
}

func TestRegistry_Heartbeat(t *testing.T) {
	r := NewRegistry()
	r.Register(Node{ID: "node-1"})

	fetched1, _ := r.Get("node-1")

	time.Sleep(10 * time.Millisecond)

	hb, err := r.Heartbeat("node-1", nil)
	if err != nil {
		t.Fatal(err)
	}

	if hb.LastHeartbeat.Before(fetched1.LastHeartbeat) || hb.LastHeartbeat.Equal(fetched1.LastHeartbeat) {
		t.Errorf("expected updated heartbeat")
	}
}

func TestRegistry_MarkOfflineNodes(t *testing.T) {
	r := NewRegistry()
	r.Register(Node{ID: "node-1"})

	time.Sleep(50 * time.Millisecond)

	r.MarkOfflineNodes(10 * time.Millisecond)

	fetched, _ := r.Get("node-1")
	if fetched.Status != StatusOffline {
		t.Errorf("expected offline, got %s", fetched.Status)
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.Register(Node{ID: "node-1"})
			r.Get("node-1")
			r.List()
			r.Heartbeat("node-1", nil)
		}(i)
	}

	wg.Wait()
}
