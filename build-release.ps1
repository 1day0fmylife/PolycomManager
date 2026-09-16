$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$Version = if ($env:VERSION) { $env:VERSION } else { "dev" }

function Assert-FileNotEmpty([string]$Path, [string]$Message) {
    if (-not (Test-Path $Path -PathType Leaf)) { throw $Message }
    if ((Get-Item $Path).Length -eq 0) { throw $Message }
}

Write-Host "[1/4] Go modules"
go mod tidy
if ($LASTEXITCODE -ne 0) { throw "go mod tidy failed" }

Write-Host "[2/4] React/Vite"
Push-Location web
try {
    if (Test-Path package-lock.json) { npm ci } else { npm install }
    if ($LASTEXITCODE -ne 0) { throw "npm dependency installation failed" }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "Vite build failed" }
} finally {
    Pop-Location
}

Assert-FileNotEmpty "web/dist/index.html" "web/dist/index.html was not produced by Vite"

$Stage = "internal/webui/.dist.tmp"
$Embed = "internal/webui/dist"
if (Test-Path $Stage) { Remove-Item $Stage -Recurse -Force }
New-Item $Stage -ItemType Directory -Force | Out-Null
Copy-Item "web/dist/*" $Stage -Recurse -Force
Assert-FileNotEmpty "$Stage/index.html" "failed to stage embedded web UI"
if (Test-Path $Embed) { Remove-Item $Embed -Recurse -Force }
Move-Item $Stage $Embed
Assert-FileNotEmpty "$Embed/index.html" "embedded web UI is missing after staging"

Write-Host "[3/4] Tests"
go test ./...
if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }

Write-Host "[4/4] Linux + Windows binaries"
New-Item dist -ItemType Directory -Force | Out-Null
$OldCGO = $env:CGO_ENABLED
$OldGOOS = $env:GOOS
$OldGOARCH = $env:GOARCH
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o dist/polycom-manager-linux-amd64 ./cmd/polycom-manager
    if ($LASTEXITCODE -ne 0) { throw "Linux build failed" }

    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o dist/polycom-manager-windows-amd64.exe ./cmd/polycom-manager
    if ($LASTEXITCODE -ne 0) { throw "Windows build failed" }
} finally {
    $env:CGO_ENABLED = $OldCGO
    $env:GOOS = $OldGOOS
    $env:GOARCH = $OldGOARCH
}

Get-ChildItem dist
