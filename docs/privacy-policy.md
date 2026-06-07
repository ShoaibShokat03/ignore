# Ignore Privacy Policy

Effective date: 2026-06-07

Ignore is a local Windows desktop utility for developer file transfers. It applies `.ignore` rules to copied project folders so unwanted files such as dependencies, build output, logs, and environment files can be skipped.

## Data Processed Locally

Ignore may process:

- File and folder paths copied through Windows Explorer.
- Local `.ignore` rule files.
- Local app settings.
- Local activity metrics such as copied/skipped counts and last activity.
- Local rotating logs for troubleshooting.

## Clipboard Access

When protection is enabled, Ignore watches Windows clipboard file-list changes. If copied paths match the configured rules, Ignore prepares a filtered local staging copy and replaces the clipboard file list with the cleaned paths.

Ignore does not read general text clipboard content for remote processing, and it does not upload clipboard data.

## Network Use

Ignore does not send project files, file paths, `.ignore` rules, logs, metrics, or clipboard contents to any server.

## Storage

Ignore stores settings and logs on the local Windows device. The global ignore file is stored at:

```txt
%USERPROFILE%\.ignore
```

Project ignore files are stored inside project folders:

```txt
<ProjectRoot>\.ignore
```

## User Control

Users can enable or disable protection from the app UI or tray menu. Start with Windows is opt-in and can be disabled from the app UI.

## Contact

GitHub: https://github.com/ShoaibShokat03

LinkedIn: https://www.linkedin.com/in/muhammad-shoaib-776521204
