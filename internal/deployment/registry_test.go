package deployment

import (
	"github.com/Tushardevx01/runstack/internal/application"
	"testing"
)

func TestRegistry_CreateGet(t *testing.T) {
	r := NewRegistry()
	spec := application.AppSpec{
		Image:    "nginx:latest",
		Replicas: 3,
	}

	dep, err := r.Create("app-1", spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dep.Version != 1 {
		t.Errorf("expected version 1, got %d", dep.Version)
	}
	if dep.Status != StatusActive {
		t.Errorf("expected PENDING, got %s", dep.Status)
	}

	dep2, _ := r.Create("app-1", spec)
	if dep2.Version != 2 {
		t.Errorf("expected version 2, got %d", dep2.Version)
	}
}

func TestRegistry_Immutability(t *testing.T) {
	r := NewRegistry()
	spec := application.AppSpec{
		Image:       "nginx:latest",
		Replicas:    3,
		Environment: map[string]string{"ENV": "prod"},
	}

	dep, _ := r.Create("app-1", spec)

	// Caller attempts to mutate returned object
	dep.SpecSnapshot.Environment["ENV"] = "dev"
	dep.ApplicationID = "hacked"
	dep.Version = 999

	// Verify internal state is unchanged
	got, _ := r.Get(dep.ID)
	if got.SpecSnapshot.Environment["ENV"] != "prod" {
		t.Errorf("deployment spec was mutated via map pointer!")
	}
	if got.ApplicationID != "app-1" {
		t.Errorf("deployment app ID was mutated!")
	}
	if got.Version != 1 {
		t.Errorf("deployment version was mutated!")
	}

	// Caller attempts to mutate the spec they passed in originally
	spec.Environment["ENV"] = "hacked-original"
	got2, _ := r.Get(dep.ID)
	if got2.SpecSnapshot.Environment["ENV"] != "prod" {
		t.Errorf("deployment spec was mutated via original spec pointer!")
	}
}
