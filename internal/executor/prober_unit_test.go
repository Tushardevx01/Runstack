package executor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Tushardevx01/runstack/internal/application"
)

func TestDoProbe(t *testing.T) {
	// Start a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else if r.URL.Path == "/unhealthy" {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	portStr := strings.TrimPrefix(ts.URL, "http://127.0.0.1:")
	port, _ := strconv.Atoi(portStr)

	t.Run("HTTP Success", func(t *testing.T) {
		p := &application.Probe{Type: "HTTP", Path: "/health", TimeoutSecs: 1}
		if !doProbe(context.Background(), p, port) {
			t.Errorf("expected true")
		}
	})
	t.Run("HTTP Failure", func(t *testing.T) {
		p := &application.Probe{Type: "HTTP", Path: "/unhealthy", TimeoutSecs: 1}
		if doProbe(context.Background(), p, port) {
			t.Errorf("expected false")
		}
	})
	t.Run("TCP Success", func(t *testing.T) {
		p := &application.Probe{Type: "TCP", TimeoutSecs: 1}
		if !doProbe(context.Background(), p, port) {
			t.Errorf("expected true")
		}
	})
	t.Run("TCP Failure", func(t *testing.T) {
		// Use a free port that is likely closed
		l, _ := net.Listen("tcp", "127.0.0.1:0")
		freePort := l.Addr().(*net.TCPAddr).Port
		l.Close()

		p := &application.Probe{Type: "TCP", TimeoutSecs: 1}
		if doProbe(context.Background(), p, freePort) {
			t.Errorf("expected false")
		}
	})
}
