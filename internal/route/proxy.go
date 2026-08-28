package route

import (
	"context"
)

type Endpoint struct {
	IP   string
	Port int
}

type RouteRule struct {
	Host      string
	Path      string
	Endpoints []Endpoint
}

type ProxyProvider interface {
	// UpdateRoutes replaces the entire routing table atomically.
	UpdateRoutes(ctx context.Context, rules []RouteRule) error
}
