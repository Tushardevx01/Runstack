package api

import (
	"encoding/json"
	"net/http"

	"github.com/Tushardevx01/runstack/internal/node"
)

type NodeHandler struct {
	Registry *node.Registry
}

type RegisterRequest struct {
	NodeID       string `json:"nodeId"`
	Hostname     string `json:"hostname"`
	CPUCores     int    `json:"cpuCores"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type RegisterResponse struct {
	Status string     `json:"status"`
	Node   *node.Node `json:"node"`
}

type ListNodesResponse struct {
	Nodes []node.Node `json:"nodes"`
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

	n := node.Node{
		ID:           req.NodeID,
		Hostname:     req.Hostname,
		CPUCores:     req.CPUCores,
		OS:           req.OS,
		Architecture: req.Architecture,
	}

	registeredNode := h.Registry.Register(n)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RegisterResponse{
		Status: "registered",
		Node:   registeredNode,
	})
}

func (h *NodeHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	nodes := h.Registry.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListNodesResponse{
		Nodes: nodes,
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

func (h *NodeHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := h.Registry.Heartbeat(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n)
}
