package node

import (
	"testing"
)

func TestPortAllocator(t *testing.T) {
	pa := NewPortAllocator(30000, 30002)

	p1, err := pa.Allocate("inst1")
	if err != nil || p1 != 30000 {
		t.Fatalf("expected 30000, got %v, err %v", p1, err)
	}

	p2, err := pa.Allocate("inst2")
	if err != nil || p2 != 30001 {
		t.Fatalf("expected 30001, got %v, err %v", p2, err)
	}

	p3, err := pa.Allocate("inst3")
	if err != nil || p3 != 30002 {
		t.Fatalf("expected 30002, got %v, err %v", p3, err)
	}

	_, err = pa.Allocate("inst4")
	if err != ErrNoPortsAvailable {
		t.Fatalf("expected ErrNoPortsAvailable, got %v", err)
	}

	pa.Release("inst2")

	p4, err := pa.Allocate("inst4")
	if err != nil || p4 != 30001 {
		t.Fatalf("expected 30001 (reused), got %v, err %v", p4, err)
	}

	// Ensure no collision
	if p1 == p4 || p3 == p4 {
		t.Fatalf("port collision detected!")
	}
}
