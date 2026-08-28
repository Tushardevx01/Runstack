package integration

import (
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/service"
)

func TestSecret_RotationIdempotency(t *testing.T) {
	appRegistry := application.NewRegistry()
	depRegistry := deployment.NewRegistry()
	secRegistry := application.NewSecretRegistry()
	appSvc := service.NewAppService(appRegistry, depRegistry, secRegistry)

	// Create App
	app, _ := appRegistry.Create("my-app", application.AppSpec{})

	// Create Secret
	_, _ = secRegistry.Set(app.ID, "DB_PASS", "v1")

	// Deploy
	spec := application.AppSpec{
		Image: "my-image",
		Environment: map[string]string{
			"DB_PASS": "secret:DB_PASS",
		},
		Replicas: 1,
	}

	app, err := appSvc.DeployApp(app.ID, spec)
	if err != nil {
		t.Fatalf("failed to deploy: %v", err)
	}
	dep1 := app.ActiveDeploymentID

	// Deploy again without changing anything (should be idempotent)
	app, err = appSvc.DeployApp(app.ID, spec)
	if err != nil {
		t.Fatalf("failed to deploy: %v", err)
	}
	if app.ActiveDeploymentID != dep1 {
		t.Fatalf("expected idempotent deployment, got new %s != %s", app.ActiveDeploymentID, dep1)
	}

	// Change secret value (simulating rotation)
	// We need to wait a tiny bit to ensure UpdatedAt is strictly greater than dep1.CreatedAt
	// In reality time.Now() is enough if there's a small pause.
	time.Sleep(10 * time.Millisecond)
	_, _ = secRegistry.Set(app.ID, "DB_PASS", "v2")

	// Deploy again with SAME AppSpec reference (should trigger NEW deployment)
	app, err = appSvc.DeployApp(app.ID, spec)
	if err != nil {
		t.Fatalf("failed to deploy: %v", err)
	}

	if app.ActiveDeploymentID == dep1 {
		t.Fatalf("expected new deployment after secret rotation, but got same %s", dep1)
	}
}
