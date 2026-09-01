package executor

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Tushardevx01/runstack/internal/runtime"
)

const (
	MaxLinesPerInstance = 500
	MaxBytesPerInstance = 1 * 1024 * 1024  // 1 MB
	MaxAgentLogBytes    = 50 * 1024 * 1024 // 50 MB
)

type CrashLogRecord struct {
	ApplicationID string
	DeploymentID  string
	InstanceID    string
	ExecutionID   string
	NodeID        string
	Lines         []string
	SizeBytes     int
	CapturedAt    time.Time
}

type LogRingBuffer struct {
	mu         sync.RWMutex
	records    map[string]*CrashLogRecord // Keyed by InstanceID
	totalBytes int
	order      []string // Tracks insertion order for eviction (oldest first)
	runtime    runtime.ContainerRuntime
}

func NewLogRingBuffer(cr runtime.ContainerRuntime) *LogRingBuffer {
	return &LogRingBuffer{
		records: make(map[string]*CrashLogRecord),
		order:   []string{},
		runtime: cr,
	}
}

// CaptureAndFreeze connects to the runtime, retrieves the tail of the logs,
// and stores them immutably.
func (b *LogRingBuffer) CaptureAndFreeze(ctx context.Context, appID, depID, instID, execID, nodeID, containerID string) {
	b.mu.Lock()
	if _, exists := b.records[instID]; exists {
		b.mu.Unlock()
		return // Already captured
	}
	b.mu.Unlock()

	// Capture logs from runtime with a short timeout
	capCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reader, err := b.runtime.Logs(capCtx, containerID)
	if err != nil {
		return
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, int64(MaxBytesPerInstance)))
	if err != nil && err != io.EOF {
		// Proceed with what we have
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > MaxLinesPerInstance {
		lines = lines[len(lines)-MaxLinesPerInstance:]
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // trim trailing empty line
	}

	size := 0
	for _, l := range lines {
		size += len(l)
	}

	record := &CrashLogRecord{
		ApplicationID: appID,
		DeploymentID:  depID,
		InstanceID:    instID,
		ExecutionID:   execID,
		NodeID:        nodeID,
		Lines:         lines,
		SizeBytes:     size,
		CapturedAt:    time.Now(),
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Double check
	if _, exists := b.records[instID]; exists {
		return
	}

	// Evict if we exceed global max
	for b.totalBytes+size > MaxAgentLogBytes && len(b.order) > 0 {
		oldestID := b.order[0]
		b.order = b.order[1:]
		if oldRec, ok := b.records[oldestID]; ok {
			b.totalBytes -= oldRec.SizeBytes
			delete(b.records, oldestID)
		}
	}

	// It's possible the new record alone exceeds MaxAgentLogBytes if configured badly,
	// but MaxBytesPerInstance should be << MaxAgentLogBytes.
	b.records[instID] = record
	b.order = append(b.order, instID)
	b.totalBytes += size
}

func (b *LogRingBuffer) Get(instID, appID, execID string) ([]string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	rec, exists := b.records[instID]
	if !exists {
		return nil, false
	}

	// Execution and ownership fencing
	if rec.ApplicationID != appID || (execID != "" && rec.ExecutionID != execID) {
		return nil, false
	}

	return rec.Lines, true
}
