package api

import (
	"encoding/json"
	"net/http"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
)

type InstanceHandler struct {
	InstanceRegistry   *instance.Registry
	DeploymentRegistry *deployment.Registry
}

type ClaimInstanceRequest struct {
	NodeID string `json:"node_id"`
}

type ClaimInstanceResponse struct {
	Instance    instance.Instance   `json:"instance"`
	ExecutionID string              `json:"execution_id"`
	Spec        application.AppSpec `json:"spec"`
}

func (h *InstanceHandler) Claim(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var req ClaimInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.NodeID == "" {
		http.Error(w, "missing node_id", http.StatusBadRequest)
		return
	}

	inst, err := h.InstanceRegistry.Claim(id, req.NodeID)
	if err != nil {
		if err == instance.ErrNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if err.Error() == "node ID mismatch" {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusConflict) // e.g. not ASSIGNED
		}
		return
	}

	// Fetch Deployment for AppSpec
	dep, err := h.DeploymentRegistry.Get(inst.DeploymentID)
	if err != nil {
		http.Error(w, "deployment not found", http.StatusInternalServerError)
		return
	}

	resp := ClaimInstanceResponse{
		Instance:    inst,
		ExecutionID: inst.ExecutionID,
		Spec:        dep.SpecSnapshot,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type InstanceStatusRequest struct {
	NodeID      string                  `json:"node_id"`
	ExecutionID string                  `json:"execution_id"`
	Status      instance.InstanceStatus `json:"status"`
	Health      instance.InstanceHealth `json:"health,omitempty"`
	ContainerID string                  `json:"container_id,omitempty"`
}

func (h *InstanceHandler) ReportStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var req InstanceStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.NodeID == "" || req.ExecutionID == "" || req.Status == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	oldInst, _ := h.InstanceRegistry.Get(id)

	inst, err := h.InstanceRegistry.ReportStatus(id, req.NodeID, req.ExecutionID, req.Status, req.Health, req.ContainerID)
	if err != nil {
		if err == instance.ErrNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if err.Error() == "node ID mismatch" || err.Error() == "stale execution ID" {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusBadRequest) // e.g. invalid state transition
		}
		return
	}

	if oldInst.Status != instance.StatusCrashed && req.Status == instance.StatusCrashed {
		h.DeploymentRegistry.RecordCrash(inst.DeploymentID, deployment.MaxCrashLoopThreshold)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inst)
}

func (h *InstanceHandler) List(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	status := r.URL.Query().Get("status")

	all := h.InstanceRegistry.List()
	var filtered []instance.Instance
	for _, inst := range all {
		if nodeID != "" && inst.NodeID != nodeID {
			continue
		}
		if status != "" && string(inst.Status) != status {
			continue
		}
		filtered = append(filtered, inst)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}
