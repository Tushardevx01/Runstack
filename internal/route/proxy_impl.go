package route

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type RouteEntry struct {
	Rule    RouteRule
	Counter *uint64
}

type HTTPProxy struct {
	routes atomic.Pointer[[]*RouteEntry]
	port   int
	server *http.Server
}

func NewHTTPProxy(port int) *HTTPProxy {
	p := &HTTPProxy{
		port: port,
	}
	empty := make([]*RouteEntry, 0)
	p.routes.Store(&empty)
	return p
}

func (p *HTTPProxy) UpdateRoutes(ctx context.Context, rules []RouteRule) error {
	newRoutes := make([]*RouteEntry, len(rules))

	// Preserve counters if possible for load balancing
	oldRoutesPtr := p.routes.Load()
	var oldRoutes []*RouteEntry
	if oldRoutesPtr != nil {
		oldRoutes = *oldRoutesPtr
	}

	for i, rule := range rules {
		var counter *uint64
		for _, old := range oldRoutes {
			if old.Rule.Host == rule.Host && old.Rule.Path == rule.Path {
				counter = old.Counter
				break
			}
		}
		if counter == nil {
			var c uint64
			counter = &c
		}
		newRoutes[i] = &RouteEntry{
			Rule:    rule,
			Counter: counter,
		}
	}

	p.routes.Store(&newRoutes)
	slog.Info("Proxy routing table updated", "routes", len(newRoutes))
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
	routesPtr := p.routes.Load()
	if routesPtr == nil {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	routes := *routesPtr

	reqHost := r.Host
	if idx := strings.Index(reqHost, ":"); idx != -1 {
		reqHost = reqHost[:idx]
	}
	reqHost = strings.TrimSuffix(strings.ToLower(reqHost), ".")

	var matched *RouteEntry
	for _, entry := range routes {
		if entry.Rule.Host != "" && entry.Rule.Host != reqHost {
			continue
		}
		if entry.Rule.Path != "" && !strings.HasPrefix(r.URL.Path, entry.Rule.Path) {
			continue
		}
		matched = entry
		break
	}

	if matched == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if len(matched.Rule.Endpoints) == 0 {
		http.Error(w, "Service Unavailable: No healthy endpoints", http.StatusServiceUnavailable)
		return
	}

	idx := atomic.AddUint64(matched.Counter, 1) % uint64(len(matched.Rule.Endpoints))
	target := matched.Rule.Endpoints[idx]

	targetURL, _ := url.Parse(fmt.Sprintf("http://%s:%d", target.IP, target.Port))
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ServeHTTP(w, r)
}
