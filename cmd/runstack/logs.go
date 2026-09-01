package main

import (
	"encoding/json"
	"fmt"
	"github.com/Tushardevx01/runstack/internal/api"
	"io"
	"os"
)

func runLogs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: runstack logs <app-id> [--instance <instance-id>]")
	}

	appID := args[0]
	instanceID := ""

	if len(args) >= 3 && args[1] == "--instance" {
		instanceID = args[2]
	}

	// Fetch app status to print metadata
	statusResp, err := getClient().Get(fmt.Sprintf("/api/v1/apps/%s/status", appID))
	if err == nil && statusResp.StatusCode == 200 {
		var detail api.AppStatusDetail
		if json.NewDecoder(statusResp.Body).Decode(&detail) == nil {
			var targetInst *api.InstanceSummary
			if instanceID != "" {
				for _, inst := range detail.Instances {
					if inst.ID == instanceID {
						targetInst = &inst
						break
					}
				}
			} else {
				for _, inst := range detail.Instances {
					if inst.Status == "RUNNING" {
						targetInst = &inst
						break
					}
				}
			}

			if targetInst != nil {
				retainedStr := "no"
				if targetInst.Status == "CRASHED" {
					retainedStr = "yes"
				}
				fmt.Printf("Application: %s\n", detail.Application.Name)
				if detail.ActiveDeployment != nil {
					fmt.Printf("Deployment:  %s\n", detail.ActiveDeployment.ID)
				}
				fmt.Printf("Instance:    %s\n", targetInst.ID)
				fmt.Printf("Node:        %s\n", targetInst.NodeID)
				fmt.Printf("Status:      %s\n", targetInst.Status)
				fmt.Printf("Logs Retained: %s\n", retainedStr)
				fmt.Println("--------------------------------------------------------------------------------")
			}
		}
		statusResp.Body.Close()
	}

	url := fmt.Sprintf("/api/v1/apps/%s/logs", appID)
	if instanceID != "" {
		url += "?instance=" + instanceID
	}

	resp, err := getClient().Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fetch logs (status %d): %s", resp.StatusCode, string(b))
	}

	io.Copy(os.Stdout, resp.Body)
	return nil
}
