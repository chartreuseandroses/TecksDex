<#
Builds webapp.exe and zips it up with the SDE database into a single
shareable archive that someone else can unzip and run as-is.
#>
param(
    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

$repoRoot = $PSScriptRoot
$webappDir = Join-Path $repoRoot "webapp"
$sdeDb = Join-Path $repoRoot "sde\eve_sde.db"
$distDir = Join-Path $repoRoot $OutputDir
$stageDir = Join-Path $distDir "pi-profitability"
$zipPath = Join-Path $distDir "pi-profitability.zip"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "go is not on PATH in this shell. Open a new terminal (Go's install added it to your PATH) and try again."
}

if (-not (Test-Path $sdeDb)) {
    throw "SDE database not found at $sdeDb"
}

$goBin = Join-Path (go env GOPATH) "bin"
if (($env:Path -split ";") -notcontains $goBin) { $env:Path += ";$goBin" }

if (-not (Get-Command govulncheck -ErrorAction SilentlyContinue)) {
    Write-Host "Installing govulncheck..."
    go install golang.org/x/vuln/cmd/govulncheck@latest
    if ($LASTEXITCODE -ne 0) { throw "Failed to install govulncheck." }
}

Write-Host "Running tests..."
Push-Location $webappDir
try {
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "Tests failed, aborting package build." }

    Write-Host "Building webapp.exe..."
    go build -o webapp.exe .
    if ($LASTEXITCODE -ne 0) { throw "Build failed, aborting package build." }

    Write-Host "Scanning webapp.exe for known vulnerabilities..."
    govulncheck -mode=binary webapp.exe
    if ($LASTEXITCODE -ne 0) { throw "govulncheck found vulnerabilities, aborting package build." }
} finally {
    Pop-Location
}

if (Test-Path $stageDir) { Remove-Item $stageDir -Recurse -Force }
New-Item -ItemType Directory -Path (Join-Path $stageDir "sde") -Force | Out-Null

Copy-Item (Join-Path $webappDir "webapp.exe") $stageDir
Copy-Item $sdeDb (Join-Path $stageDir "sde")

if (Test-Path $zipPath) { Remove-Item $zipPath -Force }
Compress-Archive -Path (Join-Path $stageDir "*") -DestinationPath $zipPath

Remove-Item $stageDir -Recurse -Force

Write-Host "Created $zipPath"
