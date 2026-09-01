package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/manifest"
)

func applyCommand(args []string) error {
	applyCmd := flag.NewFlagSet("apply", flag.ExitOnError)
	file := applyCmd.String("f", "runstack.yaml", "Path to manifest file")
	wait := applyCmd.Bool("wait", false, "Wait for rollout to complete")
	timeout := applyCmd.Duration("timeout", 5*time.Minute, "Timeout for wait")
	applyCmd.Parse(args)

	return submitManifest("POST", "/api/v1/apply", *file, *wait, *timeout)
}

func diffCommand(args []string) error {
	diffCmd := flag.NewFlagSet("diff", flag.ExitOnError)
	file := diffCmd.String("f", "runstack.yaml", "Path to manifest file")
	diffCmd.Parse(args)

	return submitManifest("POST", "/api/v1/diff", *file, false, 0)
}

func validateCommand(args []string) error {
	validateCmd := flag.NewFlagSet("validate", flag.ExitOnError)
	file := validateCmd.String("f", "runstack.yaml", "Path to manifest file")
	validateCmd.Parse(args)

	_, err := parseManifest(*file)
	if err == nil {
		fmt.Printf("Manifest '%s' is valid.\n", *file)
	}
	return err
}

func parseManifest(path string) (*manifest.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	var m manifest.Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true) // strict parsing

	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("yaml parse error: %v", err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

func submitManifest(method, path, file string, wait bool, timeout time.Duration) error {
	m, err := parseManifest(file)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}

	resp, err := getClient().Post(path, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if msg, ok := errResp["message"]; ok {
			return fmt.Errorf("server error (%d): %s", resp.StatusCode, msg)
		}
		// Try to read raw error string
		return fmt.Errorf("server error (%d)", resp.StatusCode)
	}

	var res apiApplyResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	fmt.Printf("Application: %s\nAction:      %s\n", m.Name, res.Action)
	if res.DeploymentID != "" {
		fmt.Printf("Deployment:  %s\n", res.DeploymentID)
	}

	if wait && res.DeploymentID != "" {
		fmt.Println("\nWaiting for rollout...")
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("timeout waiting for rollout")
			case <-ticker.C:
				statusResp, err := getClient().Get(fmt.Sprintf("/api/v1/apps/%s/status", m.Name))
				if err != nil {
					continue
				}

				var detail api.AppStatusDetail
				err = json.NewDecoder(statusResp.Body).Decode(&detail)
				statusResp.Body.Close()
				if err != nil {
					continue
				}

				if detail.ActiveDeployment != nil && detail.ActiveDeployment.ID == res.DeploymentID {
					rollout := detail.ActiveDeployment.RolloutStatus
					fmt.Printf("\rRollout state: %s (%d/%d Ready)  ", rollout, detail.ActiveDeployment.ReadyReplicas, detail.ActiveDeployment.DesiredReplicas)

					if rollout == deployment.RolloutCompleted {
						fmt.Println("\nRollout completed successfully.")
						return nil
					} else if rollout == deployment.RolloutFailed {
						fmt.Println("\nRollout failed.")
						if detail.ActiveDeployment.BlockedReason != "" {
							fmt.Printf("Reason: %s\n", detail.ActiveDeployment.BlockedReason)
						}
						return fmt.Errorf("rollout failed")
					} else if rollout == deployment.RolloutRolledBack {
						fmt.Println("\nRollout rolled back.")
						return fmt.Errorf("rollout rolled back")
					}
				}
			}
		}
	}

	return nil
}

type apiApplyResult struct {
	Action        string `json:"action"`
	ApplicationID string `json:"application_id"`
	DeploymentID  string `json:"deployment_id,omitempty"`
	Message       string `json:"message"`
}
