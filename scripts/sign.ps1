# Authenticode code-signing helper for the Ignore app and installer.
#
# The "Windows protected your PC" / "unrecognized app" SmartScreen prompt and most
# antivirus "unknown publisher" flags appear because the .exe is UNSIGNED. The only
# real fix is to sign the binaries with a trusted code-signing certificate. This
# script signs a file IF a certificate is configured; otherwise it skips cleanly so
# unsigned development builds still succeed.
#
# Configure ONE of these (via environment variables) before running Build.bat:
#   PFX file:
#     setx IGNORE_SIGN_PFX        "C:\path\to\cert.pfx"
#     setx IGNORE_SIGN_PASSWORD   "pfx-password"        (optional)
#   OR a certificate already installed in the Windows cert store:
#     setx IGNORE_SIGN_THUMBPRINT "AB12CD...the SHA1 thumbprint..."
#   Optional timestamp server (default: DigiCert):
#     setx IGNORE_SIGN_TIMESTAMP  "http://timestamp.digicert.com"
#
# Notes:
#   * A trusted OV/EV/Azure Artifact Signing certificate removes the "unknown
#     publisher" text and builds SmartScreen reputation over time.
#   * Microsoft no longer treats a new EV-signed binary as an instant SmartScreen
#     bypass; every new file hash still needs reputation unless it is distributed
#     through the Microsoft Store.
#   * Self-signed certs do NOT satisfy SmartScreen; they only help on machines that
#     explicitly trust that cert.

param([Parameter(Mandatory = $true)][string]$Path)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path $Path)) {
  Write-Host "sign: file not found, skipping: $Path"
  exit 0
}

$pfx = $env:IGNORE_SIGN_PFX
$thumb = $env:IGNORE_SIGN_THUMBPRINT

if (-not $pfx -and -not $thumb) {
  Write-Host "sign: no signing certificate configured - skipping '$Path'."
  Write-Host "sign: (set IGNORE_SIGN_PFX or IGNORE_SIGN_THUMBPRINT to sign and remove the SmartScreen 'unrecognized app' prompt.)"
  exit 0
}

# Locate signtool.exe (PATH first, then newest Windows SDK).
$signtool = (Get-Command signtool.exe -ErrorAction SilentlyContinue).Source
if (-not $signtool) {
  $cands = Get-ChildItem 'C:\Program Files (x86)\Windows Kits\10\bin\*\x64\signtool.exe' -ErrorAction SilentlyContinue |
    Sort-Object FullName -Descending
  if ($cands) { $signtool = $cands[0].FullName }
}
if (-not $signtool) {
  Write-Host "sign: signtool.exe not found (install the Windows 10/11 SDK). Skipping '$Path'."
  exit 0
}

$ts = $env:IGNORE_SIGN_TIMESTAMP
if (-not $ts) { $ts = 'http://timestamp.digicert.com' }

$sargs = @('sign', '/fd', 'SHA256', '/tr', $ts, '/td', 'SHA256')
if ($thumb) {
  $sargs += @('/sha1', $thumb)
} else {
  $sargs += @('/f', $pfx)
  if ($env:IGNORE_SIGN_PASSWORD) { $sargs += @('/p', $env:IGNORE_SIGN_PASSWORD) }
}
$sargs += $Path

Write-Host "sign: signing $Path"
& $signtool @sargs
if ($LASTEXITCODE -ne 0) {
  Write-Error "sign: signtool failed (exit $LASTEXITCODE) for $Path"
  exit 1
}
Write-Host "sign: signed OK -> $Path"
exit 0
