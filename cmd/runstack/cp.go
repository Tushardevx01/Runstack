package main

import (
	"flag"

	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/route"
	"github.com/Tushardevx01/runstack/internal/scheduler"
	"github.com/Tushardevx01/runstack/internal/service"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:    "ok",
		Service:   "runstack-control-plane",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := StatusResponse{
		Status:  "running",
		Version: "0.1.0",
	}

	json.NewEncoder(w).Encode(response)
}

func runControlPlane(args []string) {
	fs := flag.NewFlagSet("cp", flag.ExitOnError)
	operatorToken := fs.String("operator-token", os.Getenv("RUNSTACK_OPERATOR_TOKEN"), "Operator Bearer token")
	agentToken := fs.String("agent-token", os.Getenv("RUNSTACK_AGENT_TOKEN"), "Agent Bearer token")
	_ = fs.Parse(args)

	auth := api.NewAuthManager(*operatorToken, *agentToken)

	registry := node.NewRegistry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Background offline detection
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				registry.MarkOfflineNodes(30 * time.Second)
			}
		}
	}()

	nodeHandler := &api.NodeHandler{Registry: registry}

	jobRegistry := job.NewRegistry()
	jobHandler := &api.JobHandler{Registry: jobRegistry, NodeRegistry: registry}

	appRegistry := application.NewRegistry()
	depRegistry := deployment.NewRegistry()
	instRegistry := instance.NewRegistry()
	routeRegistry := route.NewRegistry()
	domainRegistry := ingress.NewDomainRegistry()
	ingressRegistry := ingress.NewIngressRegistry()
	secretRegistry := application.NewSecretRegistry()
	acmeProvider := ingress.NewACMEProvider(domainRegistry)

	httpProxy := route.NewHTTPProxy(80, 8443) // Default HTTP and HTTPS ports
	httpProxy.GetTLSCertificate = acmeProvider.GetTLSCertificate
	httpProxy.IsTLSEnabled = func(domain string) bool {
		d, err := domainRegistry.GetByName(domain)
		if err == nil {
			return d.TLS
		}
		return false
	}
	httpProxy.ACMEHandler = acmeProvider.HTTPHandler(nil)
	go func() {
		if err := httpProxy.Start(ctx); err != nil {
			slog.Error("HTTP Proxy failed", "error", err)
		}
	}()

	appService := service.NewAppService(appRegistry, depRegistry, secretRegistry)
	logsHandler := &api.LogsHandler{
		AppRegistry:      appRegistry,
		InstanceRegistry: instRegistry,
		NodeRegistry:     registry,
	}

	appsHandler := &api.AppsHandler{
		AppRegistry:      appRegistry,
		DepRegistry:      depRegistry,
		InstanceRegistry: instRegistry,
		DomainRegistry:   domainRegistry,
		IngressRegistry:  ingressRegistry,
	}

	appHandler := &api.AppHandler{

		Service: appService,
	}
	routeHandler := &api.RouteHandler{
		ServiceRegistry: routeRegistry,
		AppRegistry:     appRegistry,
	}

	capCalc := scheduler.NewCapacityCalculator(appRegistry, depRegistry, instRegistry, jobRegistry)
	nodeHandler.CapCalc = capCalc
	sched := scheduler.New(registry, jobRegistry, capCalc)
	instSched := scheduler.NewInstanceScheduler(registry, instRegistry, capCalc)
	instReconciler := scheduler.NewInstanceReconciler(appRegistry, depRegistry, instRegistry, registry)
	routingReconciler := scheduler.NewRoutingReconciler(appRegistry, instRegistry, registry, routeRegistry, domainRegistry, ingressRegistry, httpProxy)

	// Start background scheduler loops
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sched.SchedulePendingJobs(); err != nil {
					slog.Error("Job Scheduler error", "error", err)
				}
				if err := instReconciler.Reconcile(); err != nil {
					slog.Error("Instance Reconciler error", "error", err)
				}
				if err := instSched.SchedulePendingInstances(); err != nil {
					slog.Error("Instance Scheduler error", "error", err)
				}
				if err := routingReconciler.Reconcile(ctx); err != nil {
					slog.Error("Routing Reconciler error", "error", err)
				}
			}
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /api/v1/status", statusHandler)

	mux.HandleFunc("POST /api/v1/nodes/register", auth.RequireAgent(nodeHandler.Register))
	mux.HandleFunc("GET /api/v1/nodes", auth.RequireOperator(nodeHandler.ListNodes))
	mux.HandleFunc("GET /api/v1/nodes/{id}", auth.RequireOperator(nodeHandler.GetNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/heartbeat", auth.RequireNodeAuth(registry, nodeHandler.Heartbeat))

	mux.HandleFunc("POST /api/v1/jobs", auth.RequireOperator(jobHandler.Create))
	mux.HandleFunc("GET /api/v1/jobs", auth.RequireOperator(jobHandler.List))
	mux.HandleFunc("GET /api/v1/jobs/{id}", auth.RequireOperator(jobHandler.Get))
	mux.HandleFunc("GET /api/v1/jobs/{id}/events", auth.RequireOperator(jobHandler.GetEvents))
	mux.HandleFunc("PATCH /api/v1/jobs/{id}", auth.RequireOperator(jobHandler.Update))
	mux.HandleFunc("POST /api/v1/jobs/{id}/claim", auth.RequireNodeAuth(registry, jobHandler.Claim))
	mux.HandleFunc("POST /api/v1/jobs/{id}/result", auth.RequireNodeAuth(registry, jobHandler.ReportResult))

	mux.HandleFunc("POST /api/v1/apps", auth.RequireOperator(appHandler.Create))
	mux.HandleFunc("GET /api/v1/apps", auth.RequireOperator(appsHandler.ListApps))
	mux.HandleFunc("GET /api/v1/apps/{id}", auth.RequireOperator(appHandler.Get))
	mux.HandleFunc("GET /api/v1/apps/{id}/status", auth.RequireOperator(appsHandler.GetAppStatus))
	mux.HandleFunc("GET /api/v1/apps/{id}/logs", auth.RequireOperator(logsHandler.GetAppLogs))
	mux.HandleFunc("PUT /api/v1/apps/{id}", auth.RequireOperator(appHandler.Update))
	mux.HandleFunc("POST /api/v1/apps/{id}/deploy", auth.RequireOperator(appHandler.Deploy))
	mux.HandleFunc("POST /api/v1/apps/{id}/rollback", auth.RequireOperator(appHandler.Rollback))

	secretHandler := &api.SecretHandler{
		Registry: secretRegistry,
		AppReg:   appRegistry,
	}
	mux.HandleFunc("POST /api/v1/secrets", auth.RequireOperator(secretHandler.Set))
	mux.HandleFunc("GET /api/v1/secrets", auth.RequireOperator(secretHandler.List))
	mux.HandleFunc("DELETE /api/v1/secrets/{id}", auth.RequireOperator(secretHandler.Delete))

	domainHandler := &api.DomainHandler{
		DomainRegistry: domainRegistry,
		AppRegistry:    appRegistry,
		CertProvider:   acmeProvider,
	}
	ingressHandler := &api.IngressHandler{
		IngressRegistry: ingressRegistry,
		DomainRegistry:  domainRegistry,
		ServiceRegistry: routeRegistry,
	}

	mux.HandleFunc("POST /api/v1/services", auth.RequireOperator(routeHandler.Create))
	mux.HandleFunc("GET /api/v1/services", auth.RequireOperator(routeHandler.List))
	mux.HandleFunc("GET /api/v1/services/{id}", auth.RequireOperator(routeHandler.Get))
	mux.HandleFunc("PUT /api/v1/services/{id}", auth.RequireOperator(routeHandler.Update))
	mux.HandleFunc("DELETE /api/v1/services/{id}", auth.RequireOperator(routeHandler.Delete))

	applyHandler := &api.ApplyHandler{
		AppRegistry:     appRegistry,
		DepRegistry:     depRegistry,
		SecretRegistry:  secretRegistry,
		DomainRegistry:  domainRegistry,
		IngressRegistry: ingressRegistry,
		ServiceRegistry: routeRegistry,
		CertProvider:    acmeProvider,
	}

	mux.HandleFunc("POST /api/v1/apply", auth.RequireOperator(applyHandler.Apply))
	mux.HandleFunc("POST /api/v1/diff", auth.RequireOperator(applyHandler.Diff))

	mux.HandleFunc("POST /api/v1/domains", auth.RequireOperator(domainHandler.Create))
	mux.HandleFunc("GET /api/v1/domains", auth.RequireOperator(domainHandler.List))
	mux.HandleFunc("DELETE /api/v1/domains/{id}", auth.RequireOperator(domainHandler.Delete))
	mux.HandleFunc("POST /api/v1/domains/{domain}/tls", auth.RequireOperator(domainHandler.EnableTLS))
	mux.HandleFunc("GET /api/v1/domains/{domain}/tls", auth.RequireOperator(domainHandler.TLSStatus))

	mux.HandleFunc("POST /api/v1/ingresses", auth.RequireOperator(ingressHandler.Create))
	mux.HandleFunc("GET /api/v1/ingresses", auth.RequireOperator(ingressHandler.List))
	mux.HandleFunc("DELETE /api/v1/ingresses/{id}", auth.RequireOperator(ingressHandler.Delete))

	instanceHandler := &api.InstanceHandler{
		InstanceRegistry:   instRegistry,
		DeploymentRegistry: depRegistry,
		SecretRegistry:     secretRegistry,
	}
	mux.HandleFunc("GET /api/v1/instances", auth.RequireOperator(instanceHandler.List))
	mux.HandleFunc("POST /api/v1/instances/{id}/claim", auth.RequireNodeAuth(registry, instanceHandler.Claim))
	mux.HandleFunc("POST /api/v1/instances/{id}/status", auth.RequireNodeAuth(registry, instanceHandler.ReportStatus))

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("RunStack Control Plane started")
	slog.Info("Listening", "address", "http://localhost:8080")

	if err := server.ListenAndServe(); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
