package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Tushardevx01/runstack/internal/runtime"
)

func startAgentAPI(cr runtime.ContainerRuntime, port int) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		containerID := r.URL.Query().Get("container")
		if containerID == "" {
			http.Error(w, "missing container parameter", http.StatusBadRequest)
			return
		}

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

		// Stream logs back
		io.Copy(w, logReader)
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
