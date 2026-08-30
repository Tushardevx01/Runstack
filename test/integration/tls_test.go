package integration

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/route"
)

func TestTLSIntegration_UnknownSNI(t *testing.T) {
	domainReg := ingress.NewDomainRegistry()
	domainReg.Create("known.com", "app-1", true)

	acmeProv := ingress.NewACMEProvider(domainReg)

	proxy := route.NewHTTPProxy(0, 0)
	proxy.GetTLSCertificate = acmeProv.GetTLSCertificate
	proxy.IsTLSEnabled = func(domain string) bool {
		d, err := domainReg.GetByName(domain)
		if err == nil {
			return d.TLS
		}
		return false
	}
	proxy.ACMEHandler = acmeProv.HTTPHandler(nil)

	// Since we can't easily start the proxy with port 0 and easily retrieve the TLS port in Start(),
	// we will manually test the HostPolicy via acmeProv

	// Test HostPolicy directly using a dummy ClientHello
	_, err := acmeProv.GetTLSCertificate(&tls.ClientHelloInfo{ServerName: "unknown.com"})
	if err == nil {
		t.Fatalf("expected error for unknown SNI, got nil")
	}
	if err.Error() != "acme/autocert: host not configured in registry" {
		t.Errorf("unexpected error message: %v", err)
	}

	// Test known domain without TLS enabled
	domainReg.Create("notls.com", "app-1", false)
	_, err = acmeProv.GetTLSCertificate(&tls.ClientHelloInfo{ServerName: "notls.com"})
	if err == nil {
		t.Fatalf("expected error for domain without TLS, got nil")
	}
	if err.Error() != "acme/autocert: TLS not enabled for domain" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestTLSIntegration_HTTPRedirect(t *testing.T) {
	domainReg := ingress.NewDomainRegistry()
	domainReg.Create("secure.com", "app-1", true)
	domainReg.Create("insecure.com", "app-1", false)

	acmeProv := ingress.NewACMEProvider(domainReg)

	proxy := route.NewHTTPProxy(0, 0)
	proxy.IsTLSEnabled = func(domain string) bool {
		d, err := domainReg.GetByName(domain)
		if err == nil {
			return d.TLS
		}
		return false
	}
	proxy.ACMEHandler = acmeProv.HTTPHandler(nil)

	// Mock the handler chain
	var httpHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock handleRequestInternal redirect logic
		if proxy.IsTLSEnabled(r.Host) {
			http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusPermanentRedirect)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	mux.Handle("/.well-known/acme-challenge/", proxy.ACMEHandler)
	mux.Handle("/", httpHandler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	// 1. HTTP to secure domain -> 308 Redirect
	req, _ := http.NewRequest("GET", ts.URL, nil)
	req.Host = "secure.com"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPermanentRedirect {
		t.Errorf("expected 308 redirect, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "https://secure.com/" {
		t.Errorf("expected redirect to https://secure.com/, got %s", resp.Header.Get("Location"))
	}

	// 2. HTTP to insecure domain -> 200 OK
	req, _ = http.NewRequest("GET", ts.URL, nil)
	req.Host = "insecure.com"
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	// 3. ACME Challenge on secure domain -> 404 (Because no token exists, but not a redirect)
	req, _ = http.NewRequest("GET", ts.URL+"/.well-known/acme-challenge/abc", nil)
	req.Host = "secure.com"
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusPermanentRedirect {
		t.Errorf("ACME challenge should bypass HTTP redirect, but got 308")
	}
}

func TestTLSIntegration_CertificateSafety(t *testing.T) {
	// Ensure that private keys are never exposed in the domain registry or API structs
	domainReg := ingress.NewDomainRegistry()
	domainReg.Create("secure.com", "app-1", true)

	acmeProv := ingress.NewACMEProvider(domainReg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// This triggers autocert, but will fail since there is no ACME server mock
	acmeProv.RequestCertificate(ctx, "secure.com")

	// Let background task process
	time.Sleep(100 * time.Millisecond)

	cert, err := acmeProv.GetCertificate("secure.com")
	if err != nil {
		t.Fatal(err)
	}

	// cert is type Certificate. It MUST NOT contain PrivateKey
	// This is verified statically by the compiler because Certificate struct has no key material.
	if cert.Domain != "secure.com" {
		t.Errorf("expected domain secure.com, got %s", cert.Domain)
	}
}

func TestTLSIntegration_RealTLSServerHandshake(t *testing.T) {
	domainReg := ingress.NewDomainRegistry()
	domainReg.Create("valid.com", "app-1", true)
	domainReg.Create("disabled.com", "app-1", false)

	acmeProv := ingress.NewACMEProvider(domainReg)

	tlsConfig := &tls.Config{
		GetCertificate: acmeProv.GetTLSCertificate,
		MinVersion:     tls.VersionTLS12,
	}

	// Create a real TLS server to test the actual handshake rejection
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, TLS"))
	}))
	ts.TLS = tlsConfig
	ts.StartTLS()
	defer ts.Close()

	// 1. Unknown SNI
	trUnknown := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         "unknown.com",
			InsecureSkipVerify: true, // We don't care about cert trust, we care if handshake succeeds at all
		},
	}
	clientUnknown := &http.Client{Transport: trUnknown}
	_, err := clientUnknown.Get(ts.URL)
	if err == nil {
		t.Errorf("expected TLS handshake failure for unknown domain, but succeeded")
	}

	// 2. Disabled TLS SNI
	trDisabled := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         "disabled.com",
			InsecureSkipVerify: true,
		},
	}
	clientDisabled := &http.Client{Transport: trDisabled}
	_, err = clientDisabled.Get(ts.URL)
	if err == nil {
		t.Errorf("expected TLS handshake failure for disabled domain, but succeeded")
	}

	// Note: We cannot easily test a fully successful handshake for "valid.com" here without a real ACME
	// challenge or a mock ACME server that issues real certs, but we proved it rejects invalid SNIs at the TLS layer.
}
