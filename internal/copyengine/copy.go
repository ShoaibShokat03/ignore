package copyengine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"ignore/internal/ignore"
	"ignore/internal/metrics"
	"ignore/internal/winapi"
)

type Engine struct {
	rules   *ignore.RuleSet
	metrics *metrics.Metrics
	log     *slog.Logger
	workers int
	bufSize int
	pool    sync.Pool
}

type Result struct {
	FilesCopied     uint64        `json:"filesCopied"`
	FilesSkipped    uint64        `json:"filesSkipped"`
	DirsSkipped     uint64        `json:"dirsSkipped"`
	Errors          uint64        `json:"errors"`
	BytesCopied     uint64        `json:"bytesCopied"`
	Duration        time.Duration `json:"duration"`
	DurationSeconds float64       `json:"durationSeconds"`
}

type task struct {
	src  string
	dst  string
	info os.DirEntry
}

func New(rules *ignore.RuleSet, m *metrics.Metrics, log *slog.Logger, workers, bufSize int) *Engine {
	if workers <= 0 {
		workers = min(runtime.NumCPU(), 8)
	}
	if bufSize <= 0 {
		bufSize = 1024 * 1024
	}
	e := &Engine{rules: rules, metrics: m, log: log, workers: workers, bufSize: bufSize}
	e.pool.New = func() any { return make([]byte, e.bufSize) }
	return e
}

func (e *Engine) CopyFiltered(ctx context.Context, src, dst string) (Result, error) {
	start := time.Now()
	var result Result
	src, err := filepath.Abs(src)
	if err != nil {
		return result, err
	}
	dst, err = filepath.Abs(dst)
	if err != nil {
		return result, err
	}
	src = winapi.NormalizeLongPath(src)
	dst = winapi.NormalizeLongPath(dst)
	srcInfo, err := os.Stat(src)
	if err != nil {
		return result, err
	}
	if !srcInfo.IsDir() {
		decision := e.rules.Match(src, false)
		if decision.Ignored {
			result.FilesSkipped++
			e.metrics.AddSkippedFile()
			e.metrics.CompleteOperation("Filtered file skipped", time.Since(start))
			e.log.Info("ignored file", "path", src, "rule", decision.Rule)
			return result, nil
		}
		bytes, err := e.copyOne(ctx, src, dst)
		if err != nil {
			result.Errors++
			e.metrics.AddError()
			return result, err
		}
		result.FilesCopied = 1
		result.BytesCopied = uint64(bytes)
		result.Duration = time.Since(start)
		result.DurationSeconds = result.Duration.Seconds()
		e.metrics.AddCopied(uint64(bytes))
		e.metrics.CompleteOperation("Filtered file copied", result.Duration)
		e.log.Info("file copy operation complete", "src", src, "dst", dst, "bytes", result.BytesCopied, "duration", result.Duration.String())
		return result, nil
	}

	tasks := make(chan task, e.workers*4)
	errs := make(chan error, e.workers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < e.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range tasks {
				bytes, err := e.copyOne(ctx, t.src, t.dst)
				mu.Lock()
				if err != nil {
					result.Errors++
					e.metrics.AddError()
					e.log.Warn("copy failed", "src", t.src, "dst", t.dst, "error", err)
				} else {
					result.FilesCopied++
					result.BytesCopied += uint64(bytes)
					e.metrics.AddCopied(uint64(bytes))
				}
				mu.Unlock()
			}
		}()
	}

	walkErr := filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			mu.Lock()
			result.Errors++
			mu.Unlock()
			e.metrics.AddError()
			e.log.Warn("walk failed", "path", path, "error", walkErr)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == src {
			return os.MkdirAll(dst, 0o755)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		decision := e.rules.Match(path, d.IsDir())
		if decision.Ignored {
			if d.IsDir() {
				mu.Lock()
				result.DirsSkipped++
				mu.Unlock()
				e.metrics.AddSkippedDir()
				e.log.Info("ignored directory", "path", path, "rule", decision.Rule)
				return filepath.SkipDir
			}
			mu.Lock()
			result.FilesSkipped++
			mu.Unlock()
			e.metrics.AddSkippedFile()
			e.log.Info("ignored file", "path", path, "rule", decision.Rule)
			return nil
		}
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case tasks <- task{src: path, dst: target, info: d}:
			return nil
		}
	})
	close(tasks)
	wg.Wait()
	close(errs)

	if walkErr != nil && !errors.Is(walkErr, context.Canceled) {
		result.Errors++
		e.metrics.AddError()
	}
	result.Duration = time.Since(start)
	result.DurationSeconds = result.Duration.Seconds()
	e.metrics.CompleteOperation("Filtered copy completed", result.Duration)
	e.log.Info("copy operation complete", "src", src, "dst", dst, "files", result.FilesCopied, "skippedFiles", result.FilesSkipped, "skippedDirs", result.DirsSkipped, "bytes", result.BytesCopied, "errors", result.Errors, "duration", result.Duration.String())
	return result, walkErr
}

func (e *Engine) copyOne(ctx context.Context, src, dst string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	info, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return 0, err
	}
	buf := e.pool.Get().([]byte)
	n, copyErr := copyWithContext(ctx, out, in, buf)
	e.pool.Put(buf)
	closeErr := out.Close()
	if copyErr != nil {
		return n, copyErr
	}
	if closeErr != nil {
		return n, closeErr
	}
	_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	_ = os.Chmod(dst, info.Mode().Perm())
	return n, nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, buf []byte) (int64, error) {
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				break
			}
			return written, er
		}
	}
	return written, nil
}
