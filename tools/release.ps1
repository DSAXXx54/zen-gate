# Local release helper: build with version injection, then create a GitHub release.
# Usage:  powershell -File tools\release.ps1 -Version 1.2.0
param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)

$tag = "v$Version"
go build -trimpath -ldflags "-s -w -H=windowsgui -X zen-gate/internal/gateway.Version=$Version -X zen-gate/internal/update.Current=$Version" -o dist/zen-gate.exe ./cmd/zen-gate
if ($LASTEXITCODE -ne 0) { throw "build failed" }
go build -trimpath -ldflags "-s -w" -o dist/zenstats.exe ./cmd/zenstats
if ($LASTEXITCODE -ne 0) { throw "build failed" }

Copy-Item README.md dist/ -Force
Compress-Archive -Path dist/zen-gate.exe, dist/README.md -DestinationPath "dist/zen-gate-$Version.zip" -Force

gh release create $tag dist/zen-gate.exe dist/zenstats.exe "dist/zen-gate-$Version.zip" --title $tag --generate-notes --latest
Write-Host "release $tag published"
