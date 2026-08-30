package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/manifest"
	"github.com/Tushardevx01/runstack/internal/route"
)

type ApplyHandler struct {
	AppRegistry     *application.Registry
	DepRegistry     *deployment.Registry
	SecretRegistry  *application.SecretRegistry
	DomainRegistry  *ingress.DomainRegistry
	IngressRegistry *ingress.IngressRegistry
	ServiceRegistry *route.Registry
	CertProvider    ingress.CertificateProvider
}

type ApplyResult struct {
	ApplicationID string `json:"application_id"`
	DeploymentID  string `json:"deployment_id"`
	Action        string `json:"action"` // created, updated, unchanged
}

func (h *ApplyHandler) Apply(w http.ResponseWriter, r *http.Request) {
	var mf manifest.Manifest
	if err := json.NewDecoder(r.Body).Decode(&mf); err != nil {
		http.Error(w, "invalid manifest json", http.StatusBadRequest)
		return
	}

	if err := mf.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	res := ApplyResult{
		Action: "unchanged",
	}

	// 1. Resolve Application
	app, err := h.AppRegistry.GetByName(mf.Name)
	if err == application.ErrNotFound {
		app, err = h.AppRegistry.Create(mf.Name, application.AppSpec{})
		if err != nil {
			http.Error(w, "failed to create application: "+err.Error(), http.StatusInternalServerError)
			return
		}
		res.Action = "created"
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res.ApplicationID = app.ID

	// 2. Validate Secrets
	for _, sName := range mf.Secrets {
		_, err := h.SecretRegistry.GetByName(app.ID, sName)
		if err != nil {
			http.Error(w, "missing required secret: "+sName, http.StatusPreconditionFailed)
			return
		}
	}

	// 3. Service
	var serviceID string
	if mf.Service != nil {
		var svc route.Service
		var svcErr error = errors.New("not found")
		for _, s := range h.ServiceRegistry.List() {
			if s.ApplicationID == app.ID {
				svc = s
				svcErr = nil
				break
			}
		}
		if svcErr != nil {
			svc, err = h.ServiceRegistry.Create(app.ID, mf.Service.Port, "tcp")
			if err != nil {
				http.Error(w, "failed to create service", http.StatusInternalServerError)
				return
			}
			if res.Action == "unchanged" {
				res.Action = "updated"
			}
		} else {
			if svc.TargetPort != mf.Service.Port {
				h.ServiceRegistry.Update(svc.ID, mf.Service.Port, "tcp")
				if res.Action == "unchanged" {
					res.Action = "updated"
				}
			}
		}
		serviceID = svc.ID
	}

	// 4. Domains and Ingress
	currentDomains := make([]ingress.Domain, 0)
	for _, d := range h.DomainRegistry.List() {
		if d.ApplicationID == app.ID {
			currentDomains = append(currentDomains, d)
		}
	}

	desiredDomains := make(map[string]bool)
	if mf.Service != nil {
		for _, dSpec := range mf.Domains {
			desiredDomains[dSpec.Name] = true
			d, err := h.DomainRegistry.GetByName(dSpec.Name)
			if err == ingress.ErrDomainNotFound {
				d, err = h.DomainRegistry.Create(dSpec.Name, app.ID, dSpec.TLS)
				if err != nil {
					http.Error(w, "failed to map domain "+dSpec.Name+": "+err.Error(), http.StatusInternalServerError)
					return
				}
				if res.Action == "unchanged" {
					res.Action = "updated"
				}
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else if d.ApplicationID != app.ID {
				// Prevent implicit ownership transfer
				http.Error(w, "domain "+dSpec.Name+" is already owned by another application", http.StatusConflict)
				return
			} else if d.TLS != dSpec.TLS {
				h.DomainRegistry.UpdateTLS(d.ID, dSpec.TLS)
				if res.Action == "unchanged" {
					res.Action = "updated"
				}
			}

			// Ingress
			var ing ingress.Ingress
			var ingErr error = errors.New("not found")
			for _, i := range h.IngressRegistry.List() {
				if i.DomainID == d.ID {
					ing = i
					ingErr = nil
					break
				}
			}
			if ingErr != nil {
				_, err = h.IngressRegistry.Create(d.ID, serviceID, "/")
				if err != nil {
					http.Error(w, "failed to map ingress for "+dSpec.Name+": "+err.Error(), http.StatusInternalServerError)
					return
				}
			} else if ing.ServiceID != serviceID {
				h.IngressRegistry.Delete(ing.ID)
				_, _ = h.IngressRegistry.Create(d.ID, serviceID, "/")
			}

			// Request TLS if enabled
			if dSpec.TLS && h.CertProvider != nil {
				h.CertProvider.RequestCertificate(r.Context(), dSpec.Name)
			}
		}
	}

	// Prune domains
	for _, d := range currentDomains {
		if !desiredDomains[d.Name] {
			var ing ingress.Ingress
			var ingErr error = errors.New("not found")
			for _, i := range h.IngressRegistry.List() {
				if i.DomainID == d.ID {
					ing = i
					ingErr = nil
					break
				}
			}
			if ingErr == nil {
				h.IngressRegistry.Delete(ing.ID)
			}
			h.DomainRegistry.Delete(d.ID)
			if res.Action == "unchanged" {
				res.Action = "updated"
			}
		}
	}

	// 5. Compare & Create Deployment
	var activeDep deployment.Deployment
	deps := h.DepRegistry.ListByApplication(app.ID)
	// Find latest active
	for _, d := range deps {
		if d.ID == app.ActiveDeploymentID {
			activeDep = d
			break
		}
	}

	newHash := mf.HashDeployment()
	depCreated := false
	activeDepID := app.ActiveDeploymentID

	// Check if ANY existing deployment for this app matches the hash.
	var matchingDep *deployment.Deployment
	for _, d := range deps {
		if d.Hash == newHash {
			matched := d
			matchingDep = &matched
			break
		}
	}

	if matchingDep != nil {
		activeDep = *matchingDep
		activeDepID = matchingDep.ID
		res.DeploymentID = matchingDep.ID
		if app.ActiveDeploymentID != matchingDep.ID {
			if res.Action == "unchanged" {
				res.Action = "updated"
			}
		}
	} else if app.ActiveDeploymentID == "" || activeDep.Hash != newHash {
		spec := mf.ToDeploymentSpec()
		dep, err := h.DepRegistry.Create(app.ID, spec)
		if err != nil {
			http.Error(w, "failed to create deployment: "+err.Error(), http.StatusInternalServerError)
			return
		}
		activeDep = dep
		activeDepID = dep.ID
		depCreated = true
		res.DeploymentID = dep.ID
		if res.Action == "unchanged" {
			res.Action = "updated"
		}
	} else {
		res.DeploymentID = activeDep.ID
	}

	// 6. Update App Spec (Replicas + ActiveDepID)
	spec := mf.ToDeploymentSpec()
	spec.Replicas = mf.Replicas

	h.AppRegistry.Update(app.ID, spec, activeDepID, app.Status)

	if app.Spec.Replicas != mf.Replicas && !depCreated {
		if res.Action == "unchanged" {
			res.Action = "updated"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (h *ApplyHandler) Diff(w http.ResponseWriter, r *http.Request) {
	var mf manifest.Manifest
	if err := json.NewDecoder(r.Body).Decode(&mf); err != nil {
		http.Error(w, "invalid manifest json", http.StatusBadRequest)
		return
	}

	if err := mf.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Diff logic doesn't mutate
	// Just return JSON representation
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "diff calculated",
	})
}
