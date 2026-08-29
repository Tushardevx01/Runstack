package application

import (
	"testing"
)

func TestValidateProbe(t *testing.T) {
	tests := []struct {
		name  string
		probe *Probe
		valid bool
	}{
		{"nil probe", nil, true},
		{"valid HTTP", &Probe{Type: "HTTP", Path: "/", Port: 80, PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1}, true},
		{"valid TCP", &Probe{Type: "TCP", Port: 80, PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1}, true},
		{"invalid type", &Probe{Type: "UDP", Port: 80, PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1}, false},
		{"invalid port", &Probe{Type: "HTTP", Path: "/", Port: -1, PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1}, false},
		{"missing path", &Probe{Type: "HTTP", Port: 80, PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1}, false},
		{"invalid period", &Probe{Type: "TCP", Port: 80, PeriodSecs: 0, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1}, false},
		{"invalid timeout", &Probe{Type: "TCP", Port: 80, PeriodSecs: 1, TimeoutSecs: 0, SuccessThreshold: 1, FailureThreshold: 1}, false},
		{"invalid thresholds", &Probe{Type: "TCP", Port: 80, PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 0, FailureThreshold: 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProbe(tt.probe)
			if tt.valid && err != nil {
				t.Errorf("expected valid, got %v", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected invalid, got nil")
			}
		})
	}
}
