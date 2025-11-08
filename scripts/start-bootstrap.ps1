#Requires -Version 5.0
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Resolve-Path (Join-Path $scriptRoot '..')
Set-Location -Path $projectRoot

function Pause-IfInteractive {
    param(
        [int]$ExitCode = 0
    )

    if ($Host.Name -eq 'ConsoleHost' -and [Environment]::UserInteractive) {
        Write-Host ""
        Write-Host "Press any key to close..."
        [void][System.Console]::ReadKey($true)
    }
    exit $ExitCode
}

Write-Host "Checking Node.js installation..."
$nodeCommand = Get-Command node -ErrorAction SilentlyContinue

if (-not $nodeCommand) {
    Write-Host "-------------------------------------------------------------------"
    Write-Host "[ERROR] Node.js was not found."
    Write-Host "MarkN Resonite Headless Controller requires Node.js 20 or later."
    Write-Host "Install the latest LTS (v20 or newer) from https://nodejs.org/"
    Write-Host "-------------------------------------------------------------------"
    Pause-IfInteractive -ExitCode 1
}

$nodeVersionOutput = & $nodeCommand.Source -v 2>$null
if ($LASTEXITCODE -ne 0 -or -not $nodeVersionOutput) {
    Write-Host "-------------------------------------------------------------------"
    Write-Host "[ERROR] Failed to read Node.js version information."
    Write-Host "Make sure Node.js is installed and added to PATH."
    Write-Host "-------------------------------------------------------------------"
    Pause-IfInteractive -ExitCode 1
}

$nodeVersionOutput = $nodeVersionOutput.Trim()
Write-Host "[INFO] Detected Node.js version: $nodeVersionOutput"

$versionPattern = 'v(?<major>\d+)(?:\.(?<minor>\d+))?(?:\.(?<patch>\d+))?'
$versionMatch = [System.Text.RegularExpressions.Regex]::Match($nodeVersionOutput, $versionPattern)
if ($null -eq $versionMatch -or -not $versionMatch.Success) {
    Write-Host "-------------------------------------------------------------------"
    Write-Host "[ERROR] Unable to parse Node.js version: $nodeVersionOutput"
    Write-Host "Node.js 20 or later is required."
    Write-Host "-------------------------------------------------------------------"
    Pause-IfInteractive -ExitCode 1
}

$majorGroup = $versionMatch.Groups['major']
if ($null -eq $majorGroup -or -not $majorGroup.Success) {
    Write-Host "-------------------------------------------------------------------"
    Write-Host "[ERROR] Failed to read the Node.js major version from: $nodeVersionOutput"
    Write-Host "Node.js 20 or later is required."
    Write-Host "-------------------------------------------------------------------"
    Pause-IfInteractive -ExitCode 1
}

$major = [int]$majorGroup.Value
if ($major -lt 20) {
    Write-Host "-------------------------------------------------------------------"
    Write-Host "[ERROR] Detected Node.js v$major.x."
    Write-Host "MarkN Resonite Headless Controller requires Node.js 20 or later."
    Write-Host "Install the latest LTS (v20 or newer) from https://nodejs.org/"
    Write-Host "-------------------------------------------------------------------"
    Pause-IfInteractive -ExitCode 1
}

Write-Host ""
Write-Host "Starting application..."

$startScript = Join-Path -Path $scriptRoot -ChildPath 'start.js'

& $nodeCommand.Source $startScript
$exitCode = $LASTEXITCODE

if ($exitCode -ne 0) {
    Write-Host ""
    Write-Host "start.js exited with error code $exitCode."
}

exit $exitCode
