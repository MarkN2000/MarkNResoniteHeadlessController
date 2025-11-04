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
        throw "コピー対象が存在しません: $SourceRelative。先に npm run build を実行してください。"
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
        throw "コピー対象が存在しません: $SourceRelative"
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
        throw "コピー対象のファイルが存在しません: $SourceRelative"
    }

    $destination = Join-Path $packageRoot $DestinationRelative
    $destinationParent = Split-Path -Parent $destination
    Ensure-Directory -Path $destinationParent

    Copy-Item -LiteralPath $source -Destination $destination -Force
}

Write-Host "📦 MarkN Resonite Headless Controller - 配布Zipパッケージ作成" -ForegroundColor Cyan

if (-not $SkipBuild) {
    Write-Host "[1/5] npm run build を実行中..." -ForegroundColor Yellow
    & npm run build
    if ($LASTEXITCODE -ne 0) {
        throw "npm run build に失敗しました (exit code: $LASTEXITCODE)"
    }
}
else {
    Write-Host "[1/5] npm run build をスキップしました (--SkipBuild)" -ForegroundColor Yellow
}

$distDir = Join-Path $root 'dist'
$stagingDir = Join-Path $distDir 'package-staging'
$packageRoot = Join-Path $stagingDir 'MarkNResoniteHeadlessController'
$zipPath = Join-Path $distDir 'MarkNResoniteHeadlessController.zip'

Write-Host "[2/5] ステージングディレクトリを初期化中..." -ForegroundColor Yellow
if (Test-Path -LiteralPath $stagingDir) {
    Remove-Item -LiteralPath $stagingDir -Recurse -Force
}
Ensure-Directory -Path $packageRoot

Write-Host "[3/5] 必要なファイル/ディレクトリをコピー中..." -ForegroundColor Yellow

$directoriesToCopy = @(
    @{ Source = 'backend\dist'; Destination = 'backend\dist' },
    @{ Source = 'frontend\build'; Destination = 'frontend\build' },
    @{ Source = 'shared\dist'; Destination = 'shared\dist' }
)

foreach ($entry in $directoriesToCopy) {
    Copy-RequiredDirectory -SourceRelative $entry.Source -DestinationRelative $entry.Destination
}

# サンプルヘッドレス設定（機密情報を含まない）
Copy-RequiredDirectoryContents -SourceRelative 'sample' -DestinationRelative 'config\headless'

$filesToCopy = @(
    @{ Source = 'package.json'; Destination = 'package.json' },
    @{ Source = 'package-lock.json'; Destination = 'package-lock.json' },
    @{ Source = 'README.md'; Destination = 'README.md' },
    @{ Source = 'DISTRIBUTION_README.md'; Destination = 'DISTRIBUTION_README.md' },
    @{ Source = 'DISTRIBUTION_REQUIREMENTS.md'; Destination = 'DISTRIBUTION_REQUIREMENTS.md' },
    @{ Source = 'DEPLOYMENT.md'; Destination = 'DEPLOYMENT.md' },
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

Write-Host "[4/5] Zipファイルを生成中..." -ForegroundColor Yellow
if (Test-Path -LiteralPath $zipPath) {
    Remove-Item -LiteralPath $zipPath -Force
}

Compress-Archive -Path $packageRoot -DestinationPath $zipPath -CompressionLevel Optimal -Force

Write-Host "[5/5] ステージングディレクトリをクリーンアップ中..." -ForegroundColor Yellow
Remove-Item -LiteralPath $stagingDir -Recurse -Force

Write-Host "✅ Zipパッケージを作成しました: $zipPath" -ForegroundColor Green
Write-Host "   ※ 機密情報 (.env や config/*.json 等) は含まれていません。" -ForegroundColor Green

