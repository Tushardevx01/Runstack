package scheduler

import (
	"context"
	"sync"
	"testing"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/route"
)

type mockProxy struct {
	mu    sync.Mutex
	rules []route.RouteRule
}

func (m *mockProxy) UpdateRoutes(ctx context.Context, rules []route.RouteRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules = rules
	return nil
}

func TestRoutingReconciler_Reconcile(t *testing.T) {
	appReg := application.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()
	routeReg := route.NewRegistry()
	domainReg := ingress.NewDomainRegistry()
	ingressReg := ingress.NewIngressRegistry()
	proxy := &mockProxy{}

	reconciler := NewRoutingReconciler(appReg, instReg, nodeReg, routeReg, domainReg, ingressReg, proxy)

	appA, _ := appReg.Create("app-a", application.AppSpec{})
	appB, _ := appReg.Create("app-b", application.AppSpec{})

	n := node.Node{ID: "node1", IPAddress: "192.168.1.100"}
	nodeReg.Register(n, "")

	// App A Instance
	instA, _ := instReg.Create(appA.ID, "depA")
	instReg.UpdateState(instA.ID, instance.StatusAssigned, "node1", "")
	claimedInst, _ := instReg.Claim(instA.ID, "node1")
	instReg.ReportStatus(instA.ID, "node1", claimedInst.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "cnt1", []instance.PortMapping{{Internal: 8080, External: 30000}})

	// Create Domain and Service for App A
	domain, _ := domainReg.Create("api.example.com", appA.ID, false)
	srv, _ := routeReg.Create(appA.ID, 8080, route.ProtocolHTTP)
	ingressReg.Create(domain.ID, srv.ID, "/")

	err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	proxy.mu.Lock()
	rules := proxy.rules
	proxy.mu.Unlock()

	if len(rules) != 1 {
		t.Fatalf("expected 1 route rule, got %d", len(rules))
	}

	if rules[0].Host != "api.example.com" {
		t.Errorf("expected host api.example.com, got %s", rules[0].Host)
	}
	if len(rules[0].Endpoints) != 1 || rules[0].Endpoints[0].Port != 30000 {
		t.Errorf("expected endpoint on port 30000, got %v", rules[0].Endpoints)
	}

	// Security Test: Cross-Application Domain mapping
	srvB, _ := routeReg.Create(appB.ID, 8080, route.ProtocolHTTP)
	ingressReg.Create(domain.ID, srvB.ID, "/b")

	err = reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	proxy.mu.Lock()
	rules2 := proxy.rules
	proxy.mu.Unlock()

	// Reconciler must ignore the cross-application mapping
	if len(rules2) != 1 {
		t.Fatalf("expected 1 route rule due to cross-app rejection, got %d", len(rules2))
	}
}
