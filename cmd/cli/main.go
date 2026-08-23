package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type StatusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: runstack <command>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  status    Show control plane status")
		return
	}

	switch os.Args[1] {
	case "status":
		if err := getStatus(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}