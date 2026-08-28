package ingress

import (
	"errors"
	"sync"
)

var (
	ErrIngressAlreadyExists = errors.New("ingress already exists for this domain and path")
	ErrIngressNotFound      = errors.New("ingress not found")
)

type Ingress struct {
	ID        string `json:"id"`
	DomainID  string `json:"domain_id"`
	ServiceID string `json:"service_id"`
	Path      string `json:"path"` // V1 defaults to "/"
}

type IngressRegistry struct {
	mu        sync.RWMutex
	ingresses map[string]Ingress
}

func NewIngressRegistry() *IngressRegistry {
	return &IngressRegistry{
		ingresses: make(map[string]Ingress),
	}
}

func (r *IngressRegistry) Create(domainID, serviceID, path string) (Ingress, error) {
	if path == "" {
		path = "/"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ing := range r.ingresses {
		if ing.DomainID == domainID && ing.Path == path {
			return Ingress{}, ErrIngressAlreadyExists
		}
	}

	ing := Ingress{
		ID:        generateID(),
		DomainID:  domainID,
		ServiceID: serviceID,
		Path:      path,
	}

	r.ingresses[ing.ID] = ing
	return ing, nil
}

func (r *IngressRegistry) Get(id string) (Ingress, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ing, exists := r.ingresses[id]
	if !exists {
		return Ingress{}, ErrIngressNotFound
	}
	return ing, nil
}

func (r *IngressRegistry) List() []Ingress {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Ingress, 0, len(r.ingresses))
	for _, ing := range r.ingresses {
		list = append(list, ing)
	}
	return list
}

func (r *IngressRegistry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.ingresses[id]; !exists {
		return ErrIngressNotFound
	}

	delete(r.ingresses, id)
	return nil
}
