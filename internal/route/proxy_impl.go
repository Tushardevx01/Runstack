package route

import (
	"context"
	"crypto/tls"
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
	routes    atomic.Pointer[[]*RouteEntry]
	port      int
	tlsPort   int
	server    *http.Server
	tlsServer *http.Server

	GetTLSCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	IsTLSEnabled      func(domain string) bool
	ACMEHandler       http.Handler
}

func NewHTTPProxy(port int, tlsPort int) *HTTPProxy {
	p := &HTTPProxy{
		port:    port,
		tlsPort: tlsPort,
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
	var httpHandler http.Handler = http.HandlerFunc(p.handleRequest)
	if p.ACMEHandler != nil {
		mux := http.NewServeMux()
		mux.Handle("/.well-known/acme-challenge/", p.ACMEHandler)
		mux.Handle("/", httpHandler)
		httpHandler = mux
	}

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", p.port),
		Handler: httpHandler,
	}

	if p.GetTLSCertificate != nil {
		p.tlsServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", p.tlsPort),
			Handler: http.HandlerFunc(p.handleRequestTLS),
			TLSConfig: &tls.Config{
				GetCertificate: p.GetTLSCertificate,
				MinVersion:     tls.VersionTLS12,
			},
		}
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.server.Shutdown(shutdownCtx)
		if p.tlsServer != nil {
			p.tlsServer.Shutdown(shutdownCtx)
		}
	}()

	slog.Info("HTTP Proxy started", "port", p.port)
	if p.tlsServer != nil {
		slog.Info("HTTPS Proxy started", "port", p.tlsPort)
		go p.tlsServer.ListenAndServeTLS("", "")
	}

	if err := p.server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (p *HTTPProxy) handleRequestTLS(w http.ResponseWriter, r *http.Request) {
	p.handleRequestInternal(w, r, true)
}

func (p *HTTPProxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	p.handleRequestInternal(w, r, false)
}

func (p *HTTPProxy) handleRequestInternal(w http.ResponseWriter, r *http.Request, isTLS bool) {
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

	if !isTLS && p.IsTLSEnabled != nil && p.IsTLSEnabled(reqHost) {
		http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusPermanentRedirect)
		return
	}

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
