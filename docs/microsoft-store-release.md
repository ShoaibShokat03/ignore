# Microsoft Store Release Plan

Ignore should use the **MSIX package upload** route for Microsoft Store distribution. This gives the best user trust path because Store-distributed packages are hosted and signed through Microsoft after certification.

Avoid the Store "MSI or EXE app" path for the first release unless you already have a trusted code-signing certificate. That path can still require a signed installer and a stable HTTPS installer URL.

## Why MSIX

- Microsoft Store can sign and host the package after submission.
- Users install through the Store instead of downloading an unsigned website installer.
- Store updates, flighting, and install identity are available.
- The app gets a clearer trust story than an unsigned `Ignore-Setup.exe`.

## Required Accounts and Tools

- Microsoft Partner Center developer account.
- Reserved Microsoft Store app name: `Ignore`.
- Visual Studio 2022 with the **Universal Windows Platform development** workload, or MSIX packaging tools.
- Windows SDK.
- Built app executable from `Build.bat` or `wails build`.
- Store artwork generated from `assets\ignore_logo_transparent.png`.

## Recommended Packaging Flow

1. Run `Build.bat` and confirm the app works locally.
2. Create a Windows Application Packaging Project in Visual Studio.
3. Add the built Wails executable and required runtime files from `build\bin`.
4. Configure the package identity from Partner Center.
5. Add Store logos and tile assets generated from the Ignore logo.
6. Create an `.msixupload` package for Microsoft Store submission.
7. Install/test the package locally before submission.
8. Submit the `.msixupload` in Partner Center.

## Store Listing Checklist

- App name: `Ignore`
- Category: Developer tools or Productivity
- Short description: Background developer transfer cleaner using `.ignore` rules.
- Privacy policy URL.
- Support/contact URL.
- Screenshots of Ignore, Status, and About pages.
- Explanation that Ignore monitors Explorer clipboard file lists locally to prepare filtered copies.
- Clear note that no project files are uploaded by Ignore.

## Privacy Notes

Ignore reads file paths from the Windows clipboard when file-copy protection is enabled. It uses those paths locally to create filtered staging copies. The app does not send clipboard contents, file paths, project files, logs, or `.ignore` rules to any remote service.

Keep this statement visible in the Store privacy policy and app description because clipboard-related behavior can look sensitive during review.

## Certification Risks to Watch

- Background/tray behavior must be user-controlled.
- Start with Windows must remain opt-in.
- Clipboard monitoring must be explained honestly.
- Logs must stay local and must not include secret file contents.
- The app must uninstall cleanly.
- The app should not claim perfect Explorer/upload interception; current Windows limitations are documented in `docs/windows-integration.md`.

## Website Downloads After Store Release

Once the Store version is available, link users to the Microsoft Store instead of offering an unsigned installer download. If you still want a direct website download, sign `Ignore-Setup.exe` with a trusted certificate and run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\verify-release.ps1 -RequireValidSignature
```
