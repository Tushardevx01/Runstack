package service

import (
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"testing"
)

func TestAppService_CreateApp(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()

	s := NewAppService(appReg, depReg)

	spec := application.AppSpec{Replicas: 3}
	app, err := s.CreateApp("test-app", spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if app.ActiveDeploymentID == "" {
		t.Fatalf("expected active deployment ID to be set")
	}

	dep, _ := depReg.Get(app.ActiveDeploymentID)
	if dep.Status != deployment.StatusActive {
		t.Errorf("expected new deployment to be ACTIVE, got %s", dep.Status)
	}
}

func TestAppService_UpdateApp(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()

	s := NewAppService(appReg, depReg)

	spec1 := application.AppSpec{Replicas: 3}
	app, _ := s.CreateApp("test-app", spec1)

	dep1ID := app.ActiveDeploymentID

	spec2 := application.AppSpec{Replicas: 5}
	app, err := s.UpdateApp(app.ID, spec2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if app.ActiveDeploymentID == dep1ID {
		t.Fatalf("expected active deployment ID to change")
	}

	dep1, _ := depReg.Get(dep1ID)
	if dep1.Status != deployment.StatusSuperseded {
		t.Errorf("expected old deployment to be SUPERSEDED, got %s", dep1.Status)
	}

	dep2, _ := depReg.Get(app.ActiveDeploymentID)
	if dep2.Status != deployment.StatusActive {
		t.Errorf("expected new deployment to be ACTIVE, got %s", dep2.Status)
	}
}
