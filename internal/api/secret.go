package api

import (
	"encoding/json"
	"net/http"

	"github.com/Tushardevx01/runstack/internal/application"
)

type SecretHandler struct {
	Registry *application.SecretRegistry
	AppReg   *application.Registry
}

type SetSecretRequest struct {
	Name          string `json:"name"`
	Value         string `json:"value"`
	ApplicationID string `json:"application_id"`
}

func (h *SecretHandler) Set(w http.ResponseWriter, r *http.Request) {
	var req SetSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Value == "" || req.ApplicationID == "" {
		http.Error(w, "name, value, and application_id are required", http.StatusBadRequest)
		return
	}

	// Verify application exists
	_, err := h.AppReg.Get(req.ApplicationID)
	if err != nil {
		http.Error(w, "application not found", http.StatusNotFound)
		return
	}

	sec, err := h.Registry.Set(req.ApplicationID, req.Name, req.Value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sec)
}

func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("application_id")
	if appID == "" {
		http.Error(w, "application_id is required", http.StatusBadRequest)
		return
	}

	secrets := h.Registry.ListByApp(appID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(secrets)
}

func (h *SecretHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if err := h.Registry.Delete(id); err != nil {
		if err == application.ErrSecretNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
