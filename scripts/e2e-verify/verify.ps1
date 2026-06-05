# Phase 6 e2e 検証スクリプト (Windows)
#
# 流れ:
#   1. mrhc.exe と gen-test-config をビルド (web/dist は既存前提)
#   2. 一時 dataDir に mrhc.config.json を生成
#   3. mrhc を起動 (バックグラウンド)
#   4. 認証は Authorization: Bearer <password>（APIKey は廃止）
#   5. /api/v1/start で headless 起動 (test-multi-world.json)
#   6. 構造化API を順に叩いて JSON を保存
#   7. cleanup: shutdown → mrhc 停止

param(
    [string]$Password = "test-password-only-for-e2e",
    [int]$Port = 8080,
    [string]$InstallDir = "C:\Program Files (x86)\Steam\steamapps\common\Resonite",
    [string]$TestConfig = "C:\app\MRHC\MarkNResoniteHeadlessController\scripts\empirical-capture\test-multi-world.json",
    [string]$OutDir = "C:\app\MRHC\MarkNResoniteHeadlessController\scripts\e2e-verify\results"
)

$ErrorActionPreference = 'Continue'
$repo = "C:\app\MRHC\MarkNResoniteHeadlessController"
$env:Path = "C:\Program Files\Go\bin;" + $env:Path

# 既存プロセス掃除
Stop-Process -Name "Resonite","mrhc","gen-test-config" -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2

# 出力ディレクトリ
$ts = Get-Date -Format "yyyyMMdd-HHmmss"
$runDir = Join-Path $OutDir $ts
New-Item -ItemType Directory -Force -Path $runDir | Out-Null
Write-Output "results dir: $runDir"

# ビルド
Set-Location $repo
Write-Output "=== build mrhc.exe ==="
go build -o "$repo\bin\mrhc.exe" .\cmd\mrhc
if (-not $?) { Write-Output "BUILD FAIL"; exit 1 }
Write-Output "=== build gen-test-config ==="
go build -o "$repo\bin\gen-test-config.exe" .\scripts\e2e-verify\gen-test-config
if (-not $?) { Write-Output "BUILD FAIL"; exit 1 }

# テスト用 data dir に config 生成
$dataDir = Join-Path $runDir "data"
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
$cfgPath = Join-Path $dataDir "mrhc.config.json"
& "$repo\bin\gen-test-config.exe" $Password $InstallDir $cfgPath | Out-Null
Write-Output "config: $cfgPath"
# 認証は Bearer パスワード（gen-test-config が config に bcrypt ハッシュを保存済）
$authHeaders = @{ Authorization = "Bearer $Password" }

