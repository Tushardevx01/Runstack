package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/scheduler"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

type StatusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
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

func main() {
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
	jobHandler := &api.JobHandler{Registry: jobRegistry}

	sched := scheduler.New(registry, jobRegistry)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Background scheduling loop
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sched.SchedulePendingJobs(); err != nil {
					log.Printf("Scheduler error: %v", err)
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
	mux.HandleFunc("PATCH /api/v1/jobs/{id}", jobHandler.Update)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("RunStack Control Plane")
	log.Println("Listening on http://localhost:8080")

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
