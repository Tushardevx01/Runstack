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

func TestRouteHandler_Create(t *testing.T) {
	appReg := application.NewRegistry()
	routeReg := route.NewRegistry()

	handler := &RouteHandler{
		ServiceRegistry: routeReg,
		AppRegistry:     appReg,
	}

	app, _ := appReg.Create("app1", application.AppSpec{})

	// Create valid service
	reqBody := map[string]interface{}{
		"application_id": app.ID,
		"target_port":    8080,
	}
	b, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/services", bytes.NewBuffer(b))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rr.Code)
	}

	var srv route.Service
	json.NewDecoder(rr.Body).Decode(&srv)

	if srv.ApplicationID != app.ID {
		t.Errorf("expected %s, got %s", app.ID, srv.ApplicationID)
	}
	if srv.TargetPort != 8080 {
		t.Errorf("expected 8080, got %d", srv.TargetPort)
	}
}
