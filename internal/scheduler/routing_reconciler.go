package scheduler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/route"
)

type RoutingReconciler struct {
	appReg     *application.Registry
	instReg    *instance.Registry
	nodeReg    *node.Registry
	routeReg   *route.Registry
	domainReg  *ingress.DomainRegistry
	ingressReg *ingress.IngressRegistry
	proxy      route.ProxyProvider

	mu sync.Mutex
}

func NewRoutingReconciler(
	appReg *application.Registry,
	instReg *instance.Registry,
	nodeReg *node.Registry,
	routeReg *route.Registry,
	domainReg *ingress.DomainRegistry,
	ingressReg *ingress.IngressRegistry,
	proxy route.ProxyProvider,
) *RoutingReconciler {
	return &RoutingReconciler{
		appReg:     appReg,
		instReg:    instReg,
		nodeReg:    nodeReg,
		routeReg:   routeReg,
		domainReg:  domainReg,
		ingressReg: ingressReg,
		proxy:      proxy,
	}
}

func (r *RoutingReconciler) Reconcile(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	services := r.routeReg.List()
	apps := r.appReg.List()
	nodes := r.nodeReg.List()
	instances := r.instReg.List()
	domains := r.domainReg.List()
	ingresses := r.ingressReg.List()

	nodeMap := make(map[string]node.Node)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	appMap := make(map[string]application.Application)
	for _, app := range apps {
		appMap[app.ID] = app
	}

	serviceMap := make(map[string]route.Service)
	for _, srv := range services {
		serviceMap[srv.ID] = srv
	}

	domainMap := make(map[string]ingress.Domain)
	for _, d := range domains {
		domainMap[d.ID] = d
	}

	var rules []route.RouteRule

	// Process each ingress definition
	for _, ing := range ingresses {
		domain, exists := domainMap[ing.DomainID]
		if !exists {
			continue
		}

		// 8G: Domain must point to an Application's Service. Ensure ownership.
		if _, exists := appMap[domain.ApplicationID]; !exists {
			continue
		}

		srv, exists := serviceMap[ing.ServiceID]
		if !exists {
			continue
		}

		// Security: Service must belong to the Domain's Application
		if srv.ApplicationID != domain.ApplicationID {
			slog.Warn("Ingress rejected due to cross-application boundary", "ingress_id", ing.ID, "domain_app", domain.ApplicationID, "service_app", srv.ApplicationID)
			continue
		}

		var endpoints []route.Endpoint
		for _, inst := range instances {
			if inst.ApplicationID != srv.ApplicationID {
				continue
			}

			// Draining/Stopped check
			if inst.Draining || inst.Status != instance.StatusRunning || inst.Health != instance.HealthHealthy {
				continue
			}

			if inst.ExecutionID == "" || inst.NodeID == "" {
				continue
			}

			n, ok := nodeMap[inst.NodeID]
			if !ok || n.IPAddress == "" {
				continue
			}

			targetHostPort := 0
			for _, p := range inst.Ports {
				if p.Internal == srv.TargetPort {
					targetHostPort = p.External
					break
				}
			}

			if targetHostPort > 0 {
				endpoints = append(endpoints, route.Endpoint{
					IP:   n.IPAddress,
					Port: targetHostPort,
				})
			}
		}

		rules = append(rules, route.RouteRule{
			Host:      domain.Name,
			Path:      ing.Path,
			Endpoints: endpoints,
		})
	}

	err := r.proxy.UpdateRoutes(ctx, rules)
	if err != nil {
		slog.Error("Failed to update routes in proxy", "error", err)
	}

	return nil
}
