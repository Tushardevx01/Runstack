package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/manifest"
	"github.com/Tushardevx01/runstack/internal/route"
)

func setupApplyTest() (*api.ApplyHandler, *application.Registry, *deployment.Registry, *ingress.DomainRegistry, *application.SecretRegistry, *route.Registry) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	secretReg := application.NewSecretRegistry()
	domainReg := ingress.NewDomainRegistry()
	ingressReg := ingress.NewIngressRegistry()
	routeReg := route.NewRegistry()

	h := &api.ApplyHandler{
		AppRegistry:     appReg,
		DepRegistry:     depReg,
		SecretRegistry:  secretReg,
		DomainRegistry:  domainReg,
		IngressRegistry: ingressReg,
		ServiceRegistry: routeReg,
	}

	return h, appReg, depReg, domainReg, secretReg, routeReg
}

func doApply(h *api.ApplyHandler, m *manifest.Manifest) (*api.ApplyResult, int) {
	b, _ := json.Marshal(m)
	req := httptest.NewRequest("POST", "/api/v1/apply", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Apply(w, req)

	if w.Code >= 400 {
		return nil, w.Code
	}

	var res api.ApplyResult
	json.NewDecoder(w.Body).Decode(&res)
	return &res, w.Code
}

func TestApply_Idempotency(t *testing.T) {
	h, _, depReg, domainReg, _, _ := setupApplyTest()

	m := &manifest.Manifest{
		Name:      "test-app",
		Image:     "nginx:latest",
		Replicas:  3,
		Resources: manifest.ResourceSpec{CPU: 1, Memory: 512},
		Service:   &manifest.ServiceSpec{Port: 8080},
		Domains: []manifest.DomainSpec{
			{Name: "example.com", TLS: true},
		},
	}

	// First Apply
	res1, code := doApply(h, m)
	if code != http.StatusOK {
		t.Fatalf("first apply failed: %d", code)
	}
	if res1.Action != "created" {
		t.Errorf("expected created, got %s", res1.Action)
	}

	deps1 := depReg.ListByApplication(res1.ApplicationID)
	if len(deps1) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps1))
	}

	// Second Apply (Idempotent)
	res2, code := doApply(h, m)
	if code != http.StatusOK {
		t.Fatalf("second apply failed: %d", code)
	}
	if res2.Action != "unchanged" {
		t.Errorf("expected unchanged, got %s", res2.Action)

		dep1 := deps1[0]
		t.Logf("Hash 1: %s", dep1.Hash)
		t.Logf("Hash 2 expected: %s", m.HashDeployment())
	}

	deps2 := depReg.ListByApplication(res2.ApplicationID)
	if len(deps2) != 1 {
		t.Errorf("expected still 1 deployment, got %d", len(deps2))
	}

	// Verify Domains
	domains := domainReg.List()
	if len(domains) != 1 || domains[0].Name != "example.com" {
		t.Errorf("expected 1 domain 'example.com', got %v", domains)
	}

	// Change Domain -> Should Update but NOT create new deployment
	m.Domains[0].Name = "new.com"
	res3, code := doApply(h, m)
	if code != http.StatusOK {
		t.Fatalf("third apply failed: %d", code)
	}
	if res3.Action != "updated" {
		t.Errorf("expected updated, got %s", res3.Action)
	}

	deps3 := depReg.ListByApplication(res3.ApplicationID)
	if len(deps3) != 1 {
		t.Errorf("expected still 1 deployment after domain change, got %d", len(deps3))
	}

	// Verify Domain was pruned and replaced
	domains = domainReg.List()
	if len(domains) != 1 || domains[0].Name != "new.com" {
		t.Errorf("expected 1 domain 'new.com', got %v", domains)
	}
}

func TestApply_MissingSecret(t *testing.T) {
	h, _, _, _, _, _ := setupApplyTest()

	m := &manifest.Manifest{
		Name:      "test-app",
		Image:     "nginx:latest",
		Resources: manifest.ResourceSpec{CPU: 1, Memory: 512},
		Secrets:   []string{"MISSING_KEY"},
	}

	_, code := doApply(h, m)
	if code != http.StatusPreconditionFailed {
		t.Errorf("expected 412 Precondition Failed, got %d", code)
	}
}

