package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"

	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/scheduler"
)

type NodeHandler struct {
	Registry *node.Registry
	CapCalc  *scheduler.CapacityCalculator
}

type NodeSummary struct {
	node.Node
	AllocatedCPU float64 `json:"allocated_cpu"`
	AllocatedMem int     `json:"allocated_mem"`
	AvailableCPU float64 `json:"available_cpu"`
	AvailableMem int     `json:"available_mem"`
}

type RegisterRequest struct {
	NodeID       string            `json:"nodeId"`
	Hostname     string            `json:"hostname"`
	IPAddress    string            `json:"ipAddress"`
	CPUCores     int               `json:"cpuCores"`
	OS           string            `json:"os"`
	Architecture string            `json:"architecture"`
	Capabilities node.Capabilities `json:"capabilities"`
}

type RegisterResponse struct {
	Status string     `json:"status"`
	Node   *node.Node `json:"node"`
	Token  string     `json:"token,omitempty"`
}

type ListNodesResponse struct {
	Nodes []NodeSummary `json:"nodes"`
}

func (h *NodeHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.NodeID == "" || req.Hostname == "" || req.CPUCores <= 0 || req.OS == "" || req.Architecture == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	ip := req.IPAddress
	if ip == "" {
		ip = r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ip = host
		}
	}

	n := node.Node{
		ID:           req.NodeID,
		Hostname:     req.Hostname,
		IPAddress:    ip,
		CPUCores:     req.CPUCores,
		OS:           req.OS,
		Architecture: req.Architecture,
		Capabilities: req.Capabilities,
	}

	bTok := make([]byte, 16)
	rand.Read(bTok)
	tok := hex.EncodeToString(bTok)
	registeredNode := h.Registry.Register(n, tok)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RegisterResponse{
		Status: "registered",
		Node:   registeredNode,
		Token:  tok,
	})
}

func (h *NodeHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	nodes := h.Registry.List()
	var summaries []NodeSummary

	var caps map[string]scheduler.NodeCapacity
	if h.CapCalc != nil {
		caps = h.CapCalc.CalculateAll(nodes)
	}

	for _, n := range nodes {
		sum := NodeSummary{Node: n}
		if h.CapCalc != nil {
			cap := caps[n.ID]
			sum.AllocatedCPU = float64(n.CPUCores) - cap.AvailableCPU
			sum.AllocatedMem = int(n.Capabilities.TotalMemoryBytes/1024/1024) - cap.AvailableMemoryMB
			sum.AvailableCPU = cap.AvailableCPU
			sum.AvailableMem = cap.AvailableMemoryMB
		}
		summaries = append(summaries, sum)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListNodesResponse{
		Nodes: summaries,
	})
}

func (h *NodeHandler) GetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := h.Registry.Get(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n)
}

type HeartbeatRequest struct {
	Capabilities *node.Capabilities `json:"capabilities,omitempty"`
}

func (h *NodeHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if ctxNodeID, ok := r.Context().Value("node_id").(string); ok && ctxNodeID != id {
		http.Error(w, "Forbidden: Identity mismatch", http.StatusForbidden)
		return
	}

	var caps *node.Capabilities
	if r.Body != nil && r.ContentLength > 0 {
		var req HeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			caps = req.Capabilities
		}
	}

	n, err := h.Registry.Heartbeat(id, caps)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n)
}
