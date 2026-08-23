package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
)

type JobHandler struct {
	Registry *job.Registry
}

type CreateJobRequest struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type UpdateJobRequest struct {
	Status         *job.Status `json:"status,omitempty"`
	AssignedNodeID *string     `json:"assignedNodeId,omitempty"`
	StartedAt      *time.Time  `json:"startedAt,omitempty"`
	CompletedAt    *time.Time  `json:"completedAt,omitempty"`
	Result         *string     `json:"result,omitempty"`
}

type ListJobsResponse struct {
	Jobs []job.Job `json:"jobs"`
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
	jobs := h.Registry.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListJobsResponse{
		Jobs: jobs,
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
