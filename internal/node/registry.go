package node

import (
	"errors"
	"log/slog"
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

func (r *Registry) Register(n Node, token string) *Node {
	r.mu.Lock()

	n.Status = StatusOnline
	n.LastHeartbeat = time.Now()
	if existing, ok := r.nodes[n.ID]; ok && token == "" {
		n.Token = existing.Token
	} else {
		n.Token = token
	}

	nodePtr := &n
	r.nodes[n.ID] = nodePtr

	// Return a copy to prevent external mutation
	nodeCopy := *nodePtr
	r.mu.Unlock()

	slog.Info("Node registered", "node_id", n.ID)
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

func (r *Registry) Heartbeat(id string, caps *Capabilities) (*Node, error) {
	r.mu.Lock()

	if n, ok := r.nodes[id]; ok {
		isNewOnline := n.Status != StatusOnline
		n.Status = StatusOnline
		n.LastHeartbeat = time.Now()
		n.OfflineSince = nil
		if caps != nil {
			n.Capabilities = *caps
		}

		nodeCopy := *n
		r.mu.Unlock()

		if isNewOnline {
			slog.Info("Node came back online", "node_id", id)
		}
		return &nodeCopy, nil
	}
	r.mu.Unlock()
	return nil, ErrNodeNotFound
}

func (r *Registry) MarkOfflineNodes(timeout time.Duration) {
	r.mu.Lock()

	var offlineNodes []string
	now := time.Now()
	nowUTC := now.UTC()
	for _, n := range r.nodes {
		if now.Sub(n.LastHeartbeat) > timeout {
			if n.Status != StatusOffline {
				n.Status = StatusOffline
				n.OfflineSince = &nowUTC
				offlineNodes = append(offlineNodes, n.ID)
			}
		}
	}
	r.mu.Unlock()

	for _, id := range offlineNodes {
		slog.Warn("Node went offline", "node_id", id)
	}
}

func (r *Registry) GetByToken(token string) (Node, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, n := range r.nodes {
		if n.Token == token && token != "" {
			return *n, true
		}
	}
	return Node{}, false
}