func TestApply_RestartRecovery(t *testing.T) {
	// Simulate restart by starting with completely fresh registries
	h, appReg, depReg, domainReg, secretReg, routeReg := setupApplyTest()

	// But the user has secrets injected because secrets are volatile
	// Actually, the test requires the user to recreate secrets before apply if they are volatile.
	// For this test, we just assume the operator ran `secret set` first.

	m := &manifest.Manifest{
		Name:      "recovered-app",
		Image:     "redis:latest",
		Replicas:  2,
		Resources: manifest.ResourceSpec{CPU: 2, Memory: 1024},
		Service:   &manifest.ServiceSpec{Port: 6379},
		Domains: []manifest.DomainSpec{
			{Name: "redis.com", TLS: false},
		},
		Secrets: []string{"REDIS_PASS"},
	}

	// Apply before secret exists -> should fail
	_, code := doApply(h, m)
	if code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412, got %d", code)
	}

	// Operator creates secret
	// We must create the app first, wait, application creation in our apply flow happens *before* secrets validation,
	// so the app exists now.
	app, _ := appReg.GetByName("recovered-app")
	secretReg.Set(app.ID, "REDIS_PASS", "supersecret")

	// Apply again
	res, code := doApply(h, m)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	if res.Action != "updated" {
		t.Errorf("expected updated, got %s", res.Action)
	}

	// Verify all state is recovered
	deps := depReg.ListByApplication(app.ID)
	if len(deps) != 1 {
		t.Errorf("expected 1 deployment")
	}

	var svc route.Service
	for _, s := range routeReg.List() {
		if s.ApplicationID == app.ID {
			svc = s
			break
		}
	}
	if svc.TargetPort != 6379 {
		t.Errorf("expected service port 6379")
	}

	d, _ := domainReg.GetByName("redis.com")
	if d.ApplicationID != app.ID {
		t.Errorf("domain not linked")
	}
}

func TestEndToEndUX(t *testing.T) {
	applyH, appReg, depReg, domainReg, _, _ := setupApplyTest()

	appsH := &api.AppsHandler{
		AppRegistry:      appReg,
		DepRegistry:      depReg,
		InstanceRegistry: instance.NewRegistry(),
		DomainRegistry:   domainReg,
		IngressRegistry:  ingress.NewIngressRegistry(),
	}

	m := &manifest.Manifest{
		Name:      "test-ux-app",
		Image:     "nginx",
		Replicas:  2,
		Resources: manifest.ResourceSpec{CPU: 1.0, Memory: 128},
		Service:   &manifest.ServiceSpec{Port: 80},
		Domains:   []manifest.DomainSpec{{Name: "ux.example.com", TLS: true}},
	}

	b, _ := json.Marshal(m)
	reqApply := httptest.NewRequest("POST", "/api/v1/apply", bytes.NewReader(b))
	rrApply := httptest.NewRecorder()
	applyH.Apply(rrApply, reqApply)
	if rrApply.Code != http.StatusOK {
		t.Fatalf("apply failed, status %d, body: %s", rrApply.Code, rrApply.Body.String())
	}

	// Test GET /api/v1/apps
	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	appsH.ListApps(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list apps failed: %d", rr.Code)
	}

	var listRes map[string][]api.AppSummary
	json.NewDecoder(rr.Body).Decode(&listRes)
	if len(listRes["applications"]) != 1 || listRes["applications"][0].Name != "test-ux-app" {
		t.Errorf("expected test-ux-app in list")
	}

	// Test GET /api/v1/apps/test-ux-app/status
	req = httptest.NewRequest("GET", "/api/v1/apps/test-ux-app/status", nil)
	// We need a router to extract {name}, but our handler uses r.URL.Path parts manually.
	req.URL.Path = "/api/v1/apps/test-ux-app/status"
	rr = httptest.NewRecorder()
	appsH.GetAppStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get app status failed: %d", rr.Code)
	}

	var statusRes api.AppStatusDetail
	json.NewDecoder(rr.Body).Decode(&statusRes)
	if statusRes.Application.Name != "test-ux-app" {
		t.Errorf("expected detail name test-ux-app, got %s", statusRes.Application.Name)
	}
	if len(statusRes.Domains) != 1 || statusRes.Domains[0].Name != "ux.example.com" {
		t.Errorf("expected domain ux.example.com")
	}
}
