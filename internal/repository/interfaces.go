package repository

import (
	"context"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
)

// InstanceRepository defines domain operations for instances.
type InstanceRepository interface {
	Create(ctx context.Context, inst *instance.Instance) error
	Get(ctx context.Context, id string) (*instance.Instance, error)
	// UpdateStatusWithFencing updates an instance only if the executionID matches (V1 ExecutionID Fencing).
	UpdateStatusWithFencing(ctx context.Context, id, executionID string, status instance.InstanceStatus) error
}

// NodeRepository defines domain operations for nodes.
type NodeRepository interface {
	Register(ctx context.Context, n *node.Node) error
	GetByToken(ctx context.Context, token string) (*node.Node, error)
}

// ApplicationRepository defines domain operations for applications.
type ApplicationRepository interface {
	Create(ctx context.Context, app *application.Application) error
	Get(ctx context.Context, id string) (*application.Application, error)
}

// DeploymentRepository defines domain operations for deployments.
type DeploymentRepository interface {
	Create(ctx context.Context, dep *deployment.Deployment) error
	Get(ctx context.Context, id string) (*deployment.Deployment, error)
}
