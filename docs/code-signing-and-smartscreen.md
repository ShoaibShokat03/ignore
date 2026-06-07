# Code Signing and SmartScreen

Windows marks files downloaded from the internet with a zone marker. When a user runs a downloaded installer, Microsoft Defender SmartScreen checks the app reputation, the file hash reputation, and the digital signature. An unsigned installer can show "Windows protected your PC" or be blocked by stricter enterprise policies even when the file is clean.

## Current Release Gate

Use this command before uploading a build:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\verify-release.ps1
```

For a public release, require a valid Authenticode signature:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\verify-release.ps1 -RequireValidSignature
```

The script reports:

- Authenticode signature status
- Publisher certificate subject when present
- SHA256 hash for publishing on the download page
- Microsoft Defender local scan result when `MpCmdRun.exe` is available

## Signing Setup

`Build.bat` signs `build\bin\Ignore.exe` and `build\bin\Ignore-Setup.exe` when a signing certificate is configured. Configure one of these options before running the build.

PFX certificate:

```bat
setx IGNORE_SIGN_PFX "C:\path\to\certificate.pfx"
setx IGNORE_SIGN_PASSWORD "pfx-password"
```

Certificate already installed in the Windows certificate store:

```bat
setx IGNORE_SIGN_THUMBPRINT "SHA1_CERTIFICATE_THUMBPRINT"
```

Optional timestamp server:

```bat
setx IGNORE_SIGN_TIMESTAMP "http://timestamp.digicert.com"
```

After setting environment variables, open a new terminal and run:

```bat
Build.bat
```

## What This Fixes

A trusted code-signing certificate removes "Unknown publisher" and proves the installer was not modified after signing. Timestamping keeps the signature valid after the certificate expires.

## What It Cannot Fully Fix

SmartScreen reputation is not the same as antivirus detection. Microsoft evaluates both publisher reputation and the reputation of the exact file hash. A brand-new signed build may still show an "unrecognized app" warning until reputation accumulates. Publishing through the Microsoft Store is the cleanest way to avoid download warnings because Store apps are distributed with Microsoft's trust chain.

Self-signed certificates do not solve public SmartScreen warnings. They only help on machines that explicitly trust that private certificate.

## Recommended Release Process

1. Build with `Build.bat`.
2. Confirm both `Ignore.exe` and `Ignore-Setup.exe` are signed.
3. Run `scripts\verify-release.ps1 -RequireValidSignature`.
4. Upload only `Ignore-Setup.exe` for users, not the loose app executable.
5. Publish the SHA256 hash beside the download link.
6. Keep the same signing identity across releases.
7. Use HTTPS downloads from your own domain.
8. If Microsoft Defender reports malware, submit the installer to Microsoft Security Intelligence for review.

## Distribution Options

- Best user trust: Microsoft Store.
- Good non-Store path: Azure Artifact Signing, formerly Trusted Signing, or a standard OV/EV code-signing certificate.
- Development only: unsigned local builds.
