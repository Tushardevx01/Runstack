package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthManager(t *testing.T) {
	auth := NewAuthManager("op-token", "ag-token")

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	operatorHandler := auth.RequireOperator(handler)
	agentHandler := auth.RequireAgent(handler)

	tests := []struct {
		name           string
		handler        http.HandlerFunc
		token          string
		expectedStatus int
	}{
		{"Operator no token", operatorHandler, "", http.StatusUnauthorized},
		{"Operator wrong token", operatorHandler, "ag-token", http.StatusForbidden},
		{"Operator invalid token", operatorHandler, "invalid", http.StatusUnauthorized},
		{"Operator valid token", operatorHandler, "op-token", http.StatusOK},
		{"Agent no token", agentHandler, "", http.StatusUnauthorized},
		{"Agent wrong token", agentHandler, "op-token", http.StatusForbidden},
		{"Agent invalid token", agentHandler, "invalid", http.StatusUnauthorized},
		{"Agent valid token", agentHandler, "ag-token", http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rr := httptest.NewRecorder()
			tc.handler.ServeHTTP(rr, req)

			if rr.Result().StatusCode != tc.expectedStatus {
				t.Errorf("expected %d, got %d", tc.expectedStatus, rr.Result().StatusCode)
			}
		})
	}
}
