package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
)

func TestJobHandler_Create(t *testing.T) {
	registry := job.NewRegistry()
	handler := &JobHandler{Registry: registry}

	reqBody := `{"name": "test-job", "command": "echo test"}`
	req := httptest.NewRequest("POST", "/api/v1/jobs", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Result().StatusCode != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", w.Result().StatusCode)
	}

	var createdJob job.Job
	if err := json.NewDecoder(w.Body).Decode(&createdJob); err != nil {
		t.Fatal(err)
	}

	if createdJob.Name != "test-job" || createdJob.Status != job.StatusPending {
		t.Errorf("unexpected job creation state: %+v", createdJob)
	}
}

func TestJobHandler_CreateInvalid(t *testing.T) {
	registry := job.NewRegistry()
	handler := &JobHandler{Registry: registry}

	// Missing name
	reqBody := `{"command": "echo test"}`
	req := httptest.NewRequest("POST", "/api/v1/jobs", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()
	handler.Create(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Result().StatusCode)
	}

	// Malformed JSON
	reqBody = `{"name": "test-job", `
	req = httptest.NewRequest("POST", "/api/v1/jobs", bytes.NewBufferString(reqBody))
	w = httptest.NewRecorder()
	handler.Create(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Result().StatusCode)
	}

	// Negative maxRetries
	reqBody = `{"name": "test-job", "command": "echo test", "maxRetries": -1}`
	req = httptest.NewRequest("POST", "/api/v1/jobs", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.Create(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for negative maxRetries, got %d", w.Result().StatusCode)
	}
}

func TestJobHandler_Claim(t *testing.T) {
	nodeRegistry := node.NewRegistry()
	jobRegistry := job.NewRegistry()
	handler := &JobHandler{Registry: jobRegistry, NodeRegistry: nodeRegistry}

	nodeRegistry.Register(node.Node{ID: "node-1"}, "")
	j := jobRegistry.Create("test-claim", "echo 1", 0, 0, 0)

	// Set job to ASSIGNED manually
	status := job.StatusAssigned
	nodeID := "node-1"
	jobRegistry.Update(j.ID, job.UpdateParams{Status: &status, AssignedNodeID: &nodeID})

	reqBody := `{"nodeId": "node-1"}`
	req := httptest.NewRequest("POST", "/api/v1/jobs/"+j.ID+"/claim", bytes.NewBufferString(reqBody))
	req.SetPathValue("id", j.ID)
	w := httptest.NewRecorder()

	handler.Claim(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Result().StatusCode)
	}

	jAfter, _ := jobRegistry.Get(j.ID)
	if jAfter.Status != job.StatusRunning {
		t.Errorf("expected job status RUNNING, got %s", jAfter.Status)
	}
}

func TestJobHandler_ReportResult(t *testing.T) {
	nodeRegistry := node.NewRegistry()
	jobRegistry := job.NewRegistry()
	handler := &JobHandler{Registry: jobRegistry, NodeRegistry: nodeRegistry}

	nodeRegistry.Register(node.Node{ID: "node-1"}, "")
	j := jobRegistry.Create("test-result", "echo 1", 1, 0, 0)

	status := job.StatusAssigned
	nodeID := "node-1"
	jobRegistry.Update(j.ID, job.UpdateParams{Status: &status, AssignedNodeID: &nodeID})
	claimed, _ := jobRegistry.Claim(j.ID, "node-1")

	reqBody := fmt.Sprintf(`{"nodeId": "node-1", "executionId": "%s", "result": {"exitCode": 0, "stdout": "ok", "stderr": ""}}`, claimed.ExecutionID)
	req := httptest.NewRequest("POST", "/api/v1/jobs/"+j.ID+"/result", bytes.NewBufferString(reqBody))
	req.SetPathValue("id", j.ID)
	w := httptest.NewRecorder()

	handler.ReportResult(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Result().StatusCode)
	}

	jAfter, _ := jobRegistry.Get(j.ID)
	if jAfter.Status != job.StatusSucceeded {
		t.Errorf("expected job status SUCCEEDED, got %s", jAfter.Status)
	}
	if jAfter.Result == nil || jAfter.Result.Stdout != "ok" {
		t.Errorf("expected result stdout 'ok'")
	}
}

