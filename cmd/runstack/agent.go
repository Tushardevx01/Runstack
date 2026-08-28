package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/executor"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/runtime/docker"
	"github.com/Tushardevx01/runstack/internal/sysinfo"
)

type NodeRegistration struct {
	NodeID       string            `json:"nodeId"`
	Hostname     string            `json:"hostname"`
	CPUCores     int               `json:"cpuCores"`
	OS           string            `json:"os"`
	Architecture string            `json:"architecture"`
	Capabilities node.Capabilities `json:"capabilities"`
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func registerNode(nodeID string) error {
	memInfo := sysinfo.GetMemoryInfo()
	caps := node.Capabilities{
		TotalMemoryBytes:     memInfo.TotalBytes,
		AvailableMemoryBytes: memInfo.AvailableBytes,
		HasDocker:            sysinfo.HasDocker(),
		HasPodman:            sysinfo.HasPodman(),
	}

	nodeData := NodeRegistration{
		NodeID:       nodeID,
		Hostname:     getHostname(),
		CPUCores:     runtime.NumCPU(),
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Capabilities: caps,
	}

	payload, err := json.Marshal(nodeData)
	if err != nil {
		return fmt.Errorf("failed to encode node information: %w", err)
	}

	resp, err := http.Post(
		"http://localhost:8080/api/v1/nodes/register",
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	slog.Info("node registered", "node_id", nodeData.NodeID)
	return nil
}

func sendHeartbeat(nodeID string) error {
	memInfo := sysinfo.GetMemoryInfo()
	caps := node.Capabilities{
		TotalMemoryBytes:     memInfo.TotalBytes,
		AvailableMemoryBytes: memInfo.AvailableBytes,
		HasDocker:            sysinfo.HasDocker(),
		HasPodman:            sysinfo.HasPodman(),
	}

	payload, err := json.Marshal(map[string]interface{}{
		"capabilities": caps,
	})
	if err != nil {
		return fmt.Errorf("failed to encode heartbeat information: %w", err)
	}

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:8080/api/v1/nodes/%s/heartbeat", nodeID),
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	return nil
}

func runAgent() {
	slog.Info("RunStack Agent starting")

	nodeID := getHostname()

	slog.Info("Registering node", "node_id", nodeID)

	for {
		if err := registerNode(nodeID); err != nil {
			slog.Warn("Registration failed. Retrying...", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		slog.Info("Shutting down agent...")
		cancel()
	}()

	slog.Info("Heartbeat started")

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sendHeartbeat(nodeID); err != nil {
					slog.Error("Heartbeat failed", "error", err)
				} else {
					slog.Debug("Heartbeat sent")
				}
			}
		}
	}()

	apiClient := api.NewClient("http://localhost:8080")

	cr := docker.New()
	instanceExec := executor.NewInstanceExecutor(nodeID, apiClient, cr)
	instanceExec.Start()
	defer instanceExec.Stop()

	startAgentAPI(cr, 8081)

	startJobPolling(ctx, nodeID)
}
