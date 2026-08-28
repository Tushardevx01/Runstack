package service

import (
	"testing"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
)

func TestAppService_DeployIdempotency(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	svc := NewAppService(appReg, depReg, application.NewSecretRegistry())

	spec := application.AppSpec{
		Image:    "ghcr.io/test@sha256:abcd",
		Replicas: 2,
	}

	app, err := svc.DeployApp("test-app", spec)
	if err != nil {
		t.Fatalf("failed to deploy: %v", err)
	}

	firstDepID := app.ActiveDeploymentID
	if firstDepID == "" {
		t.Fatalf("expected deployment ID")
	}

	// Deploy exact same spec
	app2, err := svc.DeployApp("test-app", spec)
	if err != nil {
		t.Fatalf("failed second deploy: %v", err)
	}

	if app2.ActiveDeploymentID != firstDepID {
		t.Errorf("expected idempotent deploy to return same deployment ID %s, got %s", firstDepID, app2.ActiveDeploymentID)
	}

	// Deploy changed spec
	spec.Image = "ghcr.io/test@sha256:efgh"
	app3, err := svc.DeployApp("test-app", spec)
	if err != nil {
		t.Fatalf("failed third deploy: %v", err)
	}

	if app3.ActiveDeploymentID == firstDepID {
		t.Errorf("expected new deployment ID after spec change")
	}
}
