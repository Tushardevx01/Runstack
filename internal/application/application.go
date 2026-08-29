package application

import (
	"time"
)

type AppStatus string

const (
	StatusPending   AppStatus = "PENDING"
	StatusDeploying AppStatus = "DEPLOYING"
	StatusReady     AppStatus = "READY"
	StatusDegraded  AppStatus = "DEGRADED"
	StatusStopped   AppStatus = "STOPPED"
)

type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port,omitempty"`
	Protocol      string `json:"protocol,omitempty"` // e.g., "tcp", "udp"
}

type RolloutStrategyType string

const (
	RolloutStrategyImmediate     RolloutStrategyType = "Immediate"
	RolloutStrategyRollingUpdate RolloutStrategyType = "RollingUpdate"
)

type RolloutStrategy struct {
	Type           RolloutStrategyType `json:"type"`
	MaxSurge       int                 `json:"max_surge"`
	MaxUnavailable int                 `json:"max_unavailable"`
}

type Probe struct {
	Type             string `json:"type"` // "HTTP" or "TCP"
	Path             string `json:"path,omitempty"`
	Port             int    `json:"port"`
	InitialDelaySecs int    `json:"initial_delay_secs,omitempty"`
	PeriodSecs       int    `json:"period_secs,omitempty"`
	TimeoutSecs      int    `json:"timeout_secs,omitempty"`
	SuccessThreshold int    `json:"success_threshold,omitempty"`
	FailureThreshold int    `json:"failure_threshold,omitempty"`
}

type AppSpec struct {
	Image          string            `json:"image"`
	Command        []string          `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	Ports          []PortMapping     `json:"ports,omitempty"`
	Replicas       int               `json:"replicas"`
	Strategy       *RolloutStrategy  `json:"strategy,omitempty"`
	ReadinessProbe *Probe            `json:"readiness_probe,omitempty"`
	LivenessProbe  *Probe            `json:"liveness_probe,omitempty"`
}

type Application struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Spec               AppSpec   `json:"spec"`
	ActiveDeploymentID string    `json:"active_deployment_id,omitempty"`
	Status             AppStatus `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// DeepCopy creates a complete copy of the Application.
func (a *Application) DeepCopy() Application {
	copy := *a

	// Deep copy Environment
	if a.Spec.Environment != nil {
		copy.Spec.Environment = make(map[string]string, len(a.Spec.Environment))
		for k, v := range a.Spec.Environment {
			copy.Spec.Environment[k] = v
		}
	}

	// Deep copy Ports
	if a.Spec.Ports != nil {
		copy.Spec.Ports = make([]PortMapping, len(a.Spec.Ports))
		for i, p := range a.Spec.Ports {
			copy.Spec.Ports[i] = p // PortMapping is a value type, safe to copy
		}
	}

	// Deep copy Command
	if a.Spec.Command != nil {
		copy.Spec.Command = make([]string, len(a.Spec.Command))
		for i, v := range a.Spec.Command {
			copy.Spec.Command[i] = v
		}
	}

	// Deep copy Args
	if a.Spec.Args != nil {
		copy.Spec.Args = make([]string, len(a.Spec.Args))
		for i, v := range a.Spec.Args {
			copy.Spec.Args[i] = v
		}
	}

	if a.Spec.Strategy != nil {
		s := *a.Spec.Strategy
		copy.Spec.Strategy = &s
	}

	return copy
}
