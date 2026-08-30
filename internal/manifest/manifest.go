package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/Tushardevx01/runstack/internal/application"
)

type ResourceSpec struct {
	CPU    float64 `yaml:"cpu" json:"cpu"`
	Memory int     `yaml:"memory" json:"memory"`
}

type EnvVar struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value" json:"value"`
}

type ProbeSpec struct {
	Path string `yaml:"path" json:"path"`
	Port int    `yaml:"port" json:"port"`
}

type ProbesSpec struct {
	Readiness *ProbeSpec `yaml:"readiness,omitempty" json:"readiness,omitempty"`
	Liveness  *ProbeSpec `yaml:"liveness,omitempty" json:"liveness,omitempty"`
}

type ServiceSpec struct {
	Port int `yaml:"port" json:"port"`
}

type DomainSpec struct {
	Name string `yaml:"name" json:"name"`
	TLS  bool   `yaml:"tls" json:"tls"`
}

type RolloutSpec struct {
	MaxSurge       int `yaml:"max_surge" json:"max_surge"`
	MaxUnavailable int `yaml:"max_unavailable" json:"max_unavailable"`
}

type Manifest struct {
	Name      string       `yaml:"name" json:"name"`
	Image     string       `yaml:"image" json:"image"`
	Replicas  int          `yaml:"replicas" json:"replicas"`
	Command   []string     `yaml:"command,omitempty" json:"command,omitempty"`
	Args      []string     `yaml:"args,omitempty" json:"args,omitempty"`
	Resources ResourceSpec `yaml:"resources" json:"resources"`
	Env       []EnvVar     `yaml:"env,omitempty" json:"env,omitempty"`
	Secrets   []string     `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Probes    *ProbesSpec  `yaml:"probes,omitempty" json:"probes,omitempty"`
	Service   *ServiceSpec `yaml:"service,omitempty" json:"service,omitempty"`
	Domains   []DomainSpec `yaml:"domains,omitempty" json:"domains,omitempty"`
	Rollout   *RolloutSpec `yaml:"rollout,omitempty" json:"rollout,omitempty"`
}

var nameRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

func (m *Manifest) Validate() error {
	if m.Name == "" || !nameRegex.MatchString(m.Name) {
		return errors.New("invalid or missing application name (must be a-z0-9-)")
	}
	if m.Image == "" {
		return errors.New("image is required")
	}
	if m.Replicas < 0 {
		return errors.New("replicas cannot be negative")
	}
	if m.Resources.CPU <= 0 || m.Resources.Memory <= 0 {
		return errors.New("resources (cpu, memory) must be positive")
	}

	if m.Probes != nil {
		if m.Probes.Readiness != nil && (m.Probes.Readiness.Port <= 0 || m.Probes.Readiness.Path == "") {
			return errors.New("readiness probe must have a valid path and port")
		}
		if m.Probes.Liveness != nil && (m.Probes.Liveness.Port <= 0 || m.Probes.Liveness.Path == "") {
			return errors.New("liveness probe must have a valid path and port")
		}
	}

	if m.Service != nil && m.Service.Port <= 0 {
		return errors.New("service port must be greater than 0")
	}

	if len(m.Domains) > 0 && m.Service == nil {
		return errors.New("domains require a service to be defined")
	}

	for _, d := range m.Domains {
		if d.Name == "" {
			return errors.New("domain name cannot be empty")
		}
	}

	if m.Rollout != nil {
		if m.Rollout.MaxSurge < 0 || m.Rollout.MaxUnavailable < 0 {
			return errors.New("rollout parameters cannot be negative")
		}
	}

	return nil
}

// Hash Deployment computes a sha256 of all fields that require an immutable Deployment change.
// Replicas, Service, and Domains DO NOT trigger a new Deployment.
func (m *Manifest) HashDeployment() string {
	spec := m.ToDeploymentSpec()
	b, _ := json.Marshal(spec)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (m *Manifest) ToDeploymentSpec() application.AppSpec {
	env := make(map[string]string)
	for _, e := range m.Env {
		env[e.Name] = e.Value
	}
	for _, sec := range m.Secrets {
		env[sec] = "secret:" + sec
	}

	var rp, lp *application.Probe
	if m.Probes != nil {
		if m.Probes.Readiness != nil {
			rp = &application.Probe{Path: m.Probes.Readiness.Path, Port: m.Probes.Readiness.Port}
		}
		if m.Probes.Liveness != nil {
			lp = &application.Probe{Path: m.Probes.Liveness.Path, Port: m.Probes.Liveness.Port}
		}
	}

	return application.AppSpec{
		Image:   m.Image,
		Command: m.Command,
		Args:    m.Args,
		Resources: &application.ResourceRequirements{
			CPU:      m.Resources.CPU,
			MemoryMB: m.Resources.Memory,
		},
		Environment:    env,
		ReadinessProbe: rp,
		LivenessProbe:  lp,
	}
}
