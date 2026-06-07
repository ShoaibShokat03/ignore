# Architecture

Ignore is organized around a small core and replaceable integration edges.

```txt
/cmd/ignore          Wails entry point and embedded assets
/internal/app        Application service and Wails bindings
/internal/config     Local JSON configuration and default global .ignore creation
/internal/ignore     Rule parser, matcher, global/project rule cache
/internal/copyengine High-performance filtered copy engine
/internal/tray       Windows tray controller
/internal/clipboard  Clipboard awareness boundary
/internal/watcher    Debounced .ignore filesystem watcher
/internal/winapi     Windows-specific startup, path, and shell helpers
/internal/logging    Rotating JSON logs
/internal/metrics    Thread-safe counters and activity snapshot
/internal/ui         Wails app runner
/ui                  React frontend
/installer           Installer templates
```

## Core Flow

1. `cmd/ignore` starts the Wails app and embeds the built React UI.
2. `internal/app` creates config, logging, metrics, rule set, copy engine, watcher, clipboard monitor, and tray controller.
3. The rule set loads `%USERPROFILE%\.ignore`, then merges the closest project `.ignore` found by walking upward from a copied path.
4. The copy engine walks source directories with `filepath.WalkDir`.
5. Ignored directories return `filepath.SkipDir` before traversal enters them.
6. Non-ignored files are streamed by bounded worker goroutines.
7. Metrics and rotating logs are updated without blocking the UI.

## Rule Cache

Global rule reload increments a generation counter and clears directory caches. Project `.ignore` entries store the project file modification time, so a changed project file invalidates cached rules automatically on the next match.

Project rules extend global rules. Later matches win, and `!pattern` unignores a previous global or project ignore rule.

## Copy Engine

The engine is intentionally not tied to Explorer. It accepts source and destination paths, a context, and the active rule set. This lets future shell extensions or upload staging integrations call the same engine rather than reimplement filtering.

Key implementation details:

- Bounded worker pool
- Buffered streaming copy
- `sync.Pool` for reusable buffers
- Long Windows path normalization
- Per-file error handling
- Timestamp and permission preservation where Windows allows it
- Context cancellation support

## UI Boundary

The React UI only manages settings, the global ignore editor, status, metrics, and logs. It does not provide a custom copy/paste workflow because the product goal is normal Explorer workflows. Transparent Explorer integration belongs in the native shell-extension layer described in the roadmap.

