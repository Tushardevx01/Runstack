package api

import (
	"encoding/json"
	"net/http"

	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/route"
)

type IngressHandler struct {
	IngressRegistry *ingress.IngressRegistry
	DomainRegistry  *ingress.DomainRegistry
	ServiceRegistry *route.Registry
}

func (h *IngressHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DomainID  string `json:"domain_id"`
		ServiceID string `json:"service_id"`
		Path      string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	domain, err := h.DomainRegistry.Get(req.DomainID)
	if err != nil {
		http.Error(w, "domain not found", http.StatusNotFound)
		return
	}

	service, err := h.ServiceRegistry.Get(req.ServiceID)
	if err != nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	// Security: Prevent cross-application ingress mapping
	if domain.ApplicationID != service.ApplicationID {
		http.Error(w, "domain and service belong to different applications", http.StatusForbidden)
		return
	}

	ing, err := h.IngressRegistry.Create(req.DomainID, req.ServiceID, req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ing)
}

func (h *IngressHandler) List(w http.ResponseWriter, r *http.Request) {
	domainID := r.URL.Query().Get("domain_id")

	ingresses := h.IngressRegistry.List()

	var filtered []ingress.Ingress
	for _, ing := range ingresses {
		if domainID != "" && ing.DomainID != domainID {
			continue
		}
		filtered = append(filtered, ing)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (h *IngressHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.IngressRegistry.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
