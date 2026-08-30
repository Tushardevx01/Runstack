# Milestone 8L: Automatic TLS / HTTPS Ingress

## 1. Problem
Currently, RunStack routes external traffic via custom domains exclusively over unencrypted HTTP. In a modern web environment, serving applications over plaintext HTTP is insecure and unacceptable for production workloads. However, requiring users to manually manage certificates or configure reverse proxies breaks the "push-to-deploy" PaaS experience.

## 2. Goals
* Provide automatic HTTPS for custom domains via Let's Encrypt (ACME).
* Implement the HTTP-01 challenge strictly within the Control Plane.
* Terminate TLS at the RunStack `HTTPProxy`.
* Automatically redirect HTTP traffic to HTTPS for TLS-enabled domains.
* Securely manage SNI routing so multiple domains share port 443 safely.
* Maintain the strict "No Database / No Persistence" V1 constraints.

## 3. Non-Goals
* DNS-01 ACME challenges (no external DNS provider API integration).
* Wildcard certificates (requires DNS-01).
* Persistent certificate storage or Cloud KMS integration.
* Distributed certificate synchronization (V1 assumes a single Control Plane instance).
* User-provided custom certificates (for this milestone).

## 4. Domain Model
Introduce a `Certificate` abstraction in the `domain` package:
```go
type CertStatus string
const (
    CertStatusPending CertStatus = "pending"
    CertStatusIssued  CertStatus = "issued"
    CertStatusFailed  CertStatus = "failed"
    CertStatusExpired CertStatus = "expired"
)

type Certificate struct {
    ID        string
    Domain    string
    Status    CertStatus
    CreatedAt time.Time
    ExpiresAt time.Time
    Error     string
}
```
**Critical Boundary:** Private key material (`*rsa.PrivateKey`, `*ecdsa.PrivateKey`) and raw PEM blocks are strictly isolated in memory within the `CertificateProvider` and are **never** serialized, returned via the API, or logged.

## 5. CertificateProvider
Expand the `CertificateProvider` interface (introduced conceptually in 8G):
```go
type CertificateProvider interface {
    RequestCertificate(ctx context.Context, domain string) error
    GetCertificate(domain string) (Certificate, error)
    GetTLSCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error)
    GetHTTP01Challenge(token string) (string, bool)
}
```

## 6. ACME & Challenge Flow
We will use `golang.org/x/crypto/acme/autocert` or a custom ACME client wrapper.
1. **Request:** Operator triggers TLS enable. `CertificateProvider` registers an ACME account (if missing) and requests a cert.
2. **Challenge Creation:** ACME server provides an HTTP-01 challenge token and expected authorization string. These are stored in-memory in a thread-safe map.
3. **Challenge Serving:** The `HTTPProxy` intercepts ANY request matching `/.well-known/acme-challenge/{token}`. It queries the `CertificateProvider` and returns the authorization string directly.
4. **Validation:** Let's Encrypt calls the proxy, verifies the challenge.
5. **Issuance:** CP downloads the certificate, generates the private key (or uses the pre-generated one), and stores the `*tls.Certificate` in memory.

## 7. TLS Proxy Flow & SNI
The `HTTPProxy` will bind to port 443 (HTTPS) in addition to port 80 (HTTP).
*   **Listener:** `tls.NewListener` configured with `GetConfigForClient`.
*   **SNI Selection:** `GetConfigForClient` intercepts `ClientHelloInfo.ServerName`. It requests the `*tls.Certificate` from the `CertificateProvider`.
*   **Unknown Domains:** If the domain is unknown or lacks a valid certificate, the connection is dropped during the TLS handshake. No fallback certificate is provided.
*   **HTTP -> HTTPS:** The port 80 handler will check if the requested `Host` has TLS enabled. If yes, it returns `308 Permanent Redirect` to `https://{Host}{RequestURI}`.

## 8. Ownership & Security
*   **Domain Ownership:** TLS can only be requested by an Operator for a Domain mapped to an Application they own.
*   **Challenge Isolation:** The `/.well-known/acme-challenge/` route is evaluated *before* Application routing. Malicious apps cannot define custom routes to intercept challenge tokens intended for the Control Plane.
*   **Private Key Security:** Keys exist solely in RAM. They cannot be exported via the API.

