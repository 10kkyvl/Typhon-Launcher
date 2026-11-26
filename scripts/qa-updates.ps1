# Watch the launcher update itself on Windows, from a FAKE chain of releases.
#
# Builds the qa launcher (bin\typhon.exe, the way `wails3 task build:qa` does),
# starts tools/devrelease -windows in a second window (ten qa builds of the
# next patch versions, signed with a test key and served on 127.0.0.1:8099),
# then starts the launcher in portable mode on a fresh data folder, pointed at
# that source. Nothing here is a real release; the installed launcher and its
# data are not touched. Run it from the shell you build Typhon in:
#   powershell -ExecutionPolicy Bypass -File scripts\qa-updates.ps1
# Every run makes a new data folder under %LOCALAPPDATA%\TyphonQAUpdates
# (portable-<time>); delete that folder when you are done.
# The people who wrote this script have never run it on Windows.

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$control = 'http://127.0.0.1:8099/control'

function Stop-QA([string]$message) {
    Write-Host $message -ForegroundColor Red
    exit 1
}

function Test-Control {
    try {
        Invoke-WebRequest -Uri $control -UseBasicParsing -TimeoutSec 2 | Out-Null
        return $true
    } catch {
        return $false
    }
}

foreach ($tool in 'go', 'wails3', 'npm') {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Stop-QA "$tool is not on PATH; install it or open the shell you build Typhon from."
    }
}
if (Test-Control) {
    Stop-QA "Something already answers on 127.0.0.1:8099; close the earlier devrelease window first."
}

$version = (Get-Content (Join-Path $root 'VERSION') -Raw).Trim()
if (-not ($version -match '^(\d+)\.(\d+)\.(\d+)$')) {
    Stop-QA "VERSION holds '$version', not MAJOR.MINOR.PATCH."
}
$next = '{0}.{1}.{2}' -f $Matches[1], $Matches[2], ([int]$Matches[3] + 1)

# Always rebuilt: frontend\dist is ignored by git, so one left by another
# branch or build would be embedded as it is.
if (-not (Test-Path (Join-Path $root 'frontend\node_modules'))) {
    Stop-QA "frontend\node_modules is missing; install the frontend dependencies the way you usually do (for example npm --prefix frontend ci), then run this script again."
}
Write-Host "Building the frontend..."
wails3 generate bindings -f '-tags qa' -clean=true -ts -i
if ($LASTEXITCODE -ne 0) { Stop-QA "Generating the frontend bindings failed; see the output above." }
npm --prefix frontend run build
if ($LASTEXITCODE -ne 0) { Stop-QA "Building the frontend failed; see the output above." }

Write-Host "Building bin\typhon.exe $version with the qa tag..."
Push-Location (Join-Path $root 'build')
wails3 generate syso -arch amd64 -icon windows/icon.ico -manifest windows/wails.exe.manifest -info windows/info.json -out ../wails_windows_amd64.syso
$sysoExit = $LASTEXITCODE
Pop-Location
if ($sysoExit -ne 0) { Stop-QA "Generating the Windows resource failed; see the output above." }
$buildEnv = 'GOOS', 'GOARCH', 'CGO_ENABLED'
$before = @{}
foreach ($name in $buildEnv) { $before[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
go build -tags qa -trimpath -buildvcs=false -ldflags "-w -s -H windowsgui -X typhon/internal/app.Version=$version" -o bin\typhon.exe .
$buildExit = $LASTEXITCODE
foreach ($name in $buildEnv) { [Environment]::SetEnvironmentVariable($name, $before[$name], 'Process') }
Remove-Item (Join-Path $root 'wails_windows_amd64.syso')
if ($buildExit -ne 0) { Stop-QA "Building the qa launcher failed; see the output above (a running bin\typhon.exe cannot be replaced)." }

$base = Join-Path $env:LOCALAPPDATA 'TyphonQAUpdates'
$state = Join-Path $base 'devrelease'
New-Item -ItemType Directory -Force -Path $state | Out-Null
$envFile = Join-Path $state 'update.ps1'
if (Test-Path $envFile) { Remove-Item $envFile }
$failed = Join-Path $state 'failed'
if (Test-Path $failed) { Remove-Item $failed }

Write-Host "Starting the fake release source ($next and nine more) in a second window..."
$quotedRoot = $root.Replace("'", "''")
$quotedState = $state.Replace("'", "''")
$quotedFailed = $failed.Replace("'", "''")
# The second window stays open after devrelease ends, so its output can be
# read; the marker file is how this window learns that it ended.
$serve = "Set-Location '$quotedRoot'; go run ./tools/devrelease -windows -version $next -count 10 -seconds 5 -state '$quotedState'; New-Item -ItemType File -Force -Path '$quotedFailed' | Out-Null; Write-Host 'devrelease stopped; read the lines above.' -ForegroundColor Red; Read-Host 'Press Enter to close this window'"
$encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($serve))
$source = Start-Process powershell -ArgumentList '-EncodedCommand', $encoded -PassThru

# Ten cross-builds take minutes; the source answers once they are signed.
$deadline = (Get-Date).AddMinutes(30)
while (-not (Test-Control)) {
    if (Test-Path $failed) { Stop-QA "devrelease stopped before the source answered; read its window." }
    if ($source.HasExited) { Stop-QA "The devrelease window closed before the source answered." }
    if ((Get-Date) -gt $deadline) { Stop-QA "devrelease did not answer within 30 minutes; read its window." }
    Start-Sleep -Seconds 3
}
if (-not (Test-Path $envFile)) { Stop-QA "devrelease answers but wrote no $envFile." }
. $envFile

$env:TYPHON_PORTABLE_DIR = Join-Path $base ('portable-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Path $env:TYPHON_PORTABLE_DIR | Out-Null
Write-Host "Starting bin\typhon.exe $version in portable mode on $env:TYPHON_PORTABLE_DIR"
Write-Host "Update source: $env:TYPHON_QA_MANIFEST_URL (switch failures with POST $control)"
Start-Process -FilePath (Join-Path $root 'bin\typhon.exe') -WorkingDirectory (Join-Path $root 'bin')
