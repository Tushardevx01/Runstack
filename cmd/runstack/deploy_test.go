package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseRunstackConfig(t *testing.T) {
	configStr := `{
		"name": "test-app",
		"registry": "ghcr.io/test/test-app:latest",
		"replicas": 3,
		"ports": [{"container_port": 8080}]
	}`

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "runstack.json")
	if err := os.WriteFile(configFile, []byte(configStr), 0644); err != nil {
		t.Fatal(err)
	}

	b, _ := os.ReadFile(configFile)
	var config RunstackConfig
	if err := json.Unmarshal(b, &config); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if config.Name != "test-app" {
		t.Errorf("expected name test-app, got %s", config.Name)
	}
	if config.Registry != "ghcr.io/test/test-app:latest" {
		t.Errorf("expected registry ghcr.io/test/test-app:latest, got %s", config.Registry)
	}
	if config.Replicas != 3 {
		t.Errorf("expected replicas 3, got %d", config.Replicas)
	}
	if len(config.Ports) != 1 || config.Ports[0].ContainerPort != 8080 {
		t.Errorf("expected ports parsing to succeed")
	}
}
