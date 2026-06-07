package app

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ignore/internal/clipboard"
	"ignore/internal/config"
	"ignore/internal/copyengine"
	"ignore/internal/ignore"
	"ignore/internal/logging"
	"ignore/internal/metrics"
	"ignore/internal/tray"
	"ignore/internal/watcher"
	"ignore/internal/winapi"
)

type Ignore struct {
	ctx       context.Context
	assets    fs.FS
	store     *config.Store
	rules     *ignore.RuleSet
	engine    *copyengine.Engine
	metrics   *metrics.Metrics
	logger    *slog.Logger
	logWriter *logging.RotatingWriter
	watcher   *watcher.Watcher
	clip      *clipboard.Monitor
	tray      tray.Controller
	cancel    context.CancelFunc
	mu        sync.RWMutex
	enabled   bool
}

type State struct {
	Config  config.Config    `json:"config"`
	Metrics metrics.Snapshot `json:"metrics"`
	Enabled bool             `json:"enabled"`
	Status  string           `json:"status"`
}

func New(assets embed.FS) (*Ignore, error) {
	assetRoot, err := resolveAssets(assets)
	if err != nil {
		return nil, err
	}
	store, err := config.NewStore()
	if err != nil {
		return nil, err
	}
	cfg := store.Get()
	logger, writer, err := logging.NewLogger(cfg.LogDir)
	if err != nil {
		return nil, err
	}
	m := metrics.New()
	rules := ignore.NewRuleSet(cfg.GlobalIgnorePath)
	if err := rules.Reload(); err != nil {
		logger.Warn("initial rule reload failed", "error", err)
	}
	engine := copyengine.New(rules, m, logger, cfg.WorkerCount, cfg.BufferSize)
	return &Ignore{
		assets:    assetRoot,
		store:     store,
		rules:     rules,
		engine:    engine,
		metrics:   m,
		logger:    logger,
		logWriter: writer,
		enabled:   cfg.Enabled,
	}, nil
}

func (a *Ignore) Assets() fs.FS {
	return a.assets
}

// resolveAssets locates the embedded UI build. fs.Sub never fails for a
// syntactically valid path, so we must confirm index.html is actually present
// before accepting a candidate; otherwise Wails serves an empty filesystem and
// crashes at startup with "open .: file does not exist".
func resolveAssets(assets embed.FS) (fs.FS, error) {
	for _, dir := range []string{"ui/dist", "cmd/ignore/ui/dist"} {
		sub, err := fs.Sub(assets, dir)
		if err != nil {
			continue
		}
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			return sub, nil
		}
	}
	return nil, errors.New("embedded UI assets not found (no index.html in ui/dist or cmd/ignore/ui/dist)")
}

func (a *Ignore) Startup(ctx context.Context) {
	a.ctx = ctx
	runCtx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.logger.Info("application startup")

	cfg := a.store.Get()
	a.watcher = watcher.New(a.logger, 300*time.Millisecond, func(path string) {
		if err := a.rules.Reload(); err != nil {
			a.logger.Warn("rule reload failed", "path", path, "error", err)
			return
		}
		a.emit("rules:reloaded", map[string]any{"path": path})
		a.logger.Info("rules reloaded", "path", path)
	})
	_ = a.watcher.WatchFile(cfg.GlobalIgnorePath)
	go a.watcher.Run(runCtx)

	a.clip = clipboard.New(a.logger, 500*time.Millisecond, a.isEnabled, a.prepareClipboardFiles, func(event clipboard.Event) {
		a.metrics.CompleteOperation(event.Message, 0)
		a.emit("activity", event)
	})
	go a.clip.Run(runCtx)

	a.tray = tray.New(a.logger, tray.Actions{
		Open:    func() { runtime.WindowShow(a.ctx) },
		Enable:  func() { _ = a.SetEnabled(true) },
		Disable: func() { _ = a.SetEnabled(false) },
		Reload:  func() { _ = a.ReloadRules() },
		Logs:    func() { _ = a.OpenLogs() },
		Exit:    func() { runtime.Quit(a.ctx) },
	})
	go a.tray.Run(runCtx)
}

// ShowWindow brings the main window to the foreground. The single-instance
// handler calls it so launching the app again reveals the already-running
// instance instead of starting a second clipboard monitor.
func (a *Ignore) ShowWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

