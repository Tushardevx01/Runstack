package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/route"
)

func TestServiceCRUD(t *testing.T) {
	appReg := application.NewRegistry()
	srvReg := route.NewRegistry()

	handler := &RouteHandler{
		AppRegistry:     appReg,
		ServiceRegistry: srvReg,
	}

	app, _ := appReg.Create("app1", application.AppSpec{})

	// Create valid service
	reqBody := CreateServiceRequest{
		ApplicationID: app.ID,
		Domain:        "example.com",
		TargetPort:    8080,
	}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewReader(b))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Create invalid service (wrong app)
	reqBody.ApplicationID = "invalid-app"
	b, _ = json.Marshal(reqBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewReader(b))
	rec = httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid app, got %d", rec.Code)
	}
}
