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
	"strconv"
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

func (a *Ignore) prepareClipboardFiles(ctx context.Context, paths []string) ([]string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(cacheDir, "Ignore", "clipboard-staging")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	a.pruneClipboardStaging(root)
	stage := filepath.Join(root, time.Now().Format("20060102-150405.000000000"))
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(paths))
	usedNames := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if ctx.Err() != nil {
			return filtered, ctx.Err()
		}
		info, err := os.Stat(path)
		if err != nil {
			a.logger.Warn("clipboard path stat failed", "path", path, "error", err)
			continue
		}
		decision := a.rules.Match(path, info.IsDir())
		if decision.Ignored {
			if info.IsDir() {
				a.metrics.AddSkippedDir()
			} else {
				a.metrics.AddSkippedFile()
			}
			a.logger.Info("clipboard item ignored", "path", path, "rule", decision.Rule)
			continue
		}
		targetName := windowsCopyName(filepath.Dir(path), filepath.Base(path), usedNames)
		usedNames[strings.ToLower(targetName)] = struct{}{}
		target := filepath.Join(stage, targetName)
		result, err := a.engine.CopyFiltered(ctx, path, target)
		if err != nil {
			a.logger.Warn("clipboard staging copy failed", "path", path, "target", target, "error", err)
			continue
		}
		if result.FilesCopied == 0 && !info.IsDir() {
			continue
		}
		filtered = append(filtered, target)
	}
	if len(filtered) == 0 {
		_ = os.RemoveAll(stage)
		return nil, nil
	}
	a.logger.Info("clipboard staging prepared", "inputItems", len(paths), "outputItems", len(filtered), "stage", stage)
	return filtered, nil
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

func uniqueStagePath(dir, name string) string {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for i := 2; ; i++ {
		candidate := filepath.Join(dir, base+"-"+strconv.Itoa(i)+ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func windowsCopyName(originalDir, name string, used map[string]struct{}) string {
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for i := 1; ; i++ {
		candidate := ""
		if i == 1 {
			candidate = base + " - Copy" + ext
		} else {
			candidate = base + " - Copy (" + strconv.Itoa(i) + ")" + ext
		}
		key := strings.ToLower(candidate)
		if _, ok := used[key]; ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(originalDir, candidate)); os.IsNotExist(err) {
			return candidate
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
