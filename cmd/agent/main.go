package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"
)

type NodeRegistration struct {
	NodeID      string `json:"nodeId"`
	Hostname    string `json:"hostname"`
	CPUCores    int    `json:"cpuCores"`
	OS          string `json:"os"`
	Architecture string `json:"architecture"`
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}

	return hostname
}

func registerNode() error {
	node := NodeRegistration{
		NodeID:       getHostname(),
		Hostname:     getHostname(),
		CPUCores:     runtime.NumCPU(),
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}

	payload, err := json.Marshal(node)
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

	log.Printf(
		"Node registered: %s (%d CPU cores)",
		node.NodeID,
		node.CPUCores,
	)

	return nil
}

func main() {
	log.Println("RunStack Agent")
	log.Println("----------------")

	if err := registerNode(); err != nil {
		log.Fatalf("Registration failed: %v", err)
	}

	log.Println("Agent started successfully")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Heartbeat")
	}
}