package route

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type RouteEntry struct {
	Service   Service
	Endpoints []Endpoint
	Counter   *uint64
}

type HTTPProxy struct {
	mu     sync.RWMutex
	routes map[string]*RouteEntry // ServiceID -> RouteEntry
	port   int
	server *http.Server
}

func NewHTTPProxy(port int) *HTTPProxy {
	return &HTTPProxy{
		routes: make(map[string]*RouteEntry),
		port:   port,
	}
}

func (p *HTTPProxy) UpdateRoute(ctx context.Context, srv Service, endpoints []Endpoint) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, exists := p.routes[srv.ID]
	if !exists {
		var c uint64
		entry = &RouteEntry{Counter: &c}
		p.routes[srv.ID] = entry
	}
	entry.Service = srv
	entry.Endpoints = endpoints

	slog.Info("Proxy updated", "service", srv.ID, "domain", srv.Domain, "endpoints", len(endpoints))
	return nil
}

func (p *HTTPProxy) RemoveRoute(ctx context.Context, serviceID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.routes, serviceID)
	return nil
}

func (p *HTTPProxy) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handleRequest)

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", p.port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.server.Shutdown(shutdownCtx)
	}()

	slog.Info("HTTP Proxy started", "port", p.port)
	if err := p.server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (p *HTTPProxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	var matched *RouteEntry
	for _, entry := range p.routes {
		if entry.Service.Domain != "" && entry.Service.Domain != r.Host {
			continue
		}
		if entry.Service.PathPrefix != "" && !strings.HasPrefix(r.URL.Path, entry.Service.PathPrefix) {
			continue
		}
		matched = entry
		break
	}

	if matched == nil || len(matched.Endpoints) == 0 {
		p.mu.RUnlock()
		http.Error(w, "Service Unavailable: No healthy endpoints", http.StatusServiceUnavailable)
		return
	}

	// Copy endpoints to avoid holding lock during proxying
	endpoints := make([]Endpoint, len(matched.Endpoints))
	copy(endpoints, matched.Endpoints)
	counter := matched.Counter
	p.mu.RUnlock()

	idx := atomic.AddUint64(counter, 1) % uint64(len(endpoints))
	target := endpoints[idx]

	targetURL, _ := url.Parse(fmt.Sprintf("http://%s:%d", target.IP, target.Port))
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ServeHTTP(w, r)
}
