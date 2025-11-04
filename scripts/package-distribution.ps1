param(
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'

$root = Resolve-Path "$PSScriptRoot\.."
Set-Location $root

function Ensure-Directory {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { return }
    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Path $Path -Force | Out-Null
    }
}

function Copy-RequiredDirectory {
    param(
        [string]$SourceRelative,
        [string]$DestinationRelative
    )

    $source = Join-Path $root $SourceRelative
    if (-not (Test-Path -LiteralPath $source)) {
        throw "Required source not found: $SourceRelative. Run npm run build first."
    }

    $destination = Join-Path $packageRoot $DestinationRelative
    $destinationParent = Split-Path -Parent $destination
    Ensure-Directory -Path $destinationParent

    Copy-Item -LiteralPath $source -Destination $destinationParent -Recurse -Force
}

function Copy-RequiredDirectoryContents {
    param(
        [string]$SourceRelative,
        [string]$DestinationRelative
    )

    $source = Join-Path $root $SourceRelative
    if (-not (Test-Path -LiteralPath $source)) {
        throw "Required source not found: $SourceRelative"
    }

    $destination = Join-Path $packageRoot $DestinationRelative
    Ensure-Directory -Path $destination

    $items = Get-ChildItem -LiteralPath $source -Force
    foreach ($item in $items) {
        Copy-Item -LiteralPath $item.FullName -Destination $destination -Recurse -Force
    }
}

function Copy-RequiredFile {
    param(
        [string]$SourceRelative,
        [string]$DestinationRelative
    )

    $source = Join-Path $root $SourceRelative
    if (-not (Test-Path -LiteralPath $source)) {
        throw "Required file not found: $SourceRelative"
    }

    $destination = Join-Path $packageRoot $DestinationRelative
    $destinationParent = Split-Path -Parent $destination
    Ensure-Directory -Path $destinationParent

    Copy-Item -LiteralPath $source -Destination $destination -Force
}

Write-Host "Packaging MarkN Resonite Headless Controller" -ForegroundColor Cyan

if (-not $SkipBuild) {
    Write-Host "[1/5] Running npm run build..." -ForegroundColor Yellow

    $npmInfo = Get-Command npm -ErrorAction SilentlyContinue
    if (-not $npmInfo) {
        throw "npm command not found in PATH. Install Node.js or adjust PATH."
    }

    $process = Start-Process -FilePath 'cmd.exe' -ArgumentList '/c', 'npm run build' -NoNewWindow -Wait -PassThru
    if ($process.ExitCode -ne 0) {
        throw "npm run build failed (exit code: $($process.ExitCode))"
    }
}
else {
    Write-Host "[1/5] Skipped npm run build (--SkipBuild)" -ForegroundColor Yellow
}

$distDir = Join-Path $root 'dist'
$stagingDir = Join-Path $distDir 'package-staging'
$packageRoot = Join-Path $stagingDir 'MarkNResoniteHeadlessController'
$zipPath = Join-Path $distDir 'MarkNResoniteHeadlessController.zip'

Write-Host "[2/5] Preparing staging directory..." -ForegroundColor Yellow
if (Test-Path -LiteralPath $stagingDir) {
    Remove-Item -LiteralPath $stagingDir -Recurse -Force
}
Ensure-Directory -Path $packageRoot

Write-Host "[3/5] Copying required files..." -ForegroundColor Yellow

$directoriesToCopy = @(
    @{ Source = 'backend\dist'; Destination = 'backend\dist' },
    @{ Source = 'frontend\build'; Destination = 'frontend\build' },
    @{ Source = 'shared\dist'; Destination = 'shared\dist' }
)

foreach ($entry in $directoriesToCopy) {
    Copy-RequiredDirectory -SourceRelative $entry.Source -DestinationRelative $entry.Destination
}

# Copy sample headless configuration (no secrets)
Copy-RequiredDirectoryContents -SourceRelative 'sample' -DestinationRelative 'config\headless'

$filesToCopy = @(
    @{ Source = 'package.json'; Destination = 'package.json' },
    @{ Source = 'package-lock.json'; Destination = 'package-lock.json' },
    @{ Source = 'backend\package.json'; Destination = 'backend\package.json' },
    @{ Source = 'README.md'; Destination = 'README.md' },
    @{ Source = 'env.example'; Destination = 'env.example' },
    @{ Source = 'scripts\setup.bat'; Destination = 'scripts\setup.bat' },
    @{ Source = 'scripts\setup.js'; Destination = 'scripts\setup.js' },
    @{ Source = 'scripts\start-production.bat'; Destination = 'scripts\start-production.bat' }
)

foreach ($entry in $filesToCopy) {
    Copy-RequiredFile -SourceRelative $entry.Source -DestinationRelative $entry.Destination
}

$configExamples = @(
    'config\auth.json.example',
    'config\security.json.example',
    'config\runtime-state.json.example',
    'backend\config\restart.json.example',
    'backend\config\restart-status.json.example'
)

foreach ($path in $configExamples) {
    Copy-RequiredFile -SourceRelative $path -DestinationRelative $path
}

Write-Host "[4/5] Creating zip file..." -ForegroundColor Yellow
if (Test-Path -LiteralPath $zipPath) {
    Remove-Item -LiteralPath $zipPath -Force
}

Compress-Archive -Path $packageRoot -DestinationPath $zipPath -CompressionLevel Optimal -Force

Write-Host "[5/5] Cleaning up staging directory..." -ForegroundColor Yellow
Remove-Item -LiteralPath $stagingDir -Recurse -Force

Write-Host "Package created: $zipPath" -ForegroundColor Green
Write-Host "Secrets (.env, config/*.json, etc.) are NOT included." -ForegroundColor Green

