package deployment

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
)

var (
	ErrNotFound = errors.New("deployment not found")
)

type Registry struct {
	mu          sync.RWMutex
	deployments map[string]Deployment
	appVersions map[string]int // tracks the latest version number for an AppID
}

func NewRegistry() *Registry {
	return &Registry{
		deployments: make(map[string]Deployment),
		appVersions: make(map[string]int),
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Registry) Create(appID string, spec application.AppSpec) (Deployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	version := r.appVersions[appID] + 1
	r.appVersions[appID] = version

	// We must deep copy the spec to ensure the snapshot is completely detached
	// from whatever pointer the caller passed in.

	// Create a dummy app to use its DeepCopy for spec
	dummy := application.Application{Spec: spec}
	copiedSpec := dummy.DeepCopy().Spec

	// Hash computation
	b, _ := json.Marshal(copiedSpec)
	h := sha256.Sum256(b)
	hashStr := hex.EncodeToString(h[:])

	dep := Deployment{
		ID:            generateID(),
		ApplicationID: appID,
		Version:       version,
		SpecSnapshot:  copiedSpec,
		Hash:          hashStr,
		Status:        StatusActive,
		RolloutStatus: RolloutPending,
		CreatedAt:     time.Now().UTC(),
	}

	r.deployments[dep.ID] = dep.DeepCopy()

	return dep.DeepCopy(), nil
}

func (r *Registry) Get(id string) (Deployment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dep, exists := r.deployments[id]
	if !exists {
		return Deployment{}, ErrNotFound
	}
	return dep.DeepCopy(), nil
}

func (r *Registry) List() []Deployment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Deployment
	for _, dep := range r.deployments {
		result = append(result, dep.DeepCopy())
	}
	return result
}

func (r *Registry) ListByApplication(appID string) []Deployment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Deployment
	for _, dep := range r.deployments {
		if dep.ApplicationID == appID {
			result = append(result, dep.DeepCopy())
		}
	}
	return result
}

func (r *Registry) UpdateStatus(id string, status DeploymentStatus) (Deployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dep, exists := r.deployments[id]
	if !exists {
		return Deployment{}, ErrNotFound
	}

	dep.Status = status
	r.deployments[id] = dep.DeepCopy()

	return dep.DeepCopy(), nil
}

func (r *Registry) UpdateState(id string, status DeploymentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dep, exists := r.deployments[id]
	if !exists {
		return ErrNotFound
	}

	dep.Status = status
	r.deployments[id] = dep
	return nil
}

func (r *Registry) RecordCrash(id string, threshold int) (Deployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dep, exists := r.deployments[id]
	if !exists {
		return Deployment{}, ErrNotFound
	}

	dep.ConsecutiveCrashes++
	if dep.ConsecutiveCrashes >= threshold {
		dep.Degraded = true
	}

	r.deployments[id] = dep.DeepCopy()
	return dep.DeepCopy(), nil
}

func (r *Registry) ResetCrashCounter(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dep, exists := r.deployments[id]
	if !exists {
		return ErrNotFound
	}

	dep.ConsecutiveCrashes = 0
	// We intentionally DO NOT clear Degraded here. It requires manual/explicit intervention to clear.
	r.deployments[id] = dep.DeepCopy()
	return nil
}

const MaxCrashLoopThreshold = 5

func (r *Registry) UpdateRolloutStatus(id string, status RolloutStatus, desired, updated, ready, unavailable int, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dep, exists := r.deployments[id]
	if !exists {
		return ErrNotFound
	}

	dep.RolloutStatus = status
	dep.DesiredReplicas = desired
	dep.UpdatedReplicas = updated
	dep.ReadyReplicas = ready
	dep.UnavailableReplicas = unavailable
	dep.BlockedReason = reason

	r.deployments[id] = dep.DeepCopy()
	return nil
}

func (r *Registry) UpdateRolloutStatusOnly(id string, status RolloutStatus, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dep, exists := r.deployments[id]
	if !exists {
		return ErrNotFound
	}

	dep.RolloutStatus = status
	dep.BlockedReason = reason

	r.deployments[id] = dep.DeepCopy()
	return nil
}
