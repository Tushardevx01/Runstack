package main

import (
	"flag"

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

func registerNode(nodeID, cpURL, agentToken string) (string, error) {
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
		return "", fmt.Errorf("failed to encode node information: %w", err)
	}

	resp, err := agentDoReq("POST", cpURL+"/api/v1/nodes/register", payload, agentToken)
	if err != nil {
		return "", fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	slog.Info("node registered", "node_id", nodeData.NodeID)
	var res map[string]string
	json.NewDecoder(resp.Body).Decode(&res)
	return res["token"], nil
}

func sendHeartbeat(nodeID, cpURL, agentToken string) error {
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

	resp, err := agentDoReq("POST", fmt.Sprintf("%s/api/v1/nodes/%s/heartbeat", cpURL, nodeID), payload, agentToken)
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	return nil
}

func runAgent(args []string) {

	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	agentToken := fs.String("token", os.Getenv("RUNSTACK_AGENT_TOKEN"), "Agent Bearer token")
	cpURL := fs.String("cp-url", os.Getenv("RUNSTACK_CP_URL"), "Control Plane URL")
	_ = fs.Parse(args)
	if *cpURL == "" {
		*cpURL = "http://localhost:8080"
	}

	slog.Info("RunStack Agent starting")

	nodeID := getHostname()

	slog.Info("Registering node", "node_id", nodeID)

	var nodeToken string
	for {
		var err error
		nodeToken, err = registerNode(nodeID, *cpURL, *agentToken)
		if err != nil {
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
				if err := sendHeartbeat(nodeID, *cpURL, nodeToken); err != nil {
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

	startJobPolling(ctx, nodeID, *cpURL, nodeToken)
}

func agentDoReq(method, url string, body []byte, token string) (*http.Response, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return http.DefaultClient.Do(req)
}
