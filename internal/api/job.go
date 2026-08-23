package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
)

type JobHandler struct {
	Registry     *job.Registry
	NodeRegistry *node.Registry
}

type CreateJobRequest struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type UpdateJobRequest struct {
	Status         *job.Status    `json:"status,omitempty"`
	AssignedNodeID *string        `json:"assignedNodeId,omitempty"`
	StartedAt      *time.Time     `json:"startedAt,omitempty"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	Result         *job.JobResult `json:"result,omitempty"`
}

type ListJobsResponse struct {
	Jobs []job.Job `json:"jobs"`
}

type ClaimRequest struct {
	NodeID string `json:"nodeId"`
}

type ReportResultRequest struct {
	NodeID      string        `json:"nodeId"`
	ExecutionID string        `json:"executionId"`
	Result      job.JobResult `json:"result"`
}

func (h *JobHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "malformed JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Command == "" {
		http.Error(w, "missing name or command", http.StatusBadRequest)
		return
	}

	j := h.Registry.Create(req.Name, req.Command)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(j)
}

func (h *JobHandler) List(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("assignedNodeId")
	status := r.URL.Query().Get("status")

	allJobs := h.Registry.List()
	var filtered []job.Job

	for _, j := range allJobs {
		if nodeID != "" && j.AssignedNodeID != nodeID {
			continue
		}
		if status != "" && string(j.Status) != status {
			continue
		}
		filtered = append(filtered, j)
	}

	if filtered == nil {
		filtered = make([]job.Job, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListJobsResponse{
		Jobs: filtered,
	})
}

func (h *JobHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, err := h.Registry.Get(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}

func (h *JobHandler) GetEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	events, err := h.Registry.GetEvents(id)
	if err == job.ErrJobNotFound {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if events == nil {
		events = []job.JobEvent{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
	})
}

func (h *JobHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req UpdateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "malformed JSON", http.StatusBadRequest)
		return
	}

	params := job.UpdateParams{
		Status:         req.Status,
		AssignedNodeID: req.AssignedNodeID,
		StartedAt:      req.StartedAt,
		CompletedAt:    req.CompletedAt,
		Result:         req.Result,
	}

	j, err := h.Registry.Update(id, params)
	if err != nil {
		if err == job.ErrJobNotFound {
			http.Error(w, "Not Found", http.StatusNotFound)
		} else if err == job.ErrInvalidTransition {
			http.Error(w, "invalid status transition", http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}

func (h *JobHandler) Claim(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req ClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "malformed JSON", http.StatusBadRequest)
		return
	}

	n, err := h.NodeRegistry.Get(req.NodeID)
	if err != nil || n.Status != node.StatusOnline {
		http.Error(w, "node not online or not found", http.StatusForbidden)
		return
	}

	j, err := h.Registry.Claim(id, req.NodeID)
	if err != nil {
		if err == job.ErrJobNotFound {
			http.Error(w, "Not Found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusConflict)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}

func (h *JobHandler) ReportResult(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req ReportResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "malformed JSON", http.StatusBadRequest)
		return
	}

	if req.ExecutionID == "" {
		http.Error(w, "executionId is required", http.StatusBadRequest)
		return
	}

	_, err := h.NodeRegistry.Get(req.NodeID)
	if err != nil {
		http.Error(w, "node not found", http.StatusForbidden)
		return
	}
	j, err := h.Registry.ReportResult(id, req.NodeID, req.ExecutionID, req.Result)

	if err != nil {
		if err == job.ErrJobNotFound {
			http.Error(w, "Not Found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusConflict)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}
