package runtime

import (
	"context"
	"errors"
	"io"
)

var (
	ErrContainerNotFound  = errors.New("container not found")
	ErrContainerConflict  = errors.New("container conflict (identity mismatch)")
	ErrRuntimeUnavailable = errors.New("container runtime unavailable")
	ErrInvalidSpec        = errors.New("invalid container spec")
)

type ContainerState string

const (
	StateRunning ContainerState = "running"
	StateExited  ContainerState = "exited"
	StateStopped ContainerState = "stopped"
	StateUnknown ContainerState = "unknown"
)

type PortMapping struct {
	Internal int
	External int
}

type ContainerSpec struct {
	InstanceID    string
	ExecutionID   string
	ApplicationID string
	DeploymentID  string
	Image         string
	Command       []string
	Args          []string
	Environment   map[string]string
	Ports         []PortMapping
}

type ContainerInfo struct {
	ContainerID string
	State       ContainerState
}

type ContainerRuntime interface {
	Start(ctx context.Context, spec ContainerSpec) (ContainerInfo, error)
	Stop(ctx context.Context, containerID string) error
	Status(ctx context.Context, containerID string) (ContainerState, error)
	Remove(ctx context.Context, containerID string) error
	Logs(ctx context.Context, containerID string) (io.ReadCloser, error)
}
