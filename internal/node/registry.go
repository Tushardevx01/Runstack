package node

import (
	"errors"
	"sync"
	"time"
)

var ErrNodeNotFound = errors.New("node not found")

type Registry struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

func NewRegistry() *Registry {
	return &Registry{
		nodes: make(map[string]*Node),
	}
}

func (r *Registry) Register(n Node) *Node {
	r.mu.Lock()
	defer r.mu.Unlock()

	n.Status = StatusOnline
	n.LastHeartbeat = time.Now()

	nodePtr := &n
	r.nodes[n.ID] = nodePtr

	// Return a copy to prevent external mutation
	nodeCopy := *nodePtr
	return &nodeCopy
}

func (r *Registry) Get(id string) (*Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if n, ok := r.nodes[id]; ok {
		nodeCopy := *n
		return &nodeCopy, nil
	}
	return nil, ErrNodeNotFound
}

func (r *Registry) List() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		result = append(result, *n)
	}
	return result
}

func (r *Registry) Heartbeat(id string) (*Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if n, ok := r.nodes[id]; ok {
		n.Status = StatusOnline
		n.LastHeartbeat = time.Now()

		nodeCopy := *n
		return &nodeCopy, nil
	}
	return nil, ErrNodeNotFound
}

func (r *Registry) MarkOfflineNodes(timeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for _, n := range r.nodes {
		if now.Sub(n.LastHeartbeat) > timeout {
			n.Status = StatusOffline
		}
	}
}
