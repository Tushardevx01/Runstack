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

	// Background offline detection
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			registry.MarkOfflineNodes(30 * time.Second)
		}
	}()

	nodeHandler := &api.NodeHandler{Registry: registry}

	jobRegistry := job.NewRegistry()
	jobHandler := &api.JobHandler{Registry: jobRegistry, NodeRegistry: registry}

	appRegistry := application.NewRegistry()
	depRegistry := deployment.NewRegistry()
	instRegistry := instance.NewRegistry()
	appService := service.NewAppService(appRegistry, depRegistry)
	appHandler := &api.AppHandler{
		Service: appService,
	}

	sched := scheduler.New(registry, jobRegistry)
	instSched := scheduler.NewInstanceScheduler(registry, instRegistry)
	instReconciler := scheduler.NewInstanceReconciler(appRegistry, depRegistry, instRegistry, registry)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	mux.HandleFunc("PUT /api/v1/apps/{id}", appHandler.Update)

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
