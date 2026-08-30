package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/Tushardevx01/runstack/internal/application"
)

type RunstackConfig struct {
	Name        string            `json:"name"`
	Registry    string            `json:"registry"`
	Environment map[string]string `json:"environment,omitempty"`
	Ports       []struct {
		ContainerPort int `json:"container_port"`
		HostPort      int `json:"host_port,omitempty"`
	} `json:"ports,omitempty"`
	Replicas int     `json:"replicas,omitempty"`
	CPU      float64 `json:"cpu,omitempty"`
	MemoryMB int     `json:"memory_mb,omitempty"`
}

func runDeploy(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	cpuFlag := fs.Float64("cpu", 0, "CPU cores to reserve (e.g. 1.0)")
	memFlag := fs.Int("memory", 0, "Memory to reserve in MB (e.g. 512)")
	fs.Parse(args)

	b, err := os.ReadFile("runstack.json")
	if err != nil {
		return fmt.Errorf("failed to read runstack.json: %w", err)
	}

	var config RunstackConfig
	if err := json.Unmarshal(b, &config); err != nil {
		return fmt.Errorf("invalid runstack.json: %w", err)
	}

	if config.Name == "" || config.Registry == "" {
		return fmt.Errorf("name and registry are required in runstack.json")
	}

	if strings.HasPrefix(config.Registry, "-") {
		return fmt.Errorf("registry cannot start with a dash")
	}

	// Override with CLI flags if provided
	if *cpuFlag > 0 {
		config.CPU = *cpuFlag
	}
	if *memFlag > 0 {
		config.MemoryMB = *memFlag
	}

	if config.CPU < 0 {
		return fmt.Errorf("cpu must be non-negative")
	}
	if config.MemoryMB < 0 {
		return fmt.Errorf("memory must be non-negative")
	}

	fmt.Println("Building image...")

	buildArgs := []string{"build", "-t", config.Registry, "."}
	buildCmd := exec.Command("docker", buildArgs...)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Println("Pushing image...")
	pushArgs := []string{"push", config.Registry}
	pushCmd := exec.Command("docker", pushArgs...)
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	inspectArgs := []string{"inspect", "--format={{json .RepoDigests}}", config.Registry}
	inspectCmd := exec.Command("docker", inspectArgs...)
	out, err := inspectCmd.Output()
	if err != nil || len(bytes.TrimSpace(out)) == 0 || string(bytes.TrimSpace(out)) == "null" {
		return fmt.Errorf("failed to get image digest. Ensure registry returns RepoDigests: %w", err)
	}

	var digests []string
	if err := json.Unmarshal(bytes.TrimSpace(out), &digests); err != nil {
		return fmt.Errorf("failed to parse RepoDigests: %w", err)
	}

	if len(digests) == 0 {
		return fmt.Errorf("no RepoDigests found for image")
	}

	baseRepo := config.Registry
	if idx := strings.LastIndex(baseRepo, ":"); idx != -1 && !strings.Contains(baseRepo[idx:], "/") {
		baseRepo = baseRepo[:idx]
	}

	var digest string
	for _, d := range digests {
		if strings.HasPrefix(d, baseRepo+"@sha256:") {
			digest = d
			break
		}
	}

	if digest == "" {
		digest = digests[0]
	}

	if !strings.Contains(digest, "@sha256:") {
		return fmt.Errorf("invalid digest format returned: %s", digest)
	}

	fmt.Printf("Image digest resolved: %s\n", digest)

	var ports []application.PortMapping
	for _, p := range config.Ports {
		ports = append(ports, application.PortMapping{
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
		})
	}

	replicas := config.Replicas
	if replicas == 0 {
		replicas = 1
	}

	spec := application.AppSpec{
		Image:       digest,
		Environment: config.Environment,
		Ports:       ports,
		Replicas:    replicas,
	}

	if config.CPU > 0 || config.MemoryMB > 0 {
		spec.Resources = &application.ResourceRequirements{
			CPU:      config.CPU,
			MemoryMB: config.MemoryMB,
		}
	}

	payload := map[string]interface{}{
		"spec": spec,
	}
	pb, _ := json.Marshal(payload)

	url := fmt.Sprintf("/api/v1/apps/%s/deploy", config.Name)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(pb))
	req.Header.Set("Content-Type", "application/json")

	fmt.Println("Deploying to Control Plane...")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return fmt.Errorf("deploy failed: %s", errResp["error"])
		}
		return fmt.Errorf("deploy failed with status: %d", resp.StatusCode)
	}

	fmt.Println("Deployment successful!")
	return nil
}
