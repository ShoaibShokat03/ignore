package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	maxLogBytes = 5 * 1024 * 1024
	maxFiles    = 5
)

type RotatingWriter struct {
	mu   sync.Mutex
	dir  string
	file *os.File
	size int64
}

func NewLogger(dir string) (*slog.Logger, *RotatingWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	w, err := NewRotatingWriter(dir)
	if err != nil {
		return nil, nil, err
	}
	handler := slog.NewJSONHandler(io.MultiWriter(w, os.Stderr), &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler), w, nil
}

func NewRotatingWriter(dir string) (*RotatingWriter, error) {
	w := &RotatingWriter{dir: dir}
	return w, w.open()
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size+int64(len(p)) > maxLogBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *RotatingWriter) open() error {
	path := filepath.Join(w.dir, "ignore.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *RotatingWriter) rotate() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	src := filepath.Join(w.dir, "ignore.log")
	dst := filepath.Join(w.dir, fmt.Sprintf("ignore-%s.log", time.Now().Format("20060102-150405")))
	if _, err := os.Stat(src); err == nil {
		_ = os.Rename(src, dst)
	}
	if err := w.prune(); err != nil {
		return err
	}
	return w.open()
}

func (w *RotatingWriter) prune() error {
	files, err := filepath.Glob(filepath.Join(w.dir, "ignore-*.log"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for len(files) > maxFiles-1 {
		_ = os.Remove(files[0])
		files = files[1:]
	}
	return nil
}

func Tail(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	b, err := io.ReadAll(f)
	return string(b), err
}
