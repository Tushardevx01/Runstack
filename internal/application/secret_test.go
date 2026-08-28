package application

import (
	"sync"
	"testing"
)

func TestSecretRegistry_SetAndGet(t *testing.T) {
	reg := NewSecretRegistry()

	sec, err := reg.Set("app1", "DB_PASS", "s3cr3t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.Name != "DB_PASS" {
		t.Errorf("expected DB_PASS, got %s", sec.Name)
	}

	val, err := reg.Resolve("app1", "DB_PASS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "s3cr3t" {
		t.Errorf("expected s3cr3t, got %s", val)
	}

	// Update
	sec2, err := reg.Set("app1", "DB_PASS", "new_secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec2.ID != sec.ID {
		t.Errorf("expected ID to be stable on update, got %s vs %s", sec2.ID, sec.ID)
	}

	val, _ = reg.Resolve("app1", "DB_PASS")
	if val != "new_secret" {
		t.Errorf("expected new_secret, got %s", val)
	}
}

func TestSecretRegistry_CrossAppIsolation(t *testing.T) {
	reg := NewSecretRegistry()
	_, _ = reg.Set("app1", "DB_PASS", "s3cr3t")

	_, err := reg.Resolve("app2", "DB_PASS")
	if err != ErrSecretNotFound {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestSecretRegistry_InvalidNames(t *testing.T) {
	reg := NewSecretRegistry()
	tests := []string{"", "has space", "has/slash", "has!bang"}

	for _, name := range tests {
		_, err := reg.Set("app1", name, "val")
		if err == nil {
			t.Errorf("expected error for name %q", name)
		}
	}
}

func TestSecretRegistry_Concurrent(t *testing.T) {
	reg := NewSecretRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = reg.Set("app1", "CONCURRENT_KEY", "val")
			_, _ = reg.Resolve("app1", "CONCURRENT_KEY")
		}(i)
	}

	wg.Wait()
}
