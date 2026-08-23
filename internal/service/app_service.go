package service

import (
	"errors"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
)

type AppService struct {
	AppRegistry        *application.Registry
	DeploymentRegistry *deployment.Registry
}

func NewAppService(appReg *application.Registry, depReg *deployment.Registry) *AppService {
	return &AppService{
		AppRegistry:        appReg,
		DeploymentRegistry: depReg,
	}
}

func (s *AppService) CreateApp(name string, spec application.AppSpec) (application.Application, error) {
	// 1. Create App
	app, err := s.AppRegistry.Create(name, spec)
	if err != nil {
		return application.Application{}, err
	}

	// 2. Create Deployment
	dep, err := s.DeploymentRegistry.Create(app.ID, spec)
	if err != nil {
		return app, err
	}

	// 3. Update App with ActiveDeploymentID
	app, err = s.AppRegistry.Update(app.ID, spec, dep.ID, application.StatusPending)
	return app, err
}

func (s *AppService) UpdateApp(id string, spec application.AppSpec) (application.Application, error) {
	// Ensure app exists
	app, err := s.AppRegistry.Get(id)
	if err != nil {
		return application.Application{}, err
	}

	application.EnsureDefaultStrategy(&spec)
	if err := application.ValidateAppSpec(spec); err != nil {
		return application.Application{}, err
	}

	// 1. Create New Deployment
	dep, err := s.DeploymentRegistry.Create(id, spec)
	if err != nil {
		return app, err
	}

	// Capture old deployment ID before updating
	oldDepID := app.ActiveDeploymentID

	// 2. Update App with New ActiveDeploymentID
	app, err = s.AppRegistry.Update(id, spec, dep.ID, application.StatusPending)

	if oldDepID != "" && oldDepID != dep.ID {
		s.DeploymentRegistry.UpdateState(oldDepID, deployment.StatusSuperseded)
	}
	s.DeploymentRegistry.UpdateState(dep.ID, deployment.StatusActive)

	return app, err
}

func (s *AppService) GetApp(id string) (application.Application, error) {
	return s.AppRegistry.Get(id)
}

func (s *AppService) ListApps() []application.Application {
	return s.AppRegistry.List()
}

func (s *AppService) RollbackApp(id string, targetDeploymentID string, force bool) (application.Application, error) {
	app, err := s.AppRegistry.Get(id)
	if err != nil {
		return application.Application{}, err
	}

	dep, err := s.DeploymentRegistry.Get(targetDeploymentID)
	if err != nil {
		return application.Application{}, err
	}

	if !force && (dep.Degraded || dep.RolloutStatus == deployment.RolloutFailed) {
		return application.Application{}, errors.New("cannot rollback to a DEGRADED or FAILED deployment without force=true")
	}

	oldDepID := app.ActiveDeploymentID

	app, err = s.AppRegistry.Rollback(id, targetDeploymentID)
	if err != nil {
		return application.Application{}, err
	}

	if oldDepID != "" && oldDepID != targetDeploymentID {
		s.DeploymentRegistry.UpdateState(oldDepID, deployment.StatusSuperseded)
	}
	s.DeploymentRegistry.UpdateState(targetDeploymentID, deployment.StatusActive)

	// Reset rollout state for the target deployment if it was rolled back
	s.DeploymentRegistry.UpdateRolloutStatusOnly(targetDeploymentID, deployment.RolloutPending, "")

	return app, nil
}
