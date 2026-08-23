package node

import (
	"errors"
	"sync"
)

var (
	ErrNoPortsAvailable = errors.New("no ports available")
)

type PortAllocator struct {
	mu          sync.Mutex
	minPort     int
	maxPort     int
	allocated   map[int]string   // Port -> InstanceID
	instanceMap map[string][]int // InstanceID -> []Port
}

func NewPortAllocator(minPort, maxPort int) *PortAllocator {
	return &PortAllocator{
		minPort:     minPort,
		maxPort:     maxPort,
		allocated:   make(map[int]string),
		instanceMap: make(map[string][]int),
	}
}

func (pa *PortAllocator) Allocate(instanceID string) (int, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	for p := pa.minPort; p <= pa.maxPort; p++ {
		if _, inUse := pa.allocated[p]; !inUse {
			pa.allocated[p] = instanceID
			pa.instanceMap[instanceID] = append(pa.instanceMap[instanceID], p)
			return p, nil
		}
	}
	return 0, ErrNoPortsAvailable
}

func (pa *PortAllocator) Release(instanceID string) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	ports, ok := pa.instanceMap[instanceID]
	if !ok {
		return
	}

	for _, p := range ports {
		delete(pa.allocated, p)
	}
	delete(pa.instanceMap, instanceID)
}

func (pa *PortAllocator) Sync(allocated map[int]string) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.allocated = make(map[int]string)
	pa.instanceMap = make(map[string][]int)

	for p, instID := range allocated {
		pa.allocated[p] = instID
		pa.instanceMap[instID] = append(pa.instanceMap[instID], p)
	}
}
