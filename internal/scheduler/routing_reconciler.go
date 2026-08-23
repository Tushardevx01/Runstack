package scheduler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/route"
)

type RoutingReconciler struct {
	appReg  *application.Registry
	instReg *instance.Registry
	nodeReg *node.Registry
	proxy   route.ProxyProvider

	// Wait, we need a ServiceRegistry. Let's assume we have it.
	routeReg *route.Registry

	mu sync.Mutex
}

func NewRoutingReconciler(appReg *application.Registry, instReg *instance.Registry, nodeReg *node.Registry, routeReg *route.Registry, proxy route.ProxyProvider) *RoutingReconciler {
	return &RoutingReconciler{
		appReg:   appReg,
		instReg:  instReg,
		nodeReg:  nodeReg,
		routeReg: routeReg,
		proxy:    proxy,
	}
}

func (r *RoutingReconciler) Reconcile(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	services := r.routeReg.List()
	apps := r.appReg.List()
	nodes := r.nodeReg.List()
	instances := r.instReg.List()

	nodeMap := make(map[string]node.Node)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	appMap := make(map[string]application.Application)
	for _, app := range apps {
		appMap[app.ID] = app
	}

	for _, srv := range services {
		_, exists := appMap[srv.ApplicationID]
		if !exists {
			_ = r.proxy.RemoveRoute(ctx, srv.ID)
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

			// Only allow instances belonging to the ActiveDeployment or if rolling out,
			// wait, if RolloutController handles capacity, any RUNNING+HEALTHY instance of the App is technically safe
			// because obsolete deployments are killed, and we are routing to ANY healthy instance.
			// However, to be strict, we route to instances of ANY deployment that the InstanceReconciler is actively keeping alive!
			// Actually, just checking RUNNING+HEALTHY+!Draining is sufficient for Zero-Downtime Migration.
			// Let's also ensure execution identity is set and Node exists.

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

		err := r.proxy.UpdateRoute(ctx, srv, endpoints)
		if err != nil {
			slog.Error("Failed to update route in proxy", "service_id", srv.ID, "error", err)
			// Explicitly isolate proxy failures: do NOT modify instances!
		}
	}

	return nil
}
