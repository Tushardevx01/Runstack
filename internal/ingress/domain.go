package ingress

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
)

var (
	ErrDomainAlreadyExists = errors.New("domain already exists")
	ErrDomainNotFound      = errors.New("domain not found")
	ErrInvalidDomain       = errors.New("invalid domain name")
	ErrDomainHijack        = errors.New("domain belongs to another application")
)

type DomainStatus string

const (
	DomainStatusPending   DomainStatus = "Pending"
	DomainStatusActive    DomainStatus = "Active"
	DomainStatusTLSFailed DomainStatus = "TLSFailed"
)

type Domain struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	ApplicationID string       `json:"application_id"`
	TLS           bool         `json:"tls"`
	Status        DomainStatus `json:"status"`
}

type DomainRegistry struct {
	mu      sync.RWMutex
	domains map[string]Domain
	names   map[string]string // Name to ID
}

func NewDomainRegistry() *DomainRegistry {
	return &DomainRegistry{
		domains: make(map[string]Domain),
		names:   make(map[string]string),
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NormalizeDomain(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

func (r *DomainRegistry) Create(name string, appID string, tls bool) (Domain, error) {
	name = NormalizeDomain(name)
	if name == "" {
		return Domain{}, ErrInvalidDomain
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if ownerID, exists := r.names[name]; exists {
		// Even if same app, it already exists. We could return existing or error.
		// For now, return ErrDomainAlreadyExists
		if r.domains[ownerID].ApplicationID != appID {
			return Domain{}, ErrDomainHijack
		}
		return Domain{}, ErrDomainAlreadyExists
	}

	d := Domain{
		ID:            generateID(),
		Name:          name,
		ApplicationID: appID,
		TLS:           tls,
		Status:        DomainStatusPending,
	}

	r.domains[d.ID] = d
	r.names[name] = d.ID
	return d, nil
}

func (r *DomainRegistry) Get(id string) (Domain, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, exists := r.domains[id]
	if !exists {
		return Domain{}, ErrDomainNotFound
	}
	return d, nil
}

func (r *DomainRegistry) GetByName(name string) (Domain, error) {
	name = NormalizeDomain(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	id, exists := r.names[name]
	if !exists {
		return Domain{}, ErrDomainNotFound
	}

	d, exists := r.domains[id]
	if !exists {
		return Domain{}, ErrDomainNotFound
	}

	return d, nil
}

func (r *DomainRegistry) List() []Domain {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Domain, 0, len(r.domains))
	for _, d := range r.domains {
		list = append(list, d)
	}
	return list
}

func (r *DomainRegistry) UpdateStatus(id string, status DomainStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, exists := r.domains[id]
	if !exists {
		return ErrDomainNotFound
	}

	d.Status = status
	r.domains[id] = d
	return nil
}

func (r *DomainRegistry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, exists := r.domains[id]
	if !exists {
		return ErrDomainNotFound
	}

	delete(r.names, d.Name)
	delete(r.domains, id)
	return nil
}
