package route

import (
	"context"
)

type Endpoint struct {
	IP   string
	Port int
}

// ProxyProvider abstracts the underlying routing implementation.
type ProxyProvider interface {
	// UpdateRoute sets the desired healthy endpoints for a given Service.
	// It is strictly idempotent.
	UpdateRoute(ctx context.Context, srv Service, endpoints []Endpoint) error

	// RemoveRoute fully clears a Service from the routing table.
	RemoveRoute(ctx context.Context, serviceID string) error
}
