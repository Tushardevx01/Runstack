package service

import (
	"errors"
	"strings"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
)

type AppService struct {
	AppRegistry        *application.Registry
	DeploymentRegistry *deployment.Registry
	SecretRegistry     *application.SecretRegistry
}

func NewAppService(appReg *application.Registry, depReg *deployment.Registry, secReg *application.SecretRegistry) *AppService {
	return &AppService{
		AppRegistry:        appReg,
		DeploymentRegistry: depReg,
		SecretRegistry:     secReg,
	}
}

func (s *AppService) CreateApp(name string, spec application.AppSpec) (application.Application, error) {
	application.EnsureDefaultStrategy(&spec)
	if err := application.ValidateAppSpec(spec); err != nil {
		return application.Application{}, err
	}

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

	if dep.ApplicationID != app.ID {
		return application.Application{}, errors.New("forbidden: deployment does not belong to this application")
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

func (s *AppService) DeployApp(id string, spec application.AppSpec) (application.Application, error) {
	app, err := s.AppRegistry.Get(id)
	if err != nil && err == application.ErrNotFound {
		app, err = s.AppRegistry.GetByName(id)
		if err == nil {
			id = app.ID
		}
	}
	if err != nil {
		if err == application.ErrNotFound {
			return s.CreateApp(id, spec)
		}
		return application.Application{}, err
	}

	application.EnsureDefaultStrategy(&spec)
	if err := application.ValidateAppSpec(spec); err != nil {
		return application.Application{}, err
	}

	if app.ActiveDeploymentID != "" {
		activeDep, err := s.DeploymentRegistry.Get(app.ActiveDeploymentID)
		if err == nil {
			if s.specsMatch(id, activeDep, spec) {
				return app, nil
			}
		}
	}

	dep, err := s.DeploymentRegistry.Create(id, spec)
	if err != nil {
		return app, err
	}

	oldDepID := app.ActiveDeploymentID

	app, err = s.AppRegistry.Update(id, spec, dep.ID, application.StatusPending)

	if oldDepID != "" && oldDepID != dep.ID {
		s.DeploymentRegistry.UpdateState(oldDepID, deployment.StatusSuperseded)
	}
	s.DeploymentRegistry.UpdateState(dep.ID, deployment.StatusActive)

	return app, err
}

func (s *AppService) specsMatch(appID string, activeDep deployment.Deployment, b application.AppSpec) bool {
	a := activeDep.SpecSnapshot
	if a.Image != b.Image || len(a.Command) != len(b.Command) || len(a.Args) != len(b.Args) || len(a.Environment) != len(b.Environment) || len(a.Ports) != len(b.Ports) || a.Replicas != b.Replicas {
		return false
	}
	for i := range a.Command {
		if a.Command[i] != b.Command[i] {
			return false
		}
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	for i := range a.Ports {
		if a.Ports[i] != b.Ports[i] {
			return false
		}
	}
	for k, v := range a.Environment {
		if b.Environment[k] != v {
			return false
		}
		// If it's a secret, check if the secret was updated after this deployment was created
		if strings.HasPrefix(v, "secret:") {
			secretName := strings.TrimPrefix(v, "secret:")
			secMetadata, err := s.SecretRegistry.GetByName(appID, secretName)
			if err == nil { // if not found, we don't force a new deployment, let it fail on claim
				if secMetadata.UpdatedAt.After(activeDep.CreatedAt) {
					return false // Secret was updated, require new deployment
				}
			}
		}
	}
	if (a.Strategy == nil) != (b.Strategy == nil) {
		return false
	}
	if a.Strategy != nil && *a.Strategy != *b.Strategy {
		return false
	}
	return true
}
