package ingress

import (
	"testing"
)

func TestDomainRegistry_Create(t *testing.T) {
	r := NewDomainRegistry()

	d, err := r.Create("API.Example.Com ", "app-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if d.Name != "api.example.com" {
		t.Errorf("expected normalized name 'api.example.com', got '%s'", d.Name)
	}
	if d.ApplicationID != "app-1" {
		t.Errorf("expected app-1, got %s", d.ApplicationID)
	}

	// Test Duplicate
	_, err = r.Create("api.example.com", "app-1", false)
	if err != ErrDomainAlreadyExists {
		t.Errorf("expected ErrDomainAlreadyExists, got %v", err)
	}

	// Test Hijack
	_, err = r.Create("api.example.com", "app-2", false)
	if err != ErrDomainHijack {
		t.Errorf("expected ErrDomainHijack, got %v", err)
	}
}

func TestDomainRegistry_Delete(t *testing.T) {
	r := NewDomainRegistry()
	d, _ := r.Create("test.com", "app-1", false)

	err := r.Delete(d.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = r.Get(d.ID)
	if err != ErrDomainNotFound {
		t.Errorf("expected ErrDomainNotFound, got %v", err)
	}
}
