# Verifies the Windows release artifacts before they are uploaded.
#
# This script does not sign files. It checks the signature state, prints SHA256
# hashes for website/release notes, and optionally runs Microsoft Defender's
# command-line scanner when it is available on the machine.

param(
  [string[]]$Path = @('build\bin\Ignore.exe', 'build\bin\Ignore-Setup.exe'),
  [switch]$RequireValidSignature,
  [switch]$SkipDefenderScan
)

$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
$failed = $false

function Resolve-ReleasePath {
  param([string]$Candidate)
  if ([System.IO.Path]::IsPathRooted($Candidate)) {
    return $Candidate
  }
  return Join-Path $root $Candidate
}

Write-Host 'Ignore release verification'
Write-Host '==========================='

foreach ($candidate in $Path) {
  $file = Resolve-ReleasePath $candidate
  Write-Host ''
  Write-Host "Artifact: $file"

  if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
    Write-Host '  status: missing'
    $failed = $true
    continue
  }

  $hash = Get-FileHash -LiteralPath $file -Algorithm SHA256
  Write-Host "  sha256: $($hash.Hash)"

  $signature = Get-AuthenticodeSignature -LiteralPath $file
  Write-Host "  signature: $($signature.Status)"
  if ($signature.SignerCertificate) {
    Write-Host "  publisher: $($signature.SignerCertificate.Subject)"
  } else {
    Write-Host '  publisher: unsigned'
  }

  if ($RequireValidSignature -and $signature.Status -ne 'Valid') {
    Write-Host '  release gate: failed because the signature is not valid'
    $failed = $true
  } elseif ($signature.Status -ne 'Valid') {
    Write-Host '  warning: unsigned downloads commonly trigger SmartScreen reputation blocks'
  }
}

if (-not $SkipDefenderScan) {
  $mpcmd = Join-Path $env:ProgramFiles 'Windows Defender\MpCmdRun.exe'
  if (-not (Test-Path -LiteralPath $mpcmd)) {
    $mpcmd = Join-Path ${env:ProgramFiles(x86)} 'Windows Defender\MpCmdRun.exe'
  }

  if (Test-Path -LiteralPath $mpcmd) {
    foreach ($candidate in $Path) {
      $file = Resolve-ReleasePath $candidate
      if (Test-Path -LiteralPath $file -PathType Leaf) {
        Write-Host ''
        Write-Host "Microsoft Defender scan: $file"
        & $mpcmd -Scan -ScanType 3 -File $file
        if ($LASTEXITCODE -ne 0) {
          Write-Host "  Defender scan exited with code $LASTEXITCODE"
          $failed = $true
        }
      }
    }
  } else {
    Write-Host ''
    Write-Host 'Microsoft Defender command-line scanner was not found; skipped local AV scan.'
  }
}

if ($failed) {
  exit 1
}

exit 0
