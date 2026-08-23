package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Tushardevx01/runstack/internal/node"
)

type StatusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type ListNodesResponse struct {
	Nodes []node.Node `json:"nodes"`
}

func getStatus() error {
	resp, err := http.Get("http://localhost:8080/api/v1/status")
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var status StatusResponse

	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Println("RunStack Control Plane")
	fmt.Println("----------------------")
	fmt.Printf("Status:  %s\n", status.Status)
	fmt.Printf("Version: %s\n", status.Version)

	return nil
}

func getNodes() error {
	resp, err := http.Get("http://localhost:8080/api/v1/nodes")
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var result ListNodesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Nodes) == 0 {
		fmt.Println("No nodes registered.")
		return nil
	}

	fmt.Println("RunStack Nodes")
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tHOSTNAME\tCPU\tOS\tSTATUS")
	fmt.Fprintln(w, "------------------------------------------------")
	for _, n := range result.Nodes {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n", n.ID, n.Hostname, n.CPUCores, n.OS, n.Status)
	}
	w.Flush()

	return nil
}

func getNode(id string) error {
	resp, err := http.Get("http://localhost:8080/api/v1/nodes/" + id)
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("node not found")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var n node.Node
	if err := json.NewDecoder(resp.Body).Decode(&n); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Println("Node")
	fmt.Println("-------------------------")
	fmt.Printf("ID:             %s\n", n.ID)
	fmt.Printf("Hostname:       %s\n", n.Hostname)
	fmt.Printf("CPU Cores:      %d\n", n.CPUCores)
	fmt.Printf("OS:             %s\n", n.OS)
	fmt.Printf("Architecture:   %s\n", n.Architecture)
	fmt.Printf("Status:         %s\n", n.Status)
	fmt.Printf("Last Heartbeat: %s\n", n.LastHeartbeat.Format(time.RFC3339))

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: runstack <command>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  status    Show control plane status")
		fmt.Println("  nodes     List all registered nodes")
		fmt.Println("  node <id> Show details of a specific node")
		return
	}

	switch os.Args[1] {
	case "status":
		if err := getStatus(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "nodes":
		if err := getNodes(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "node":
		if len(os.Args) < 3 {
			fmt.Println("Usage: runstack node <id>")
			os.Exit(1)
		}
		if err := getNode(os.Args[2]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
