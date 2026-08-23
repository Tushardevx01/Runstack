package scheduler

import (
	"context"
	"testing"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/route"
)

type mockProxy struct {
	routes map[string][]route.Endpoint
}

func (m *mockProxy) UpdateRoute(ctx context.Context, srv route.Service, endpoints []route.Endpoint) error {
	m.routes[srv.ID] = endpoints
	return nil
}

func (m *mockProxy) RemoveRoute(ctx context.Context, serviceID string) error {
	delete(m.routes, serviceID)
	return nil
}

func TestRoutingReconciler_DrainAndCrash(t *testing.T) {
	appReg := application.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()
	routeReg := route.NewRegistry()
	proxy := &mockProxy{routes: make(map[string][]route.Endpoint)}

	reconciler := NewRoutingReconciler(appReg, instReg, nodeReg, routeReg, proxy)
	ctx := context.Background()

	// Setup Node
	nodeReg.Register(node.Node{ID: "node1", IPAddress: "10.0.0.1"})

	// Setup App & Route
	app, _ := appReg.Create("", application.AppSpec{})
	srv, _ := routeReg.Create(app.ID, "test.local", "/", 8080, route.ProtocolHTTP)

	// Setup Instances
	iA, _ := instReg.Create(app.ID, "dep1")
	iB, _ := instReg.Create(app.ID, "dep1")
	iC, _ := instReg.Create(app.ID, "dep1")

	ports := []instance.PortMapping{{Internal: 8080, External: 30001}}
	instReg.UpdateState(iA.ID, instance.StatusAssigned, "node1", "")
	iA, _ = instReg.Claim(iA.ID, "node1")
	iA, _ = instReg.ReportStatus(iA.ID, "node1", iA.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "c1", ports)

	ports2 := []instance.PortMapping{{Internal: 8080, External: 30002}}
	instReg.UpdateState(iB.ID, instance.StatusAssigned, "node1", "")
	iB, _ = instReg.Claim(iB.ID, "node1")
	iB, _ = instReg.ReportStatus(iB.ID, "node1", iB.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "c2", ports2)

	ports3 := []instance.PortMapping{{Internal: 8080, External: 30003}}
	instReg.UpdateState(iC.ID, instance.StatusAssigned, "node1", "")
	iC, _ = instReg.Claim(iC.ID, "node1")
	iC, _ = instReg.ReportStatus(iC.ID, "node1", iC.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "c3", ports3)

	// Initial Reconcile
	_ = reconciler.Reconcile(ctx)
	if len(proxy.routes[srv.ID]) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(proxy.routes[srv.ID]))
	}

	// Drain iA and iB
	instReg.MarkDraining(iA.ID)
	instReg.MarkDraining(iB.ID)

	// Reconcile after drain
	_ = reconciler.Reconcile(ctx)
	if len(proxy.routes[srv.ID]) != 1 {
		t.Fatalf("expected 1 endpoint after drain, got %d", len(proxy.routes[srv.ID]))
	}
	if proxy.routes[srv.ID][0].Port != 30003 {
		t.Fatalf("expected port 30003 to remain active")
	}

	// Crash iC
	instReg.ReportStatus(iC.ID, "node1", iC.ExecutionID, instance.StatusCrashed, instance.HealthUnknown, "c3", nil)
	_ = reconciler.Reconcile(ctx)
	if len(proxy.routes[srv.ID]) != 0 {
		t.Fatalf("expected 0 endpoints after crash, got %d", len(proxy.routes[srv.ID]))
	}
}
