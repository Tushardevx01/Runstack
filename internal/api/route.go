package api

import (
	"encoding/json"
	"net/http"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/route"
)

type RouteHandler struct {
	ServiceRegistry *route.Registry
	AppRegistry     *application.Registry
}

type CreateServiceRequest struct {
	ApplicationID string         `json:"application_id"`
	Domain        string         `json:"domain"`
	PathPrefix    string         `json:"path_prefix"`
	TargetPort    int            `json:"target_port"`
	Protocol      route.Protocol `json:"protocol"`
}

func (h *RouteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.ApplicationID == "" || req.TargetPort <= 0 {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	// Validate Application ownership
	if _, err := h.AppRegistry.Get(req.ApplicationID); err != nil {
		http.Error(w, "application not found", http.StatusBadRequest)
		return
	}

	srv, err := h.ServiceRegistry.Create(req.ApplicationID, req.Domain, req.PathPrefix, req.TargetPort, req.Protocol)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(srv)
}

func (h *RouteHandler) List(w http.ResponseWriter, r *http.Request) {
	services := h.ServiceRegistry.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func (h *RouteHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	srv, err := h.ServiceRegistry.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(srv)
}

func (h *RouteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := h.ServiceRegistry.Delete(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type UpdateServiceRequest struct {
	Domain     string         `json:"domain"`
	PathPrefix string         `json:"path_prefix"`
	TargetPort int            `json:"target_port"`
	Protocol   route.Protocol `json:"protocol"`
}

func (h *RouteHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req UpdateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	srv, err := h.ServiceRegistry.Update(id, req.Domain, req.PathPrefix, req.TargetPort, req.Protocol)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(srv)
}
