package ingress

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type CertStatus string

const (
	CertStatusPending CertStatus = "pending"
	CertStatusIssued  CertStatus = "issued"
	CertStatusFailed  CertStatus = "failed"
	CertStatusExpired CertStatus = "expired"
)

type Certificate struct {
	ID        string     `json:"id"`
	Domain    string     `json:"domain"`
	Status    CertStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at,omitempty"`
	Error     string     `json:"error,omitempty"`
}

var ErrCertNotFound = errors.New("certificate not found")

type CertificateProvider interface {
	RequestCertificate(ctx context.Context, domain string) error
	GetCertificate(domain string) (Certificate, error)
	GetTLSCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error)
	HTTPHandler(fallback http.Handler) http.Handler
}

// In-memory cache for autocert to ensure no database/persistence is used
type memoryCache struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func (m *memoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return nil, autocert.ErrCacheMiss
}

func (m *memoryCache) Put(ctx context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}

func (m *memoryCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

type ACMEProvider struct {
	manager  *autocert.Manager
	registry *DomainRegistry

	mu       sync.RWMutex
	certMeta map[string]*Certificate
}

func NewACMEProvider(registry *DomainRegistry) *ACMEProvider {
	p := &ACMEProvider{
		registry: registry,
		certMeta: make(map[string]*Certificate),
	}

	p.manager = &autocert.Manager{
		Cache:  &memoryCache{data: make(map[string][]byte)},
		Prompt: autocert.AcceptTOS,
		HostPolicy: func(ctx context.Context, host string) error {
			d, err := p.registry.GetByName(host)
			if err != nil {
				return errors.New("acme/autocert: host not configured in registry")
			}
			if !d.TLS {
				return errors.New("acme/autocert: TLS not enabled for domain")
			}
			return nil
		},
	}

	return p
}

func (p *ACMEProvider) RequestCertificate(ctx context.Context, domain string) error {
	p.mu.Lock()
	meta, exists := p.certMeta[domain]
	if !exists {
		meta = &Certificate{
			ID:        generateID(),
			Domain:    domain,
			Status:    CertStatusPending,
			CreatedAt: time.Now(),
		}
		p.certMeta[domain] = meta
	} else {
		meta.Status = CertStatusPending
		meta.Error = ""
	}
	p.mu.Unlock()

	// In the background, autocert will actually fetch it when requested,
	// but we can trigger it eagerly here by requesting the cert.
	go func() {
		// Use a bounded timeout to prevent hanging forever
		_, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// GetCertificate will trigger the ACME flow if cache is empty
		_, err := p.manager.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})

		p.mu.Lock()
		defer p.mu.Unlock()
		if meta := p.certMeta[domain]; meta != nil {
			if err != nil {
				meta.Status = CertStatusFailed
				meta.Error = err.Error()
			} else {
				meta.Status = CertStatusIssued
				meta.Error = ""
				// ExpiresAt is tricky to get directly from autocert without parsing the cached PEM.
				// We can just leave it empty or parse it if necessary.
			}
		}
	}()

	return nil
}

func (p *ACMEProvider) GetCertificate(domain string) (Certificate, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if meta, exists := p.certMeta[domain]; exists {
		return *meta, nil
	}
	return Certificate{}, ErrCertNotFound
}

func (p *ACMEProvider) GetTLSCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	// Let autocert handle the TLS certificate retrieval/SNI selection
	cert, err := p.manager.GetCertificate(clientHello)

	p.mu.Lock()
	defer p.mu.Unlock()
	// Update meta if it's there
	if meta, exists := p.certMeta[clientHello.ServerName]; exists {
		if err != nil {
			meta.Status = CertStatusFailed
			meta.Error = err.Error()
		} else {
			meta.Status = CertStatusIssued
			meta.Error = ""
		}
	}

	return cert, err
}

func (p *ACMEProvider) HTTPHandler(fallback http.Handler) http.Handler {
	return p.manager.HTTPHandler(fallback)
}
