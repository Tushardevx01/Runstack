package service

import (
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
)

type AppService struct {
	AppRegistry        *application.Registry
	DeploymentRegistry *deployment.Registry
	InstanceRegistry   *instance.Registry
}

func NewAppService(appReg *application.Registry, depReg *deployment.Registry, instReg *instance.Registry) *AppService {
	return &AppService{
		AppRegistry:        appReg,
		DeploymentRegistry: depReg,
		InstanceRegistry:   instReg,
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
		// In a persistent system we'd rollback. Here, it won't realistically fail.
		return app, err
	}

	// 3. Create Instances
	for i := 0; i < spec.Replicas; i++ {
		_, _ = s.InstanceRegistry.Create(app.ID, dep.ID)
	}

	// 4. Update App with ActiveDeploymentID
	app, err = s.AppRegistry.Update(app.ID, spec, dep.ID, application.StatusPending)
	return app, err
}

func (s *AppService) UpdateApp(id string, spec application.AppSpec) (application.Application, error) {
	// Ensure app exists
	app, err := s.AppRegistry.Get(id)
	if err != nil {
		return application.Application{}, err
	}

	// 1. Create New Deployment
	dep, err := s.DeploymentRegistry.Create(id, spec)
	if err != nil {
		return app, err
	}

	// 2. Create New Pending Instances
	for i := 0; i < spec.Replicas; i++ {
		_, _ = s.InstanceRegistry.Create(id, dep.ID)
	}

	// 3. Update App
	app, err = s.AppRegistry.Update(id, spec, dep.ID, application.StatusPending)

	// Note: Old instances for previous deployments remain.
	// A future reconciliation loop would terminate them.

	return app, err
}

func (s *AppService) GetApp(id string) (application.Application, error) {
	return s.AppRegistry.Get(id)
}

func (s *AppService) ListApps() []application.Application {
	return s.AppRegistry.List()
}