func TestJobHandler_GetEvents(t *testing.T) {
	nodeReg := node.NewRegistry()
	jobReg := job.NewRegistry()
	handler := &JobHandler{Registry: jobReg, NodeRegistry: nodeReg}

	j := jobReg.Create("test-job", "echo 1", 0, 0, 0)

	// Missing job
	req := httptest.NewRequest("GET", "/api/v1/jobs/fake-id/events", nil)
	req.SetPathValue("id", "fake-id")
	rr := httptest.NewRecorder()
	handler.GetEvents(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}

	// Existing job
	req = httptest.NewRequest("GET", "/api/v1/jobs/"+j.ID+"/events", nil)
	req.SetPathValue("id", j.ID)
	rr = httptest.NewRecorder()
	handler.GetEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string][]job.JobEvent
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp["events"]) != 1 || resp["events"][0].Type != job.EventCreated {
		t.Errorf("expected exactly 1 CREATED event")
	}
}

// ========================================================================
// ADVERSARIAL AUDIT REGRESSION TESTS
// ========================================================================

func TestJobHandler_ReportResult_EmptyNodeID(t *testing.T) {
	nodeRegistry := node.NewRegistry()
	jobRegistry := job.NewRegistry()
	handler := &JobHandler{Registry: jobRegistry, NodeRegistry: nodeRegistry}

	nodeRegistry.Register(node.Node{ID: "node-1"}, "")
	j := jobRegistry.Create("test", "echo 1", 0, 0, 0)

	status := job.StatusAssigned
	nodeID := "node-1"
	jobRegistry.Update(j.ID, job.UpdateParams{Status: &status, AssignedNodeID: &nodeID})
	claimed, _ := jobRegistry.Claim(j.ID, "node-1")

	// Empty nodeId should return 400, not reach the registry
	reqBody := fmt.Sprintf(`{"nodeId": "", "executionId": "%s", "result": {"exitCode": 0}}`, claimed.ExecutionID)
	req := httptest.NewRequest("POST", "/api/v1/jobs/"+j.ID+"/result", bytes.NewBufferString(reqBody))
	req.SetPathValue("id", j.ID)
	w := httptest.NewRecorder()

	handler.ReportResult(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty nodeId, got %d", w.Result().StatusCode)
	}
}

func TestJobHandler_ReportResult_EmptyExecutionID(t *testing.T) {
	nodeRegistry := node.NewRegistry()
	jobRegistry := job.NewRegistry()
	handler := &JobHandler{Registry: jobRegistry, NodeRegistry: nodeRegistry}

	nodeRegistry.Register(node.Node{ID: "node-1"}, "")
	j := jobRegistry.Create("test", "echo 1", 0, 0, 0)

	status := job.StatusAssigned
	nodeID := "node-1"
	jobRegistry.Update(j.ID, job.UpdateParams{Status: &status, AssignedNodeID: &nodeID})
	jobRegistry.Claim(j.ID, "node-1")

	// Empty executionId should return 400
	reqBody := `{"nodeId": "node-1", "executionId": "", "result": {"exitCode": 0}}`
	req := httptest.NewRequest("POST", "/api/v1/jobs/"+j.ID+"/result", bytes.NewBufferString(reqBody))
	req.SetPathValue("id", j.ID)
	w := httptest.NewRecorder()

	handler.ReportResult(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty executionId, got %d", w.Result().StatusCode)
	}
}

func TestJobHandler_Update_PendingToFailed_Blocked(t *testing.T) {
	nodeRegistry := node.NewRegistry()
	jobRegistry := job.NewRegistry()
	handler := &JobHandler{Registry: jobRegistry, NodeRegistry: nodeRegistry}

	j := jobRegistry.Create("test", "echo 1", 0, 0, 0)

	reqBody := `{"status": "failed"}`
	req := httptest.NewRequest("PATCH", "/api/v1/jobs/"+j.ID, bytes.NewBufferString(reqBody))
	req.SetPathValue("id", j.ID)
	w := httptest.NewRecorder()

	handler.Update(w, req)

	// Should not be 200 OK — this transition must be blocked
	if w.Result().StatusCode == http.StatusOK {
		t.Errorf("expected PENDING→FAILED to be blocked via API, but got 200 OK")
	}

	jAfter, _ := jobRegistry.Get(j.ID)
	if jAfter.Status != job.StatusPending {
		t.Fatalf("expected job to remain PENDING, got %s", jAfter.Status)
	}
}
