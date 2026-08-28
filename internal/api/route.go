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

func (h *RouteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ApplicationID string         `json:"application_id"`
		TargetPort    int            `json:"target_port"`
		Protocol      route.Protocol `json:"protocol"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, err := h.AppRegistry.Get(req.ApplicationID)
	if err != nil {
		app, err := h.AppRegistry.GetByName(req.ApplicationID)
		if err != nil {
			http.Error(w, "application not found", http.StatusNotFound)
			return
		}
		req.ApplicationID = app.ID
	}

	if req.Protocol == "" {
		req.Protocol = route.ProtocolHTTP
	}

	srv, err := h.ServiceRegistry.Create(req.ApplicationID, req.TargetPort, req.Protocol)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(srv)
}

func (h *RouteHandler) List(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("application_id")

	services := h.ServiceRegistry.List()

	var filtered []route.Service
	for _, srv := range services {
		if appID != "" && srv.ApplicationID != appID {
			continue
		}
		filtered = append(filtered, srv)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (h *RouteHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	srv, err := h.ServiceRegistry.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(srv)
}

func (h *RouteHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		TargetPort int            `json:"target_port"`
		Protocol   route.Protocol `json:"protocol"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	srv, err := h.ServiceRegistry.Update(id, req.TargetPort, req.Protocol)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(srv)
}

func (h *RouteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.ServiceRegistry.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
