package application

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("application not found")
	ErrAlreadyExists = errors.New("application already exists with this name")
)

type Registry struct {
	mu    sync.RWMutex
	apps  map[string]Application
	names map[string]string // maps Name to ID to enforce unique names
}

func NewRegistry() *Registry {
	return &Registry{
		apps:  make(map[string]Application),
		names: make(map[string]string),
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Registry) Create(name string, spec AppSpec) (Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.names[name]; exists {
		return Application{}, ErrAlreadyExists
	}

	app := Application{
		ID:        generateID(),
		Name:      name,
		Spec:      spec,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	appCopy := app.DeepCopy()
	r.apps[app.ID] = appCopy
	r.names[name] = app.ID

	return appCopy.DeepCopy(), nil
}

func (r *Registry) Get(id string) (Application, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	app, exists := r.apps[id]
	if !exists {
		return Application{}, ErrNotFound
	}
	return app.DeepCopy(), nil
}

func (r *Registry) List() []Application {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Application
	for _, app := range r.apps {
		result = append(result, app.DeepCopy())
	}
	return result
}

func (r *Registry) Update(id string, spec AppSpec, activeDeploymentID string, status AppStatus) (Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	app, exists := r.apps[id]
	if !exists {
		return Application{}, ErrNotFound
	}

	app.Spec = spec
	app.ActiveDeploymentID = activeDeploymentID
	app.Status = status
	app.UpdatedAt = time.Now().UTC()

	appCopy := app.DeepCopy()
	r.apps[id] = appCopy
	return appCopy.DeepCopy(), nil
}
