package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/instance"
)

type AppsHandler struct {
	AppRegistry      *application.Registry
	DepRegistry      *deployment.Registry
	InstanceRegistry *instance.Registry
	DomainRegistry   *ingress.DomainRegistry
	IngressRegistry  *ingress.IngressRegistry
}

type AppSummary struct {
	ID                 string                   `json:"id"`
	Name               string                   `json:"name"`
	Status             application.AppStatus    `json:"status"`
	ActiveDeploymentID string                   `json:"active_deployment_id"`
	RolloutStatus      deployment.RolloutStatus `json:"rollout_status"`
	ReadyReplicas      int                      `json:"ready_replicas"`
	DesiredReplicas    int                      `json:"desired_replicas"`
	BlockedReason      string                   `json:"blocked_reason,omitempty"`
	Domains            []string                 `json:"domains"`
	TLSActive          bool                     `json:"tls_active"`
}

type InstanceSummary struct {
	ID           string                  `json:"id"`
	DeploymentID string                  `json:"deployment_id"`
	NodeID       string                  `json:"node_id"`
	Status       instance.InstanceStatus `json:"status"`
	Health       instance.InstanceHealth `json:"health"`
	CPU          float64                 `json:"cpu"`
	MemoryMB     int                     `json:"memory_mb"`
}

type DomainSummary struct {
	Name   string               `json:"name"`
	TLS    bool                 `json:"tls"`
	Status ingress.DomainStatus `json:"status"`
	Path   string               `json:"path"`
}

type AppStatusDetail struct {
	Application      ApplicationMeta    `json:"application"`
	ActiveDeployment *DeploymentSummary `json:"active_deployment"`
	Instances        []InstanceSummary  `json:"instances"`
	Domains          []DomainSummary    `json:"domains"`
}

type ApplicationMeta struct {
	ID     string                `json:"id"`
	Name   string                `json:"name"`
	Status application.AppStatus `json:"status"`
}

type DeploymentSummary struct {
	ID                  string                            `json:"id"`
	Version             int                               `json:"version"`
	Hash                string                            `json:"hash"`
	RolloutStatus       deployment.RolloutStatus          `json:"rollout_status"`
	DesiredReplicas     int                               `json:"desired_replicas"`
	UpdatedReplicas     int                               `json:"updated_replicas"`
	ReadyReplicas       int                               `json:"ready_replicas"`
	UnavailableReplicas int                               `json:"unavailable_replicas"`
	BlockedReason       string                            `json:"blocked_reason,omitempty"`
	Degraded            bool                              `json:"degraded"`
	Resources           *application.ResourceRequirements `json:"resources,omitempty"`
	Image               string                            `json:"image"`
}

func (h *AppsHandler) ListApps(w http.ResponseWriter, r *http.Request) {
	apps := h.AppRegistry.List()

	// Deterministic sort
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})

	var summaries []AppSummary
	for _, app := range apps {
		summary := AppSummary{
			ID:                 app.ID,
			Name:               app.Name,
			Status:             app.Status,
			ActiveDeploymentID: app.ActiveDeploymentID,
			Domains:            []string{},
		}

		// Fetch Deployment
		if app.ActiveDeploymentID != "" {
			if dep, err := h.DepRegistry.Get(app.ActiveDeploymentID); err == nil {
				summary.RolloutStatus = dep.RolloutStatus
				summary.ReadyReplicas = dep.ReadyReplicas
				summary.DesiredReplicas = dep.DesiredReplicas
				summary.BlockedReason = dep.BlockedReason
			}
		} else {
			summary.DesiredReplicas = app.Spec.Replicas
		}

		// Fetch Domains
		tlsActive := false
		for _, d := range h.DomainRegistry.List() {
			if d.ApplicationID == app.ID {
				summary.Domains = append(summary.Domains, d.Name)
				if d.TLS && d.Status == ingress.DomainStatusActive {
					tlsActive = true
				}
			}
		}
		sort.Strings(summary.Domains)
		summary.TLSActive = tlsActive

		summaries = append(summaries, summary)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"applications": summaries})
}

func (h *AppsHandler) GetAppStatus(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	name := parts[4]

	app, err := h.AppRegistry.GetByName(name)
	if err != nil {
		http.Error(w, "application not found", http.StatusNotFound)
		return
	}

	detail := AppStatusDetail{
		Application: ApplicationMeta{
			ID:     app.ID,
			Name:   app.Name,
			Status: app.Status,
		},
		Instances: []InstanceSummary{},
		Domains:   []DomainSummary{},
	}

	if app.ActiveDeploymentID != "" {
		if dep, err := h.DepRegistry.Get(app.ActiveDeploymentID); err == nil {
			detail.ActiveDeployment = &DeploymentSummary{
				ID:                  dep.ID,
				Version:             dep.Version,
				Hash:                dep.Hash,
				RolloutStatus:       dep.RolloutStatus,
				DesiredReplicas:     dep.DesiredReplicas,
				UpdatedReplicas:     dep.UpdatedReplicas,
				ReadyReplicas:       dep.ReadyReplicas,
				UnavailableReplicas: dep.UnavailableReplicas,
				BlockedReason:       dep.BlockedReason,
				Degraded:            dep.Degraded,
				Resources:           dep.SpecSnapshot.Resources,
				Image:               dep.SpecSnapshot.Image,
			}
		}
	}

	for _, inst := range h.InstanceRegistry.List() {
		if inst.ApplicationID == app.ID {
			sum := InstanceSummary{
				ID:           inst.ID,
				DeploymentID: inst.DeploymentID,
				NodeID:       inst.NodeID,
				Status:       inst.Status,
				Health:       inst.Health,
			}
			if detail.ActiveDeployment != nil && detail.ActiveDeployment.Resources != nil {
				sum.CPU = detail.ActiveDeployment.Resources.CPU
				sum.MemoryMB = detail.ActiveDeployment.Resources.MemoryMB
			}
			detail.Instances = append(detail.Instances, sum)
		}
	}
	sort.Slice(detail.Instances, func(i, j int) bool {
		return detail.Instances[i].ID < detail.Instances[j].ID
	})

	ingresses := h.IngressRegistry.List()
	for _, d := range h.DomainRegistry.List() {
		if d.ApplicationID == app.ID {
			sum := DomainSummary{
				Name:   d.Name,
				TLS:    d.TLS,
				Status: d.Status,
				Path:   "/",
			}
			for _, ing := range ingresses {
				if ing.DomainID == d.ID {
					sum.Path = ing.Path
					break
				}
			}
			detail.Domains = append(detail.Domains, sum)
		}
	}
	sort.Slice(detail.Domains, func(i, j int) bool {
		return detail.Domains[i].Name < detail.Domains[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}
