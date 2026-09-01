package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Tushardevx01/runstack/internal/executor"
	"github.com/Tushardevx01/runstack/internal/runtime"
)

func startAgentAPI(cr runtime.ContainerRuntime, crashLogs *executor.LogRingBuffer, port int) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		containerID := r.URL.Query().Get("container")
		instID := r.URL.Query().Get("instance_id")
		appID := r.URL.Query().Get("app_id")
		execID := r.URL.Query().Get("exec_id")

		if containerID == "" && instID == "" {
			http.Error(w, "missing container or instance parameter", http.StatusBadRequest)
			return
		}

		// If it's a crashed instance, we might have logs in the ring buffer
		if instID != "" && appID != "" {
			lines, ok := crashLogs.Get(instID, appID, execID)
			if ok {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(strings.Join(lines, "\n") + "\n"))
				return
			}
		}

		if containerID != "" {
			if !strings.HasPrefix(containerID, "runstack-") {
				http.Error(w, "invalid container prefix", http.StatusForbidden)
				return
			}

			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()

			logReader, err := cr.Logs(ctx, containerID)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to get logs: %v", err), http.StatusInternalServerError)
				return
			}
			defer logReader.Close()

			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)

			io.Copy(w, logReader)
			return
		}

		http.Error(w, "Not found", http.StatusNotFound)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		slog.Info("Agent API listening", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Agent API server failed", "error", err)
		}
	}()
}
