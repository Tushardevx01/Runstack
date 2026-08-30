package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/Tushardevx01/runstack/internal/manifest"
)

func applyCommand(args []string) error {
	applyCmd := flag.NewFlagSet("apply", flag.ExitOnError)
	file := applyCmd.String("f", "runstack.yaml", "Path to manifest file")
	applyCmd.Parse(args)

	return submitManifest("POST", "/api/v1/apply", *file)
}

func diffCommand(args []string) error {
	diffCmd := flag.NewFlagSet("diff", flag.ExitOnError)
	file := diffCmd.String("f", "runstack.yaml", "Path to manifest file")
	diffCmd.Parse(args)

	return submitManifest("POST", "/api/v1/diff", *file)
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

func submitManifest(method, path, file string) error {
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

	fmt.Printf("Application: %s\nAction:      %s\nMessage:     %s\n", m.Name, res.Action, res.Message)
	if res.DeploymentID != "" {
		fmt.Printf("Deployment:  %s\n", res.DeploymentID)
	}

	return nil
}

type apiApplyResult struct {
	Action        string `json:"action"`
	ApplicationID string `json:"application_id"`
	DeploymentID  string `json:"deployment_id,omitempty"`
	Message       string `json:"message"`
}
