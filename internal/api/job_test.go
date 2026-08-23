package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tushardevx01/runstack/internal/job"
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
}
