package sysinfo

import (
	"testing"
)

func TestGetMemoryInfo(t *testing.T) {
	mem := GetMemoryInfo()
	if mem.TotalBytes == 0 {
		t.Log("Total memory is 0, might be unsupported on this OS")
	} else {
		t.Logf("Total memory: %d, Available: %d", mem.TotalBytes, mem.AvailableBytes)
	}
}

func TestRuntimeChecks(t *testing.T) {
	docker := HasDocker()
	podman := HasPodman()
	t.Logf("Docker: %v, Podman: %v", docker, podman)
}
