package main

import (
	"fmt"
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
