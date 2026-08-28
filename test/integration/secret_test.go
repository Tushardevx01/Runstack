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
	"github.com/Tushardevx01/runstack/internal/instance"
)

func TestSecret_E2EResolution(t *testing.T) {
	appRegistry := application.NewRegistry()
	depRegistry := deployment.NewRegistry()
	instRegistry := instance.NewRegistry()
	secRegistry := application.NewSecretRegistry()

	// 1. Create App
	app, _ := appRegistry.Create("my-app", application.AppSpec{})

	// 2. Create Secret
	_, err := secRegistry.Set(app.ID, "API_KEY", "super-secret-value")
	if err != nil {
		t.Fatalf("failed to set secret: %v", err)
	}

	// 3. Create Deployment referencing Secret
	spec := application.AppSpec{
		Image: "my-image",
		Environment: map[string]string{
			"API_KEY": "secret:API_KEY",
			"PLAIN":   "plain-value",
		},
		Replicas: 1,
	}
	dep, _ := depRegistry.Create(app.ID, spec)
	appRegistry.Update(app.ID, app.Spec, dep.ID, application.StatusReady)

	// 4. Create Instance assigned to node
	inst, _ := instRegistry.Create(app.ID, dep.ID)
	instRegistry.UpdateState(inst.ID, instance.StatusAssigned, "node-1", "")

	// 5. Setup Claim Handler
	handler := &api.InstanceHandler{
		InstanceRegistry:   instRegistry,
		DeploymentRegistry: depRegistry,
		SecretRegistry:     secRegistry,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/instances/{id}/claim", handler.Claim)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 6. Claim Instance (Agent requests JIT resolution)
	payload := `{"node_id": "node-1"}`
	resp, err := http.Post(ts.URL+"/api/v1/instances/"+inst.ID+"/claim", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("failed to claim: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var claimResp api.ClaimInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&claimResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// 7. Verify JIT Resolution
	if claimResp.Spec.Environment["API_KEY"] != "super-secret-value" {
		t.Errorf("expected JIT resolution to super-secret-value, got %s", claimResp.Spec.Environment["API_KEY"])
	}
	if claimResp.Spec.Environment["PLAIN"] != "plain-value" {
		t.Errorf("expected plain-value, got %s", claimResp.Spec.Environment["PLAIN"])
	}

	// Ensure deployment remains immutable with reference
	savedDep, _ := depRegistry.Get(dep.ID)
	if savedDep.SpecSnapshot.Environment["API_KEY"] != "secret:API_KEY" {
		t.Errorf("deployment snapshot was mutated! got %s", savedDep.SpecSnapshot.Environment["API_KEY"])
	}
}

func TestSecret_E2EMissingSecretFails(t *testing.T) {
	appRegistry := application.NewRegistry()
	depRegistry := deployment.NewRegistry()
	instRegistry := instance.NewRegistry()
	secRegistry := application.NewSecretRegistry()

	app, _ := appRegistry.Create("my-app", application.AppSpec{})
	spec := application.AppSpec{
		Image: "my-image",
		Environment: map[string]string{
			"MISSING": "secret:DOES_NOT_EXIST",
		},
		Replicas: 1,
	}
	dep, _ := depRegistry.Create(app.ID, spec)
	inst, _ := instRegistry.Create(app.ID, dep.ID)
	instRegistry.UpdateState(inst.ID, instance.StatusAssigned, "node-1", "")

	handler := &api.InstanceHandler{
		InstanceRegistry:   instRegistry,
		DeploymentRegistry: depRegistry,
		SecretRegistry:     secRegistry,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/instances/{id}/claim", handler.Claim)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	payload := `{"node_id": "node-1"}`
	resp, _ := http.Post(ts.URL+"/api/v1/instances/"+inst.ID+"/claim", "application/json", bytes.NewBufferString(payload))

	// Claim MUST fail if secret is missing
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 when secret missing, got %d", resp.StatusCode)
	}

	// Check instance state - it was marked STARTING by Claim but since response failed,
	// Agent will crash it eventually. The test just checks the API failed securely.
}
