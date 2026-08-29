package service

import (
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"testing"
)

func TestAppService_RollbackApp_IDOR(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	secReg := application.NewSecretRegistry()
	svc := NewAppService(appReg, depReg, secReg)

	// App A
	specA := application.AppSpec{Image: "image-a", Replicas: 1}
	appA, _ := svc.CreateApp("AppA", specA)
	appA, _ = svc.DeployApp(appA.ID, specA)
	depA := appA.ActiveDeploymentID

	// App B
	specB := application.AppSpec{Image: "image-b", Replicas: 1}
	appB, _ := svc.CreateApp("AppB", specB)
	appB, _ = svc.DeployApp(appB.ID, specB)
	depB := appB.ActiveDeploymentID

	// App A rolling back to App B
	_, err := svc.RollbackApp(appA.ID, depB, false)
	if err == nil || err.Error() != "forbidden: deployment does not belong to this application" {
		t.Fatalf("expected forbidden error, got: %v", err)
	}

	// App A rolling back to App A
	_, err = svc.RollbackApp(appA.ID, depA, false)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// Nonexistent deployment
	_, err = svc.RollbackApp(appA.ID, "fake", false)
	if err == nil {
		t.Fatalf("expected error for nonexistent deployment")
	}
}
