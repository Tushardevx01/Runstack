package application

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"
)

var (
	ErrSecretNotFound    = errors.New("secret not found")
	ErrSecretConflict    = errors.New("secret already exists")
	ErrInvalidSecretName = errors.New("invalid secret name")
)

type Secret struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	ApplicationID string    `json:"application_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SecretEntry struct {
	Secret Secret
	Value  string // Plaintext value, never serialized in API responses
}

type SecretRegistry struct {
	mu      sync.RWMutex
	secrets map[string]*SecretEntry // map[ID]*SecretEntry
	nameMap map[string]string       // map[AppID:Name]ID
}

func NewSecretRegistry() *SecretRegistry {
	return &SecretRegistry{
		secrets: make(map[string]*SecretEntry),
		nameMap: make(map[string]string),
	}
}

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func ValidateSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name cannot be empty", ErrInvalidSecretName)
	}
	if len(name) > 255 {
		return fmt.Errorf("%w: name too long", ErrInvalidSecretName)
	}
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("%w: name contains invalid characters", ErrInvalidSecretName)
	}
	return nil
}

func (r *SecretRegistry) key(appID, name string) string {
	return fmt.Sprintf("%s:%s", appID, name)
}

func (r *SecretRegistry) Set(appID, name, value string) (Secret, error) {
	if err := ValidateSecretName(name); err != nil {
		return Secret{}, err
	}
	if appID == "" {
		return Secret{}, errors.New("application id cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	k := r.key(appID, name)
	if id, exists := r.nameMap[k]; exists {
		// Update existing
		entry := r.secrets[id]
		entry.Value = value
		entry.Secret.UpdatedAt = time.Now().UTC()
		return entry.Secret, nil
	}

	// Create new
	id := fmt.Sprintf("sec-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	sec := Secret{
		ID:            id,
		Name:          name,
		ApplicationID: appID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	r.secrets[id] = &SecretEntry{
		Secret: sec,
		Value:  value,
	}
	r.nameMap[k] = id

	return sec, nil
}

func (r *SecretRegistry) Get(id string) (Secret, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, exists := r.secrets[id]; exists {
		return entry.Secret, nil
	}
	return Secret{}, ErrSecretNotFound
}

func (r *SecretRegistry) Resolve(appID, name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, exists := r.nameMap[r.key(appID, name)]
	if !exists {
		return "", ErrSecretNotFound
	}
	entry := r.secrets[id]
	return entry.Value, nil
}

func (r *SecretRegistry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.secrets[id]
	if !exists {
		return ErrSecretNotFound
	}

	delete(r.nameMap, r.key(entry.Secret.ApplicationID, entry.Secret.Name))
	delete(r.secrets, id)
	return nil
}

func (r *SecretRegistry) DeleteByName(appID, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := r.key(appID, name)
	id, exists := r.nameMap[k]
	if !exists {
		return ErrSecretNotFound
	}

	delete(r.nameMap, k)
	delete(r.secrets, id)
	return nil
}

func (r *SecretRegistry) ListByApp(appID string) []Secret {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res []Secret
	for _, entry := range r.secrets {
		if entry.Secret.ApplicationID == appID {
			res = append(res, entry.Secret)
		}
	}
	return res
}

func (r *SecretRegistry) GetByName(appID, name string) (Secret, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, exists := r.nameMap[r.key(appID, name)]
	if !exists {
		return Secret{}, ErrSecretNotFound
	}
	entry := r.secrets[id]
	return entry.Secret, nil
}
