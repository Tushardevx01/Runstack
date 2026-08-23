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
	NodeID       string `json:"nodeId"`
	Hostname     string `json:"hostname"`
	CPUCores     int    `json:"cpuCores"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func registerNode(nodeID string) error {
	node := NodeRegistration{
		NodeID:       nodeID,
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

	log.Printf("Node registered: %s", node.NodeID)
	return nil
}

func sendHeartbeat(nodeID string) error {
	resp, err := http.Post(
		fmt.Sprintf("http://localhost:8080/api/v1/nodes/%s/heartbeat", nodeID),
		"application/json",
		nil,
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

func main() {
	log.Println("RunStack Agent")
	log.Println("--------------")

	nodeID := getHostname()

	log.Println("Registering node...")

	for {
		if err := registerNode(nodeID); err != nil {
			log.Printf("Registration failed: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}

	log.Println("Heartbeat started")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := sendHeartbeat(nodeID); err != nil {
			log.Printf("Heartbeat failed: %v", err)
		} else {
			log.Println("Heartbeat sent")
		}
	}
}