## 9. Renewal
*   Certificates are checked periodically (e.g., daily) by a background worker.
*   If a certificate is within 30 days of expiration (`ExpiresAt`), a renewal ACME flow is triggered.
*   **Failure:** Bounded retries (e.g., max 3 attempts per day) to prevent rate-limit bans if DNS is misconfigured.

## 10. Restart Behavior & V1 Limitations
**Crucial Limitation:** Because V1 strictly enforces "No Database" and "No Persistent Key Store", **all certificates and private keys are lost when the Control Plane restarts.**
*   **Behavior:** On CP startup, the reconciler will detect domains configured for TLS but lacking in-memory certificates. It will immediately trigger ACME requests for all of them.
*   **Risk - ACME Rate Limits:** Let's Encrypt enforces a limit of 50 certificates per registered domain per week. If the CP is restarted more than 50 times a week, issuance will fail, and HTTPS will be broken.
*   **Mitigation:** We will clearly document this limitation. Bounded concurrency will prevent slamming the ACME server at startup. The API will reflect `CertStatusFailed` with a clear "rate limit" error message if this occurs.

## 11. Rate Limit Protection
*   In-memory deduplication: Only one active ACME request per domain at a time (preventing stampeding herds).
*   Failed requests enforce a minimum backoff (e.g., 1 hour) before retrying.

## 12. HTTP / HTTPS State Semantics
*   **HTTPS + Valid Cert:** Routes to backend.
*   **HTTPS + Expired/Missing Cert:** Connection rejected at TLS handshake.
*   **HTTP + TLS Enabled:** 308 Redirect to HTTPS.
*   **HTTP + TLS Disabled:** Routes to backend (legacy behavior).
*   **Zero Healthy Backends:** 503 Service Unavailable (existing behavior).

## 13. API
*   `POST /api/v1/domains/{domain}/tls` - Enable TLS (Async).
*   `GET /api/v1/domains/{domain}/tls` - View status (Pending, Issued, ExpiresAt).
*   `DELETE /api/v1/domains/{domain}/tls` - Disable TLS and remove cert from memory.

## 14. CLI
*   `runstack domain tls enable <domain>`
*   `runstack domain tls status <domain>`
*   `runstack domain tls disable <domain>`

## 15. Concurrency
*   Use `sync.RWMutex` to protect the in-memory certificate map and challenge map.
*   Ensure `GetTLSCertificate` (called concurrently by every incoming HTTPS connection handshake) uses lock-free or `RLock` fast-paths.

## 16. Resource Safety
*   ACME HTTP clients must use strict `context.WithTimeout`.
*   Limit concurrent ACME issuance background workers (e.g., max 5 concurrent negotiations).

## 17. Observability
*   Log `Certificate requested`, `Certificate issued`, `ACME challenge generated`.
*   Log ACME errors (e.g., DNS resolution failed, rate limit hit).
*   **Never log:** Private keys, challenge expected payloads (beyond token ID), or ACME account private keys.

## 18. Implementation Order
1. Define `Certificate` domain model and `CertificateProvider` interface.
2. Implement in-memory `CertificateProvider` with ACME HTTP-01 support (using `x/crypto/acme` or similar).
3. Modify `HTTPProxy` to bind 443, implement `GetConfigForClient` for SNI, and handle `/.well-known/acme-challenge/` paths.
4. Add HTTP-to-HTTPS redirection in the port 80 listener.
5. Implement API endpoints (`RequireOperator`) for TLS management.
6. Implement CLI commands.
7. Add robust integration tests mocking the ACME server (or simulating the interface).

## 19. Tests
*   **Unit:** Challenge map insertion/retrieval, SNI selection, certificate expiration detection.
*   **Integration:** Multi-domain routing, TLS handshake rejection for unknown domains, HTTP->HTTPS redirect.
*   **Concurrency:** Race detector against simulated high-throughput TLS handshakes while a certificate is being renewed.

---
**Verdict:** `READY FOR 8L IMPLEMENTATION`
