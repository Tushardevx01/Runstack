package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tushardevx01/runstack/internal/runtime"
)

type FakeRuntime struct {
	mu         sync.Mutex
	Containers map[string]*FakeContainer
	ShouldFail bool
}

type FakeContainer struct {
	ID          string
	InstanceID  string
	ExecutionID string
	State       runtime.ContainerState
}

func New() *FakeRuntime {
	return &FakeRuntime{
		Containers: make(map[string]*FakeContainer),
	}
}

func (f *FakeRuntime) Start(ctx context.Context, spec runtime.ContainerSpec) (runtime.ContainerInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ShouldFail {
		return runtime.ContainerInfo{}, fmt.Errorf("fake start failed")
	}

	name := "runstack-" + spec.InstanceID

	c, exists := f.Containers[name]
	if exists {
		if c.ExecutionID != spec.ExecutionID {
			return runtime.ContainerInfo{}, runtime.ErrContainerConflict
		}
		return runtime.ContainerInfo{
			ContainerID: c.ID,
			State:       c.State,
		}, nil
	}

	f.Containers[name] = &FakeContainer{
		ID:          name,
		InstanceID:  spec.InstanceID,
		ExecutionID: spec.ExecutionID,
		State:       runtime.StateRunning,
	}

	return runtime.ContainerInfo{
		ContainerID: name,
		State:       runtime.StateRunning,
	}, nil
}

func (f *FakeRuntime) Stop(ctx context.Context, containerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ShouldFail {
		return fmt.Errorf("fake stop failed")
	}

	c, exists := f.Containers[containerID]
	if !exists {
		return nil
	}

	c.State = runtime.StateExited
	return nil
}

func (f *FakeRuntime) Status(ctx context.Context, containerID string) (runtime.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ShouldFail {
		return runtime.StateUnknown, fmt.Errorf("fake status failed")
	}

	c, exists := f.Containers[containerID]
	if !exists {
		return runtime.StateUnknown, runtime.ErrContainerNotFound
	}

	return c.State, nil
}

func (f *FakeRuntime) Remove(ctx context.Context, containerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ShouldFail {
		return fmt.Errorf("fake remove failed")
	}

	delete(f.Containers, containerID)
	return nil
}