# mrhc 起動
Write-Output "=== launch mrhc ==="
$mrhcOut = Join-Path $runDir "mrhc.out.log"
$mrhcErr = Join-Path $runDir "mrhc.err.log"
$mrhc = Start-Process -FilePath "$repo\bin\mrhc.exe" -ArgumentList "-data", "`"$dataDir`"" -PassThru -WindowStyle Hidden -RedirectStandardOutput $mrhcOut -RedirectStandardError $mrhcErr
Write-Output "mrhc pid=$($mrhc.Id)"

# サーバー起動待ち
$base = "http://localhost:$Port"
$ready = $false
$deadline = (Get-Date).AddSeconds(15)
while ((Get-Date) -lt $deadline) {
    try {
        $r = Invoke-WebRequest "$base/api/v1/status" -Headers $authHeaders -UseBasicParsing -ErrorAction Stop
        if ($r.StatusCode -eq 200) { $ready = $true; break }
    } catch { }
    Start-Sleep -Milliseconds 300
}
if (-not $ready) {
    Write-Output "!! mrhc server didn't respond"
    Get-Content $mrhcErr -Tail 20 -ErrorAction SilentlyContinue
    Stop-Process -Id $mrhc.Id -EA SilentlyContinue
    exit 2
}
Write-Output "mrhc up"

# helper: GET → JSON ファイル保存
function Save-Get {
    param([string]$Path, [string]$FileName)
    $url = "$base$Path"
    $outFile = Join-Path $runDir $FileName
    Write-Output ">>> GET $Path"
    try {
        $r = Invoke-WebRequest $url -Headers $authHeaders -UseBasicParsing -ErrorAction Stop
        $r.Content | Out-File -FilePath $outFile -Encoding utf8 -NoNewline
        Write-Output "  status=$($r.StatusCode) bytes=$($r.RawContentLength) saved=$outFile"
    } catch {
        $msg = $_.Exception.Message
        "ERROR: $msg" | Out-File -FilePath $outFile -Encoding utf8 -NoNewline
        Write-Output "  ERROR msg=$msg"
    }
}

# helper: POST (form encoded)
function Save-Post {
    param([string]$Path, [hashtable]$Body, [string]$FileName)
    $url = "$base$Path"
    $outFile = Join-Path $runDir $FileName
    Write-Output ">>> POST $Path  body=$($Body | ConvertTo-Json -Compress)"
    try {
        $r = Invoke-WebRequest $url -Method POST -Headers $authHeaders -Body ($Body | ConvertTo-Json) -ContentType "application/json" -UseBasicParsing -ErrorAction Stop
        $r.Content | Out-File -FilePath $outFile -Encoding utf8 -NoNewline
        Write-Output "  status=$($r.StatusCode) saved=$outFile"
    } catch {
        Write-Output "  EXCEPTION: $($_.Exception.Message)"
    }
}

# テスト config を headless config ディレクトリに配置し、名前で起動する（Pre-7b: start は名前指定）
$cfgDir = Join-Path $dataDir "headless-configs"
New-Item -ItemType Directory -Force -Path $cfgDir | Out-Null
Copy-Item -Path $TestConfig -Destination (Join-Path $cfgDir "e2e.json") -Force
Write-Output "test config copied to: $(Join-Path $cfgDir 'e2e.json')"

# headless 起動（config 名で指定。config 自身の creds が使われる）
Save-Post "/api/v1/start" @{ config = "e2e" } "01-start.json"

# Driver の State が Running になり Ready になるまで待つ
$ready = $false
$deadline = (Get-Date).AddSeconds(180)
while ((Get-Date) -lt $deadline) {
    try {
        $r = Invoke-WebRequest "$base/api/v1/status" -Headers $authHeaders -UseBasicParsing -ErrorAction Stop
        $j = $r.Content | ConvertFrom-Json
        if ($j.data.ready) { $ready = $true; break }
    } catch { }
    Start-Sleep -Seconds 2
}
if (-not $ready) {
    Write-Output "!! headless never became ready"
    Save-Get "/api/v1/status" "99-status-not-ready.json"
    Save-Post "/api/v1/stop" @{} "99-stop.json"
    Start-Sleep -Seconds 5
    Stop-Process -Id $mrhc.Id -EA SilentlyContinue
    Stop-Process -Name "Resonite" -EA SilentlyContinue
    exit 3
}
Write-Output "headless ready"

# 構造化API を順次叩く
Save-Get "/api/v1/sessions" "10-sessions.json"
Save-Get "/api/v1/sessions/0/status" "11-session-0-status.json"
Save-Get "/api/v1/sessions/0/users" "12-session-0-users.json"
Save-Get "/api/v1/sessions/1/status" "13-session-1-status.json"
Save-Get "/api/v1/sessions/1/users" "14-session-1-users.json"
Save-Get "/api/v1/listbans" "15-listbans.json"
Save-Get "/api/v1/friendrequests" "16-friendrequests.json"

# cleanup
Write-Output "=== shutdown headless ==="
Save-Post "/api/v1/stop" @{} "90-stop.json"
$deadline = (Get-Date).AddSeconds(90)
while ((Get-Date) -lt $deadline) {
    try {
        $r = Invoke-WebRequest "$base/api/v1/status" -Headers $authHeaders -UseBasicParsing -ErrorAction Stop
        $j = $r.Content | ConvertFrom-Json
        if ($j.data.state -eq "stopped") { break }
    } catch { break }
    Start-Sleep -Seconds 2
}
Write-Output "=== stopping mrhc ==="
Stop-Process -Id $mrhc.Id -EA SilentlyContinue
Stop-Process -Name "Resonite" -EA SilentlyContinue
Write-Output "=== DONE ==="
Write-Output "results: $runDir"
Get-ChildItem $runDir -Name | ForEach-Object { Write-Output "  $_" }
