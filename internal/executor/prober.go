package executor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/instance"
)

func (e *InstanceExecutor) startProbers(inst instance.Instance, mappedPort int, spec application.AppSpec) {
	e.proberMu.Lock()
	defer e.proberMu.Unlock()

	if _, exists := e.probers[inst.ID]; exists {
		return // already probing
	}

	ctx, cancel := context.WithCancel(e.ctx)
	e.probers[inst.ID] = cancel

	if spec.ReadinessProbe == nil && spec.LivenessProbe == nil {
		// No probes configured. Default to HEALTHY immediately if not already reported.
		if inst.Health != instance.HealthHealthy {
			_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusRunning, instance.HealthHealthy, inst.ContainerID, nil)
		}
		return
	}

	if spec.ReadinessProbe != nil {
		go e.runProbe(ctx, inst, mappedPort, spec.ReadinessProbe, true)
	} else {
		// No readiness probe, but liveness might be present. Still default to HEALTHY.
		if inst.Health != instance.HealthHealthy {
			_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusRunning, instance.HealthHealthy, inst.ContainerID, nil)
		}
	}

	if spec.LivenessProbe != nil {
		go e.runProbe(ctx, inst, mappedPort, spec.LivenessProbe, false)
	}
}

func (e *InstanceExecutor) stopProbers(instanceID string) {
	e.proberMu.Lock()
	defer e.proberMu.Unlock()
	if cancel, exists := e.probers[instanceID]; exists {
		cancel()
		delete(e.probers, instanceID)
	}
}

func (e *InstanceExecutor) runProbe(ctx context.Context, inst instance.Instance, mappedPort int, probe *application.Probe, isReadiness bool) {
	if probe.InitialDelaySecs > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(probe.InitialDelaySecs) * time.Second):
		}
	}

	ticker := time.NewTicker(time.Duration(probe.PeriodSecs) * time.Second)
	defer ticker.Stop()

	successCount := 0
	failureCount := 0
	currentHealth := instance.HealthUnknown

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		success := doProbe(ctx, probe, mappedPort)

		if success {
			failureCount = 0
			successCount++
			if successCount >= probe.SuccessThreshold {
				if isReadiness && currentHealth != instance.HealthHealthy {
					currentHealth = instance.HealthHealthy
					_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusRunning, currentHealth, inst.ContainerID, nil)
				}
				// Liveness success doesn't need API report, it just means it doesn't crash.
			}
		} else {
			successCount = 0
			failureCount++
			if failureCount >= probe.FailureThreshold {
				if isReadiness && currentHealth != instance.HealthUnhealthy {
					currentHealth = instance.HealthUnhealthy
					_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusRunning, currentHealth, inst.ContainerID, nil)
				}
				if !isReadiness {
					// Liveness failure. Crash the container.
					_ = e.APIClient.ReportInstanceStatus(inst.ID, e.NodeID, inst.ExecutionID, instance.StatusCrashed, instance.HealthUnknown, inst.ContainerID, nil)
					// The monitorActive loop will see it's crashed on CP but running locally and stop it.
					// We also cancel ourselves.
					e.stopProbers(inst.ID)
					return
				}
			}
		}
	}
}

func doProbe(ctx context.Context, probe *application.Probe, mappedPort int) bool {
	timeout := time.Duration(probe.TimeoutSecs) * time.Second
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if probe.Type == "TCP" {
		addr := fmt.Sprintf("127.0.0.1:%d", mappedPort)
		dialer := net.Dialer{}
		conn, err := dialer.DialContext(probeCtx, "tcp", addr)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	} else if probe.Type == "HTTP" {
		url := fmt.Sprintf("http://127.0.0.1:%d%s", mappedPort, probe.Path)
		req, err := http.NewRequestWithContext(probeCtx, "GET", url, nil)
		if err != nil {
			return false
		}
		// Do not follow external redirects, but standard Go client follows redirects by default.
		// That's fine as long as they resolve internally, but to be safe we could restrict it.
		// V1 constraint: simple HTTP GET.
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return true
		}
		return false
	}
	return false
}
