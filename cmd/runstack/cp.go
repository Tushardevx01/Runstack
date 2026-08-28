package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/route"
	"github.com/Tushardevx01/runstack/internal/scheduler"
	"github.com/Tushardevx01/runstack/internal/service"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:    "ok",
		Service:   "runstack-control-plane",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := StatusResponse{
		Status:  "running",
		Version: "0.1.0",
	}

	json.NewEncoder(w).Encode(response)
}

func runControlPlane() {
	registry := node.NewRegistry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Background offline detection
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				registry.MarkOfflineNodes(30 * time.Second)
			}
		}
	}()

	nodeHandler := &api.NodeHandler{Registry: registry}

	jobRegistry := job.NewRegistry()
	jobHandler := &api.JobHandler{Registry: jobRegistry, NodeRegistry: registry}

	appRegistry := application.NewRegistry()
	depRegistry := deployment.NewRegistry()
	instRegistry := instance.NewRegistry()
	routeRegistry := route.NewRegistry()

	httpProxy := route.NewHTTPProxy(80) // Default proxy port
	go func() {
		if err := httpProxy.Start(ctx); err != nil {
			slog.Error("HTTP Proxy failed", "error", err)
		}
	}()

	appService := service.NewAppService(appRegistry, depRegistry)
	logsHandler := &api.LogsHandler{
		AppRegistry:      appRegistry,
		InstanceRegistry: instRegistry,
		NodeRegistry:     registry,
	}

	appHandler := &api.AppHandler{
		Service: appService,
	}
	routeHandler := &api.RouteHandler{
		ServiceRegistry: routeRegistry,
		AppRegistry:     appRegistry,
	}

	sched := scheduler.New(registry, jobRegistry)
	instSched := scheduler.NewInstanceScheduler(registry, instRegistry)
	instReconciler := scheduler.NewInstanceReconciler(appRegistry, depRegistry, instRegistry, registry)
	routingReconciler := scheduler.NewRoutingReconciler(appRegistry, instRegistry, registry, routeRegistry, httpProxy)

	// Start background scheduler loops
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sched.SchedulePendingJobs(); err != nil {
					slog.Error("Job Scheduler error", "error", err)
				}
				if err := instReconciler.Reconcile(); err != nil {
					slog.Error("Instance Reconciler error", "error", err)
				}
				if err := instSched.SchedulePendingInstances(); err != nil {
					slog.Error("Instance Scheduler error", "error", err)
				}
				if err := routingReconciler.Reconcile(ctx); err != nil {
					slog.Error("Routing Reconciler error", "error", err)
				}
			}
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /api/v1/status", statusHandler)

	mux.HandleFunc("POST /api/v1/nodes/register", nodeHandler.Register)
	mux.HandleFunc("GET /api/v1/nodes", nodeHandler.ListNodes)
	mux.HandleFunc("GET /api/v1/nodes/{id}", nodeHandler.GetNode)
	mux.HandleFunc("POST /api/v1/nodes/{id}/heartbeat", nodeHandler.Heartbeat)

	mux.HandleFunc("POST /api/v1/jobs", jobHandler.Create)
	mux.HandleFunc("GET /api/v1/jobs", jobHandler.List)
	mux.HandleFunc("GET /api/v1/jobs/{id}", jobHandler.Get)
	mux.HandleFunc("GET /api/v1/jobs/{id}/events", jobHandler.GetEvents)
	mux.HandleFunc("PATCH /api/v1/jobs/{id}", jobHandler.Update)
	mux.HandleFunc("POST /api/v1/jobs/{id}/claim", jobHandler.Claim)
	mux.HandleFunc("POST /api/v1/jobs/{id}/result", jobHandler.ReportResult)

	mux.HandleFunc("POST /api/v1/apps", appHandler.Create)
	mux.HandleFunc("GET /api/v1/apps", appHandler.List)
	mux.HandleFunc("GET /api/v1/apps/{id}", appHandler.Get)
	mux.HandleFunc("GET /api/v1/apps/{id}/logs", logsHandler.GetAppLogs)
	mux.HandleFunc("PUT /api/v1/apps/{id}", appHandler.Update)
	mux.HandleFunc("POST /api/v1/apps/{id}/deploy", appHandler.Deploy)
	mux.HandleFunc("POST /api/v1/apps/{id}/rollback", appHandler.Rollback)

	mux.HandleFunc("POST /api/v1/services", routeHandler.Create)
	mux.HandleFunc("GET /api/v1/services", routeHandler.List)
	mux.HandleFunc("GET /api/v1/services/{id}", routeHandler.Get)
	mux.HandleFunc("PUT /api/v1/services/{id}", routeHandler.Update)
	mux.HandleFunc("DELETE /api/v1/services/{id}", routeHandler.Delete)

	instanceHandler := &api.InstanceHandler{
		InstanceRegistry:   instRegistry,
		DeploymentRegistry: depRegistry,
	}
	mux.HandleFunc("GET /api/v1/instances", instanceHandler.List)
	mux.HandleFunc("POST /api/v1/instances/{id}/claim", instanceHandler.Claim)
	mux.HandleFunc("POST /api/v1/instances/{id}/status", instanceHandler.ReportStatus)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("RunStack Control Plane started")
	slog.Info("Listening", "address", "http://localhost:8080")

	if err := server.ListenAndServe(); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
