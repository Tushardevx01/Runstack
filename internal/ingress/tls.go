package ingress

import (
	"context"
	"time"
)

type Certificate struct {
	DomainName string
	CertPEM    []byte
	KeyPEM     []byte
	Expiry     time.Time
}

type CertificateProvider interface {
	Provision(ctx context.Context, domain string) (*Certificate, error)
	Renew(ctx context.Context, domain string) (*Certificate, error)
}
