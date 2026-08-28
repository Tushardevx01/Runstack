# Milestone 8G: Custom Domains & Ingress Design

## Overview
Milestone 8G completes the user-facing product loop by bridging the gap between internal HTTP Services and public Internet traffic. It introduces **Ingress**, which manages custom domain names, Host-based routing, and an extensible TLS abstraction for automated certificate provisioning, culminating in the "code to URL" PaaS promise.

---

## 1. Domain Model
Domains are first-class primitives managed by the Control Plane. 
A `Domain` maps exactly to one Fully Qualified Domain Name (FQDN) (e.g., `api.example.com`).

```go
type Domain struct {
	ID            string 
	Name          string // The FQDN
	ApplicationID string // Strict ownership
	Status        DomainStatus
}
```

## 2. Ingress Model
`Ingress` acts as the mapping layer between a `Domain` and an internal `Service`.

```go
type Ingress struct {
	ID        string
	DomainID  string
	ServiceID string
	Path      string // V1 defaults to "/"
}
```

## 3. Service Relationship
The relationship chain is strictly hierarchical:
`Domain → Ingress → Service → Active Instances (via Endpoints)`.
The `RoutingReconciler` will be updated to watch the `IngressRegistry`. When an Ingress changes, the Reconciler will rebuild the routing table, mapping the Host header (from the Domain) to the Endpoints backing the specified Service.

## 4. Host-Based Routing
The existing `ProxyProvider` (the HTTP Proxy) will be enhanced to route traffic based on the HTTP `Host` header.
- Traffic arriving at the edge is matched against configured Domains.
- If a match is found, traffic is routed to the corresponding internal Service.
- Unmatched Host headers are rejected safely (see HTTP behavior).

## 5. Domain Ownership
To guarantee security, **a Domain must belong to an Application**. 
An Application can claim multiple Domains, but a single Domain can only belong to one Application. The API will enforce that an `Ingress` can only link a `Domain` and a `Service` if they are both owned by the same `ApplicationID`.

## 6. Conflict Handling
- **Domain Conflict:** The `DomainRegistry` will guarantee uniqueness on `Domain.Name`. If Application B attempts to register `api.example.com` while Application A owns it, the API will return `409 Conflict`.
- **Ingress Conflict:** If two Ingresses attempt to map the same Domain and Path, the Control Plane will reject the creation.

## 7. Routing Reconciliation
The `RoutingReconciler` currently bridges `Service` + `InstanceHealth` -> `ProxyProvider`.
In 8G, the Reconciler will observe:
1. `DomainRegistry` (for active domain definitions)
2. `IngressRegistry` (for Domain -> Service mappings)
3. `ServiceRegistry` (for Service definitions)
4. `InstanceRegistry` (for healthy endpoints)

State updates to any of these registries trigger a rapid asynchronous rebuilding of the internal routing tree (e.g., a Radix tree or Hash map) which is then atomically swapped into the active `HTTPProxy`.

## 8. HTTP Behavior
- **Valid Route:** HTTP 200/Success (proxied to Instance).
- **Missing Domain:** HTTP 404 Not Found (RunStack default edge response).
- **Domain Mapped, but No Healthy Instances:** HTTP 503 Service Unavailable.
- **TLS Required but HTTP Requested:** HTTP 301 Redirect to HTTPS (if TLS is enabled for the Domain).

## 9. TLS Abstraction
TLS automation will be bounded behind a generic interface, ensuring RunStack is not hopelessly tied to a specific provider (e.g., Let's Encrypt).

```go
type Certificate struct {
	DomainName string
	CertPEM    []byte
	KeyPEM     []byte
	Expiry     time.Time
}

type CertificateProvider interface {
	// Provision requests a new certificate for the given domain.
	Provision(ctx context.Context, domain string) (*Certificate, error)
	// Renew renews an existing certificate.
	Renew(ctx context.Context, domain string) (*Certificate, error)
}
```

## 10. ACME Lifecycle (V1 Implementation)
The primary `CertificateProvider` implementation will use ACME (Automatic Certificate Management Environment).
- **Challenge Type:** HTTP-01 (since RunStack owns the port 80 routing layer).
- **Challenge Routing:** The `RoutingReconciler` will automatically inject a temporary route for `/.well-known/acme-challenge/` to intercept ACME verification traffic and pass it to the ACME solver.
- **Issuance:** Once issued, the `Certificate` is stored in an in-memory `CertificateRegistry`.

## 11. Certificate Renewal
A background `CertificateReconciler` will periodically scan the `CertificateRegistry`. 
- If a certificate is within 30 days of expiration, it will invoke `CertificateProvider.Renew()`.
- Upon successful renewal, the new certificate is stored, and the `HTTPProxy` is dynamically updated via `tls.Config.GetCertificate` to seamlessly serve the new certificate without dropping connections.

## 12. DNS Boundary
**DNS management is strictly outside the Control Plane.**
RunStack will *not* integrate with Cloudflare, Route53, or Google Cloud DNS APIs. 
The developer is responsible for creating a CNAME or A record pointing to the RunStack Node(s). RunStack merely observes arriving traffic and issues certificates based on HTTP-01 challenges.

## 13. Security
- **Strict Host Checking:** The Proxy drops traffic for unknown Host headers.
- **Ownership Fencing:** As defined in Section 5, Cross-Application Ingress hijacking is structurally prevented in the API layer.
- **Memory Safety:** Certificates are stored purely in memory. They do not touch the disk, avoiding local credential leakage (though they will be lost on CP restart until re-provisioned).

## 14. Failure Scenarios
- **ACME Rate Limits / Failures:** The Domain status reflects `TLS_FAILED`, but unencrypted HTTP traffic can continue to serve if configured. The Reconciler uses exponential backoff to retry issuance.
- **Control Plane Restart:** Because there is **no database**, certificates are lost on CP restart. The CP will automatically request new certificates for all active Domains upon boot. 
- **Instance Crash:** Standard 8C healing applies; Ingress simply routes to remaining healthy instances or returns 503 if none exist.

## 15. CLI / API
**New API Endpoints:**
- `POST /api/v1/domains` (Create Domain)
- `POST /api/v1/ingresses` (Create Domain->Service mapping)
- `GET /api/v1/domains/{id}` (Check status/TLS state)

**CLI UX:**
```bash
# Add a domain to an app
runstack domain add my-app api.example.com --tls=true

# View domains
runstack domain ls my-app
```

## 16. Concurrency
- Registries (`DomainRegistry`, `IngressRegistry`, `CertificateRegistry`) will utilize strict `sync.RWMutex` locking mechanisms identical to existing registries.
- The `HTTPProxy` will use a synchronized pointer swap (`atomic.Pointer`) to update routing rules and TLS certificates safely under high concurrency without dropping active requests.

## 17. Tests
- **Domain Conflict:** Verify Application A cannot claim Application B's Domain.
- **Routing Engine:** Validate Host-header based routing, including wildcard or path prefix matching.
- **TLS Swapping:** Verify that `GetCertificate` serves the new certificate instantly after issuance.
- **Data Races:** Run complete `go test -race` against the new Reconcilers and Proxy updates.

## 18. V1 Non-Goals
- External DNS Provider Integration (Route53/Cloudflare/etc.).
- DNS-01 ACME Challenges (since DNS is external).
- Distributed/Persistent Storage for Certificates.
- Let's Encrypt production rate limit handling (V1 will use Staging environments for testing).
- Mutual TLS (mTLS) for edge traffic.
