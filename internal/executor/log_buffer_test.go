package executor

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/Tushardevx01/runstack/internal/runtime"
)

type mockRuntime struct {
	runtime.ContainerRuntime // Embedding the interface to panic on unexpected calls
	logs                     map[string]string
	mu                       sync.RWMutex
}

func (m *mockRuntime) Logs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	m.mu.RLock()
	str := m.logs[containerID]
	m.mu.RUnlock()
	return io.NopCloser(strings.NewReader(str)), nil
}

func TestLogRingBuffer_Eviction(t *testing.T) {
	cr := &mockRuntime{logs: make(map[string]string)}
	buf := NewLogRingBuffer(cr)

	largeStr := strings.Repeat("a", MaxBytesPerInstance) // 1MB string
	cr.logs["container-1"] = largeStr
	cr.logs["container-2"] = largeStr

	// Create enough instances to evict
	for i := 0; i < 55; i++ {
		id := "inst-" + string(rune(i))
		cr.logs[id] = largeStr
		buf.CaptureAndFreeze(context.Background(), "app-1", "dep-1", id, "exec-1", "node-1", id)
	}

	buf.mu.RLock()
	if len(buf.records) > 50 {
		t.Errorf("expected max 50 records, got %d", len(buf.records))
	}
	buf.mu.RUnlock()

	_, ok := buf.Get("inst-\x00", "app-1", "exec-1") // The first one added
	if ok {
		t.Errorf("expected first instance to be evicted")
	}
}

func TestLogRingBuffer_ExecutionFencing(t *testing.T) {
	cr := &mockRuntime{logs: map[string]string{"c1": "hello world\n"}}
	buf := NewLogRingBuffer(cr)

	buf.CaptureAndFreeze(context.Background(), "app-1", "dep-1", "inst-1", "exec-1", "node-1", "c1")

	// Try wrong app
	_, ok := buf.Get("inst-1", "app-2", "exec-1")
	if ok {
		t.Errorf("should fail with wrong app")
	}

	// Try wrong exec
	_, ok = buf.Get("inst-1", "app-1", "exec-2")
	if ok {
		t.Errorf("should fail with wrong exec")
	}

	// Correct
	lines, ok := buf.Get("inst-1", "app-1", "exec-1")
	if !ok || len(lines) == 0 || lines[0] != "hello world" {
		t.Errorf("failed to get correct logs")
	}
}

func TestLogRingBuffer_Concurrency(t *testing.T) {
	cr := &mockRuntime{logs: make(map[string]string)}
	buf := NewLogRingBuffer(cr)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "inst-c-" + string(rune(i))
			cr.mu.Lock()
			cr.logs[id] = "hello"
			cr.mu.Unlock()
			buf.CaptureAndFreeze(context.Background(), "app-1", "dep-1", id, "exec-1", "node-1", id)
			buf.Get(id, "app-1", "exec-1")
		}(i)
	}
	wg.Wait()
}
