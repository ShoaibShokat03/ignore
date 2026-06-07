package clipboard

import (
	"context"
	"log/slog"
	"reflect"
	"time"
)

type Event struct {
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

type PrepareFunc func(context.Context, FileList) (PreparedFileList, error)

type Monitor struct {
	log      *slog.Logger
	interval time.Duration
	enabled  func() bool
	prepare  PrepareFunc
	onEvent  func(Event)
}

func New(log *slog.Logger, interval time.Duration, enabled func() bool, prepare PrepareFunc, onEvent func(Event)) *Monitor {
	return &Monitor{log: log, interval: interval, enabled: enabled, prepare: prepare, onEvent: onEvent}
}

func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	m.log.Info("clipboard monitor started")
	var lastSequence uint32
	var pendingMoveCleanup func()
	var pendingMoveDeadline time.Time
	for {
		select {
		case <-ctx.Done():
			m.log.Info("clipboard monitor stopped")
			return
		case <-ticker.C:
			if m.enabled != nil && !m.enabled() {
				continue
			}
			if pendingMoveCleanup != nil {
				if readPerformedDropEffect()&DropEffectMove != 0 {
					pendingMoveCleanup()
					pendingMoveCleanup = nil
					pendingMoveDeadline = time.Time{}
				} else if !pendingMoveDeadline.IsZero() && time.Now().After(pendingMoveDeadline) {
					pendingMoveCleanup = nil
					pendingMoveDeadline = time.Time{}
				}
			}
			files, err := readFiles()
			if err != nil {
				m.log.Warn("clipboard read failed", "error", err)
				continue
			}
			if pendingMoveCleanup != nil && files.Sequence != 0 && files.Sequence != lastSequence {
				pendingMoveCleanup = nil
				pendingMoveDeadline = time.Time{}
			}
			if files.Sequence == 0 || files.Sequence == lastSequence || len(files.Paths) == 0 {
				continue
			}
			lastSequence = files.Sequence
			if m.prepare == nil {
				continue
			}
			filtered, err := m.prepare(ctx, files)
			if err != nil {
				m.log.Warn("clipboard filtering failed", "error", err)
				continue
			}
			if reflect.DeepEqual(files.Paths, filtered.Paths) {
				continue
			}
			if err := writeFiles(filtered.Paths, files.DropEffect); err != nil {
				m.log.Warn("clipboard replacement failed", "error", err)
				continue
			}
			if filtered.AfterMovePaste != nil && files.DropEffect == DropEffectMove {
				pendingMoveCleanup = filtered.AfterMovePaste
				pendingMoveDeadline = time.Now().Add(30 * time.Minute)
			}
			lastSequence = currentSequence()
			m.log.Info("clipboard file list replaced with filtered staging paths", "items", len(filtered.Paths))
			if m.onEvent != nil {
				m.onEvent(Event{Message: "Clipboard file list filtered", Time: time.Now()})
			}
		}
	}
}