func (a *Ignore) Shutdown(ctx context.Context) {
	a.logger.Info("application shutdown")
	if a.cancel != nil {
		a.cancel()
	}
	if a.tray != nil {
		a.tray.Stop()
	}
	if a.logWriter != nil {
		_ = a.logWriter.Close()
	}
}

func (a *Ignore) GetState() State {
	cfg := a.store.Get()
	a.mu.RLock()
	enabled := a.enabled
	a.mu.RUnlock()
	status := "Protection enabled"
	if !enabled {
		status = "Protection disabled"
	}
	return State{Config: cfg, Metrics: a.metrics.Snapshot(), Enabled: enabled, Status: status}
}

func (a *Ignore) isEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.enabled
}

func (a *Ignore) prepareClipboardFiles(ctx context.Context, files clipboard.FileList) (clipboard.PreparedFileList, error) {
	paths := files.Paths
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return clipboard.PreparedFileList{}, err
	}
	root := filepath.Join(cacheDir, "Ignore", "clipboard-staging")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return clipboard.PreparedFileList{}, err
	}
	a.pruneClipboardStaging(root)
	stage := filepath.Join(root, time.Now().Format("20060102-150405.000000000"))
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return clipboard.PreparedFileList{}, err
	}
	filtered := make([]string, 0, len(paths))
	cutSources := make([]struct {
		path  string
		isDir bool
	}, 0, len(paths))
	for i, path := range paths {
		if ctx.Err() != nil {
			return clipboard.PreparedFileList{Paths: filtered}, ctx.Err()
		}
		info, err := os.Stat(path)
		if err != nil {
			a.logger.Warn("clipboard path stat failed", "path", path, "error", err)
			continue
		}
		needsFilter, ignoredRoot, err := a.clipboardPathNeedsFiltering(path, info.IsDir())
		if err != nil {
			a.logger.Warn("clipboard filter scan failed", "path", path, "error", err)
		}
		if ignoredRoot {
			a.addIgnoredMetric(info.IsDir())
			continue
		}
		if !needsFilter {
			filtered = append(filtered, path)
			continue
		}
		itemStage := filepath.Join(stage, fmt.Sprintf("%04d", i))
		target := filepath.Join(itemStage, filepath.Base(path))
		result, err := a.engine.CopyFiltered(ctx, path, target)
		if err != nil {
			a.logger.Warn("clipboard staging copy failed", "path", path, "target", target, "error", err)
			continue
		}
		if result.FilesCopied == 0 && !info.IsDir() {
			continue
		}
		filtered = append(filtered, target)
		if files.DropEffect == clipboard.DropEffectMove {
			cutSources = append(cutSources, struct {
				path  string
				isDir bool
			}{path: path, isDir: info.IsDir()})
		}
	}
	if len(filtered) == 0 {
		_ = os.RemoveAll(stage)
		return clipboard.PreparedFileList{}, nil
	}
	if sameStringSet(paths, filtered) {
		_ = os.RemoveAll(stage)
		return clipboard.PreparedFileList{Paths: paths}, nil
	}
	a.logger.Info("clipboard staging prepared", "inputItems", len(paths), "outputItems", len(filtered), "stage", stage)
	prepared := clipboard.PreparedFileList{Paths: filtered}
	if len(cutSources) > 0 {
		prepared.AfterMovePaste = func() {
			for _, source := range cutSources {
				if err := a.deleteFilteredSource(source.path, source.isDir); err != nil {
					a.logger.Warn("clipboard cut source cleanup failed", "path", source.path, "error", err)
				}
			}
		}
	}
	return prepared, nil
}

func (a *Ignore) clipboardPathNeedsFiltering(path string, isDir bool) (needsFilter bool, ignoredRoot bool, err error) {
	decision := a.rules.Match(path, isDir)
	if decision.Ignored {
		a.logger.Info("clipboard item ignored", "path", path, "rule", decision.Rule)
		return true, true, nil
	}
	if !isDir {
		return false, false, nil
	}
	walkErr := filepath.WalkDir(path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == path {
			return nil
		}
		decision := a.rules.Match(current, d.IsDir())
		if !decision.Ignored {
			return nil
		}
		if d.IsDir() {
			a.logger.Info("clipboard directory will be filtered", "path", current, "rule", decision.Rule)
			return errClipboardFilteringNeeded
		}
		a.logger.Info("clipboard file will be filtered", "path", current, "rule", decision.Rule)
		return errClipboardFilteringNeeded
	})
	if errors.Is(walkErr, errClipboardFilteringNeeded) {
		return true, false, nil
	}
	if walkErr != nil {
		return true, false, walkErr
	}
	return false, false, nil
}

