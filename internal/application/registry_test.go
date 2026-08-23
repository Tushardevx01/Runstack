package application

import (
	"fmt"
	"sync"
	"testing"
)

func TestRegistry_CreateGet(t *testing.T) {
	r := NewRegistry()
	spec := AppSpec{
		Image:       "nginx:latest",
		Replicas:    3,
		Environment: map[string]string{"ENV": "prod"},
	}

	app, err := r.Create("frontend", spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if app.Name != "frontend" {
		t.Errorf("expected frontend, got %s", app.Name)
	}

	got, err := r.Get(app.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != app.ID {
		t.Errorf("expected %s, got %s", app.ID, got.ID)
	}
}

func TestRegistry_Immutability(t *testing.T) {
	r := NewRegistry()
	spec := AppSpec{
		Image:       "nginx:latest",
		Replicas:    3,
		Environment: map[string]string{"ENV": "prod"},
		Command:     []string{"start"},
	}

	app, _ := r.Create("frontend", spec)

	app.Spec.Environment["ENV"] = "dev"
	app.Spec.Command[0] = "stop"
	app.Name = "hacked"

	got, _ := r.Get(app.ID)
	if got.Spec.Environment["ENV"] != "prod" {
		t.Errorf("registry state was mutated via map pointer!")
	}
	if got.Spec.Command[0] != "start" {
		t.Errorf("registry state was mutated via slice pointer!")
	}
	if got.Name != "frontend" {
		t.Errorf("registry state was mutated!")
	}
}

func TestRegistry_Concurrency(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			spec := AppSpec{Replicas: i}
			name := fmt.Sprintf("app-%d", i)
			app, _ := r.Create(name, spec)
			r.Update(app.ID, spec, "dep-1", StatusReady)
			r.Get(app.ID)
			r.List()
		}(i)
	}
	wg.Wait()
}
