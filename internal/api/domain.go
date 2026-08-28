package api

import (
	"encoding/json"
	"net/http"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/ingress"
)

type DomainHandler struct {
	DomainRegistry *ingress.DomainRegistry
	AppRegistry    *application.Registry
}

func (h *DomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		ApplicationID string `json:"application_id"`
		TLS           bool   `json:"tls"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.ApplicationID == "" {
		http.Error(w, "name and application_id are required", http.StatusBadRequest)
		return
	}

	// Verify Application Ownership
	_, err := h.AppRegistry.Get(req.ApplicationID)
	if err != nil {
		// Try get by name
		app, err := h.AppRegistry.GetByName(req.ApplicationID)
		if err != nil {
			http.Error(w, "application not found", http.StatusNotFound)
			return
		}
		req.ApplicationID = app.ID
	}

	domain, err := h.DomainRegistry.Create(req.Name, req.ApplicationID, req.TLS)
	if err != nil {
		if err == ingress.ErrDomainHijack {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(domain)
}

func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("application_id")

	domains := h.DomainRegistry.List()

	var filtered []ingress.Domain
	for _, d := range domains {
		if appID != "" && d.ApplicationID != appID {
			continue
		}
		filtered = append(filtered, d)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.DomainRegistry.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