var errClipboardFilteringNeeded = errors.New("clipboard filtering needed")

func (a *Ignore) addIgnoredMetric(isDir bool) {
	if isDir {
		a.metrics.AddSkippedDir()
	} else {
		a.metrics.AddSkippedFile()
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (a *Ignore) deleteFilteredSource(path string, isDir bool) error {
	if !isDir {
		decision := a.rules.Match(path, false)
		if decision.Ignored {
			return nil
		}
		return os.Remove(path)
	}
	var dirs []string
	err := filepath.WalkDir(path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			a.logger.Warn("cut cleanup walk failed", "path", current, "error", walkErr)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if current == path {
			dirs = append(dirs, current)
			return nil
		}
		decision := a.rules.Match(current, d.IsDir())
		if decision.Ignored {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			dirs = append(dirs, current)
			return nil
		}
		if err := os.Remove(current); err != nil && !errors.Is(err, os.ErrNotExist) {
			a.logger.Warn("cut cleanup file remove failed", "path", current, "error", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Remove(dirs[i]); err != nil && !errors.Is(err, os.ErrNotExist) {
			a.logger.Debug("cut cleanup directory kept", "path", dirs[i], "error", err)
		}
	}
	return nil
}

func (a *Ignore) pruneClipboardStaging(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func (a *Ignore) ReadGlobalIgnore() (string, error) {
	b, err := os.ReadFile(a.store.Get().GlobalIgnorePath)
	return string(b), err
}

func (a *Ignore) SaveGlobalIgnore(content string) error {
	path := a.store.Get().GlobalIgnorePath
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	return a.ReloadRules()
}

func (a *Ignore) ReloadRules() error {
	if err := a.rules.Reload(); err != nil {
		return err
	}
	a.logger.Info("rules reloaded")
	a.emit("rules:reloaded", map[string]any{"path": a.store.Get().GlobalIgnorePath})
	return nil
}

func (a *Ignore) SetEnabled(enabled bool) error {
	a.mu.Lock()
	a.enabled = enabled
	a.mu.Unlock()
	if err := a.store.Update(func(c *config.Config) { c.Enabled = enabled }); err != nil {
		return err
	}
	a.logger.Info("protection state changed", "enabled", enabled)
	a.emit("state:changed", a.GetState())
	return nil
}

func (a *Ignore) SetStartWithWindows(enabled bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := winapi.SetStartup("Ignore", exe, enabled); err != nil {
		return err
	}
	return a.store.Update(func(c *config.Config) { c.StartWithWindows = enabled })
}

func (a *Ignore) CopyFiltered(src, dst string) (copyengine.Result, error) {
	a.mu.RLock()
	enabled := a.enabled
	a.mu.RUnlock()
	if !enabled {
		return copyengine.Result{}, errors.New("protection is disabled")
	}
	return a.engine.CopyFiltered(context.Background(), src, dst)
}

func (a *Ignore) GetLogs() (string, error) {
	return logging.Tail(filepath.Join(a.store.Get().LogDir, "ignore.log"), 256*1024)
}

func (a *Ignore) OpenLogs() error {
	return winapi.OpenPath(a.store.Get().LogDir)
}

func (a *Ignore) Quit() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// ----- Feedback -----------------------------------------------------------

const (
	ntfyFeedbackURL    = "https://ntfy.sh/ignore-feadbacks"
	feedbackMaxChars   = 300
	feedbackDailyLimit = 3
)

// FeedbackStatus is returned to the UI so it can enable/disable the form.
type FeedbackStatus struct {
	CanSend     bool   `json:"canSend"`
	SentToday   int    `json:"sentToday"`
	Remaining   int    `json:"remaining"`
	Limit       int    `json:"limit"`
	Sent        int    `json:"sent"`
	LastSent    string `json:"lastSent,omitempty"`
	NextAllowed string `json:"nextAllowed,omitempty"`
}

// FeedbackItem is one stored message returned to the UI for the history list.
type FeedbackItem struct {
	Time      string `json:"time"`
	Message   string `json:"message"`
	Delivered bool   `json:"delivered"`
}

type feedbackEntry struct {
	Time      time.Time `json:"time"`
	Message   string    `json:"message"`
	Delivered bool      `json:"delivered"`
	Machine   string    `json:"machine,omitempty"`
	User      string    `json:"user,omitempty"`
}

func feedbackFile() string {
	dir, err := config.DataDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "feedback.jsonl")
}

// readFeedbackEntries loads all stored feedback entries (oldest first).
func readFeedbackEntries() []feedbackEntry {
	f, err := os.Open(feedbackFile())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []feedbackEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e feedbackEntry
		if json.Unmarshal([]byte(line), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func countFeedbackToday(entries []feedbackEntry) int {
	now := time.Now()
	n := 0
	for _, e := range entries {
		if sameDay(e.Time, now) {
			n++
		}
	}
	return n
}

func feedbackStatusFrom(entries []feedbackEntry) FeedbackStatus {
	today := countFeedbackToday(entries)
	remaining := feedbackDailyLimit - today
	if remaining < 0 {
		remaining = 0
	}
	st := FeedbackStatus{
		CanSend:   today < feedbackDailyLimit,
		SentToday: today,
		Remaining: remaining,
		Limit:     feedbackDailyLimit,
		Sent:      len(entries),
	}
	var last time.Time
	for _, e := range entries {
		if e.Time.After(last) {
			last = e.Time
		}
	}
	if !last.IsZero() {
		st.LastSent = last.Format(time.RFC3339)
	}
	if !st.CanSend {
		n := time.Now()
		tomorrow := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()).Add(24 * time.Hour)
		st.NextAllowed = tomorrow.Format(time.RFC3339)
	}
	return st
}

// GetFeedbackStatus reports how many messages the user may still send today.
func (a *Ignore) GetFeedbackStatus() FeedbackStatus {
	return feedbackStatusFrom(readFeedbackEntries())
}

// GetFeedbacks returns the user's stored feedback, newest first (max 50).
func (a *Ignore) GetFeedbacks() []FeedbackItem {
	entries := readFeedbackEntries()
	items := make([]FeedbackItem, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		items = append(items, FeedbackItem{
			Time:      e.Time.Format(time.RFC3339),
			Message:   e.Message,
			Delivered: e.Delivered,
		})
		if len(items) >= 50 {
			break
		}
	}
	return items
}

// SubmitFeedback stores the message locally (one per calendar day) and forwards
// it to ntfy.sh so the developer is notified. The local file is the durable
// record; ntfy delivery is best-effort.
func (a *Ignore) SubmitFeedback(message string) (FeedbackStatus, error) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return a.GetFeedbackStatus(), errors.New("please enter a message before sending")
	}
	if len(msg) > feedbackMaxChars {
		msg = msg[:feedbackMaxChars]
	}
	if countFeedbackToday(readFeedbackEntries()) >= feedbackDailyLimit {
		return a.GetFeedbackStatus(), fmt.Errorf("you can send up to %d feedback messages per day — please try again tomorrow", feedbackDailyLimit)
	}

	host, _ := os.Hostname()
	user := os.Getenv("USERNAME")
	delivered := a.sendFeedbackToNtfy(msg, host, user)

	entry := feedbackEntry{Time: time.Now(), Message: msg, Delivered: delivered, Machine: host, User: user}
	if err := appendFeedback(entry); err != nil {
		a.logger.Warn("feedback store failed", "error", err)
		return a.GetFeedbackStatus(), errors.New("could not save your feedback locally")
	}
	a.logger.Info("feedback submitted", "delivered", delivered, "chars", len(msg))
	return a.GetFeedbackStatus(), nil
}

func appendFeedback(e feedbackEntry) error {
	path := feedbackFile()
	if path == "" {
		return errors.New("cannot resolve data directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func (a *Ignore) sendFeedbackToNtfy(message, host, user string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	title := "Ignore feedback"
	if host != "" {
		title += " from " + host
	}
	if user != "" {
		title += " (" + user + ")"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ntfyFeedbackURL, strings.NewReader(message))
	if err != nil {
		return false
	}
	req.Header.Set("Title", strings.ToValidUTF8(title, ""))
	req.Header.Set("Tags", "speech_balloon")
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.logger.Warn("feedback ntfy delivery failed", "error", err)
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	a.logger.Warn("feedback ntfy non-2xx", "status", resp.StatusCode)
	return false
}

func (a *Ignore) emit(name string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}
