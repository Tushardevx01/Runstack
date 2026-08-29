package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/Tushardevx01/runstack/internal/runtime"
)

type DockerRuntime struct{}

func New() *DockerRuntime {
	return &DockerRuntime{}
}

func containerName(instanceID string) string {
	return fmt.Sprintf("runstack-%s", instanceID)
}

func (d *DockerRuntime) Start(ctx context.Context, spec runtime.ContainerSpec) (runtime.ContainerInfo, error) {
	if spec.InstanceID == "" || spec.ExecutionID == "" || spec.Image == "" {
		return runtime.ContainerInfo{}, runtime.ErrInvalidSpec
	}

	if strings.HasPrefix(spec.Image, "-") {
		return runtime.ContainerInfo{}, fmt.Errorf("%w: image cannot start with a dash", runtime.ErrInvalidSpec)
	}
	if strings.HasPrefix(spec.InstanceID, "-") {
		return runtime.ContainerInfo{}, fmt.Errorf("%w: instance ID cannot start with a dash", runtime.ErrInvalidSpec)
	}

	name := containerName(spec.InstanceID)

	// First, check if it already exists to handle idempotency
	info, err := d.inspect(ctx, name)
	if err == nil {
		// It exists. Check identity.
		if info.ExecutionID != spec.ExecutionID {
			return runtime.ContainerInfo{}, runtime.ErrContainerConflict
		}

		// Note: we do NOT restart exited containers. If it exited, it is crashed.
		// The CP requires a new instance for replacements.
		// If it's "created", it might be still starting up, or docker run failed before start.
		// In Milestone 8, we just return its current state so the Agent can report it.
		return runtime.ContainerInfo{
			ContainerID: info.ID,
			State:       info.State,
		}, nil
	} else if !errorsIsNotFound(err) {
		return runtime.ContainerInfo{}, err
	}

	// It doesn't exist, create and start it (docker run)
	args := []string{"run", "-d", "--name", name}

	// Labels for ownership
	args = append(args, "-l", "runstack.instance.id="+spec.InstanceID)
	args = append(args, "-l", "runstack.execution.id="+spec.ExecutionID)
	if spec.ApplicationID != "" {
		args = append(args, "-l", "runstack.application.id="+spec.ApplicationID)
	}
	if spec.DeploymentID != "" {
		args = append(args, "-l", "runstack.deployment.id="+spec.DeploymentID)
	}

	if spec.CPU > 0 {
		args = append(args, fmt.Sprintf("--cpus=%f", spec.CPU))
	}
	if spec.MemoryMB > 0 {
		args = append(args, fmt.Sprintf("--memory=%dm", spec.MemoryMB))

	}

	// Environment
	for k, v := range spec.Environment {
		if strings.Contains(k, "=") || strings.HasPrefix(k, "-") {
			return runtime.ContainerInfo{}, fmt.Errorf("%w: invalid environment key %q", runtime.ErrInvalidSpec, k)
		}
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Ports
	for _, p := range spec.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", p.External, p.Internal))
	}

	// Image
	args = append(args, spec.Image)

	// Command
	args = append(args, spec.Command...)

	// Args
	args = append(args, spec.Args...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "Conflict") {
			// Race condition: another process created it. We recursively call Start.
			// Or just return error for now to keep it simple, it will be retried.
			return runtime.ContainerInfo{}, fmt.Errorf("docker run conflict: %s", stderr.String())
		}
		return runtime.ContainerInfo{}, fmt.Errorf("docker run failed: %w (stderr: %s)", err, stderr.String())
	}

	containerID := strings.TrimSpace(string(out))

	return runtime.ContainerInfo{
		ContainerID: containerID,
		State:       runtime.StateRunning,
	}, nil
}

func (d *DockerRuntime) Stop(ctx context.Context, containerID string) error {
	// Idempotent stop. If it's already stopped, docker stop might just return ok, or we can ignore errors if it doesn't exist.
	// But let's verify ownership first.
	info, err := d.inspect(ctx, containerID)
	if err != nil {
		if errorsIsNotFound(err) {
			return nil // Already gone
		}
		return err
	}

	// Verify it's a runstack container
	if info.InstanceID == "" {
		return runtime.ErrContainerConflict
	}

	cmd := exec.CommandContext(ctx, "docker", "stop", "-t", "10", containerID)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// If it's already stopped, docker might return error or not. Let's inspect again.
		infoAfter, _ := d.inspect(ctx, containerID)
		if infoAfter.State == runtime.StateExited || infoAfter.State == runtime.StateUnknown {
			return nil
		}
		return fmt.Errorf("docker stop failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

func (d *DockerRuntime) Status(ctx context.Context, containerID string) (runtime.ContainerState, error) {
	info, err := d.inspect(ctx, containerID)
	if err != nil {
		if errorsIsNotFound(err) {
			return runtime.StateUnknown, runtime.ErrContainerNotFound
		}
		return runtime.StateUnknown, err
	}

	// Verify it's a runstack container
	if info.InstanceID == "" {
		return runtime.StateUnknown, runtime.ErrContainerConflict
	}

	return info.State, nil
}

func (d *DockerRuntime) Remove(ctx context.Context, containerID string) error {
	info, err := d.inspect(ctx, containerID)
	if err != nil {
		if errorsIsNotFound(err) {
			return nil
		}
		return err
	}

	// Verify it's a runstack container
	if info.InstanceID == "" {
		return runtime.ErrContainerConflict
	}

	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker rm failed: %w", err)
	}
	return nil
}

type inspectInfo struct {
	ID          string
	State       runtime.ContainerState
	InstanceID  string
	ExecutionID string
	OOMKilled   bool
}

func (d *DockerRuntime) Logs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	info, err := d.inspect(ctx, containerID)
	if err != nil {
		return nil, err
	}
	if info.InstanceID == "" {
		return nil, fmt.Errorf("container is not managed by RunStack")
	}

	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "100", containerID)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker logs failed: %w (stderr: %s)", err, stderr.String())
	}
	return io.NopCloser(bytes.NewReader(out.Bytes())), nil
}

func (d *DockerRuntime) inspect(ctx context.Context, idOrName string) (inspectInfo, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", idOrName)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "No such object") {
			return inspectInfo{}, runtime.ErrContainerNotFound
		}
		return inspectInfo{}, fmt.Errorf("inspect failed: %w (stderr: %s)", err, stderr.String())
	}

	var inspectData []struct {
		Id    string `json:"Id"`
		State struct {
			Status    string `json:"Status"`
			OOMKilled bool   `json:"OOMKilled"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}

	if err := json.Unmarshal(out.Bytes(), &inspectData); err != nil {
		return inspectInfo{}, fmt.Errorf("failed to parse inspect json: %w", err)
	}

	if len(inspectData) == 0 {
		return inspectInfo{}, runtime.ErrContainerNotFound
	}

	data := inspectData[0]

	state := runtime.StateUnknown
	switch data.State.Status {
	case "running":
		state = runtime.StateRunning
	case "exited", "dead":
		state = runtime.StateExited
	}

	labels := data.Config.Labels

	return inspectInfo{
		ID:          data.Id,
		State:       state,
		InstanceID:  labels["runstack.instance.id"],
		ExecutionID: labels["runstack.execution.id"],
		OOMKilled:   data.State.OOMKilled,
	}, nil
}

func errorsIsNotFound(err error) bool {
	if errors.Is(err, runtime.ErrContainerNotFound) {
		return true
	}
	// "docker inspect" returns exit status 1 and error msg containing "No such object"
	if err != nil && strings.Contains(err.Error(), "exit status 1") {
		return true
	}
	return false
}
