package watcher

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	log      *slog.Logger
	delay    time.Duration
	onChange func(string)
	watcher  *fsnotify.Watcher
	mu       sync.Mutex
	timers   map[string]*time.Timer
}

func New(log *slog.Logger, delay time.Duration, onChange func(string)) *Watcher {
	return &Watcher{log: log, delay: delay, onChange: onChange, timers: make(map[string]*time.Timer)}
}

func (w *Watcher) WatchFile(path string) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = fsw
	return fsw.Add(filepath.Dir(path))
}

func (w *Watcher) Run(ctx context.Context) {
	if w.watcher == nil {
		return
	}
	defer w.watcher.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 && filepath.Base(ev.Name) == ".ignore" {
				w.debounce(ev.Name)
			}
		case err, ok := <-w.watcher.Errors:
			if ok {
				w.log.Warn("watcher error", "error", err)
			}
		}
	}
}

func (w *Watcher) debounce(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t := w.timers[path]; t != nil {
		t.Reset(w.delay)
		return
	}
	w.timers[path] = time.AfterFunc(w.delay, func() {
		w.mu.Lock()
		delete(w.timers, path)
		w.mu.Unlock()
		w.onChange(path)
	})
}
