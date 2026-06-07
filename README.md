# Ignore

Ignore is a lightweight Windows desktop utility for developer transfers. It keeps a fast `.ignore` rule engine running in the background and provides a high-performance filtered copy engine that skips unwanted folders and files such as `node_modules`, `vendor`, `.next`, `build`, `.git`, logs, caches, and local environment files.

The app is built with Go and Wails, uses a minimal React UI, lives in the Windows tray, stores configuration locally, writes rotating logs, and is structured so native Explorer shell-extension support can be added without changing rule parsing or copy behavior.

## Current Capabilities

- Wails desktop app with React UI
- Native Windows tray icon via `Shell_NotifyIcon`
- Global ignore editor for `%USERPROFILE%\.ignore`
- Enable/disable protection state
- Start with Windows registration through `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
- Explorer clipboard fallback for normal copy/paste file lists
- Rule reload with cache invalidation
- Project-level `.ignore` support
- High-performance filtered copy engine using `filepath.WalkDir`, bounded workers, streaming I/O, buffer reuse, context cancellation, timestamp preservation, and graceful per-file errors
- Metrics for copied files, skipped files, skipped directories, bytes, errors, operations, duration, and last activity
- Rotating JSON logs
- Installer templates for Inno Setup and NSIS

## Honest Windows Integration Status

Windows does not provide a safe pure-Go hook that can transparently rewrite every Explorer Ctrl+C, Ctrl+X, Ctrl+V, drag-drop, folder transfer, and browser upload operation. Full interception requires a native Explorer shell extension, copy hook handler, drag-drop handler, cloud provider integration, or browser extension depending on the workflow.

Ignore implements the production-ready core that those integrations need: rule parsing, caching, metrics, logging, tray control, startup integration, and a filtered copy engine. It also includes a Windows clipboard fallback: after you copy files or folders in Explorer, Ignore creates a filtered staging copy and replaces the clipboard with the cleaned staged paths. See [Windows integration notes](docs/windows-integration.md) for details and the roadmap.

## Rule Files

Global rules live at:

```txt
%USERPROFILE%\.ignore
```

Project rules live at:

```txt
<ProjectRoot>\.ignore
```

Only lines after `[IGNORE]` are active. Empty lines and `#` comments are ignored. Rules are case-insensitive on Windows. Project rules extend global rules. A project rule prefixed with `!` overrides a previous ignore rule.

```txt
# ignored until the marker

[IGNORE]

node_modules
vendor
dist
build
.next
.git

.env
.env.local

*.log
*.tmp
*.cache
*.bak

# project-level override example
!keep.log
```

## Build

Prerequisites:

- Go 1.24 or newer
- Node.js 20 or newer
- Wails CLI v2

Install Wails if needed:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Build the frontend:

```powershell
npm install --prefix ui
npm run build --prefix ui
```

Run tests:

```powershell
go test ./...
```

Build the app:

```powershell
wails build
```

For a plain Go compile check:

```powershell
go build ./cmd/ignore
```

To build the branded Windows installer, run:

```bat
Build.bat
```

Before uploading the installer to a website, verify the release artifacts:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\verify-release.ps1
```

Public website downloads should be signed with a trusted code-signing certificate. See [Code signing and SmartScreen](docs/code-signing-and-smartscreen.md).

For Microsoft Store distribution, use the MSIX package upload route. See [Microsoft Store release plan](docs/microsoft-store-release.md).

## Installer

After `wails build`, point either installer template at the generated executable:

- [installer/ignore.iss](installer/ignore.iss)
- [installer/ignore.nsi](installer/ignore.nsi)

Both templates install the executable, create Start Menu entries, and optionally launch Ignore after installation. Startup-at-login is controlled by the app setting, not forced by the installer.

## Documentation

- [Architecture](docs/architecture.md)
- [Performance notes](docs/performance.md)
- [Windows integration notes](docs/windows-integration.md)
- [Code signing and SmartScreen](docs/code-signing-and-smartscreen.md)
- [Microsoft Store release plan](docs/microsoft-store-release.md)
- [Privacy policy](docs/privacy-policy.md)
- [Example ignore files](examples/)
