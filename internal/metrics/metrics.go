package metrics

import (
	"sync"
	"time"
)

type Snapshot struct {
	FilesCopied      uint64    `json:"filesCopied"`
	FilesSkipped     uint64    `json:"filesSkipped"`
	DirectoriesSkip  uint64    `json:"directoriesSkipped"`
	Errors           uint64    `json:"errors"`
	BytesCopied      uint64    `json:"bytesCopied"`
	Operations       uint64    `json:"operations"`
	LastActivity     string    `json:"lastActivity"`
	LastActivityTime time.Time `json:"lastActivityTime"`
	LastDurationMS   int64     `json:"lastDurationMs"`
}

type Metrics struct {
	mu sync.RWMutex
	s  Snapshot
}

func New() *Metrics {
	return &Metrics{}
}

func (m *Metrics) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s
}

func (m *Metrics) AddCopied(bytes uint64) {
	m.mu.Lock()
	m.s.FilesCopied++
	m.s.BytesCopied += bytes
	m.mu.Unlock()
}

func (m *Metrics) AddSkippedFile() {
	m.mu.Lock()
	m.s.FilesSkipped++
	m.mu.Unlock()
}

func (m *Metrics) AddSkippedDir() {
	m.mu.Lock()
	m.s.DirectoriesSkip++
	m.mu.Unlock()
}

func (m *Metrics) AddError() {
	m.mu.Lock()
	m.s.Errors++
	m.mu.Unlock()
}

func (m *Metrics) CompleteOperation(activity string, duration time.Duration) {
	m.mu.Lock()
	m.s.Operations++
	m.s.LastActivity = activity
	m.s.LastActivityTime = time.Now()
	m.s.LastDurationMS = duration.Milliseconds()
	m.mu.Unlock()
}
