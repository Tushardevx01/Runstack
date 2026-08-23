package route

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("service not found")
)

type Registry struct {
	mu       sync.RWMutex
	services map[string]Service
}

func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]Service),
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Registry) Create(appID, domain, pathPrefix string, targetPort int, protocol Protocol) (Service, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	srv := Service{
		ID:            generateID(),
		ApplicationID: appID,
		Domain:        domain,
		PathPrefix:    pathPrefix,
		TargetPort:    targetPort,
		Protocol:      protocol,
		CreatedAt:     time.Now().UTC(),
	}

	r.services[srv.ID] = srv
	return srv, nil
}

func (r *Registry) Get(id string) (Service, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	srv, exists := r.services[id]
	if !exists {
		return Service{}, ErrNotFound
	}
	return srv, nil
}

func (r *Registry) List() []Service {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Service
	for _, srv := range r.services {
		result = append(result, srv)
	}
	return result
}

func (r *Registry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.services[id]; !exists {
		return ErrNotFound
	}

	delete(r.services, id)
	return nil
}
