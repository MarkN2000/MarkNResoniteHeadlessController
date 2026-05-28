# Phase 6 e2e verify の results/ を掃除する。
# 既定: 最新 N 件を残して古いものを削除。
#
# usage:
#   .\scripts\e2e-verify\cleanup.ps1            # 最新 3 件を残し他を削除
#   .\scripts\e2e-verify\cleanup.ps1 -Keep 1    # 最新 1 件を残す
#   .\scripts\e2e-verify\cleanup.ps1 -Keep 0    # 全削除
#   .\scripts\e2e-verify\cleanup.ps1 -DryRun    # 削除対象表示のみ

param(
    [int]$Keep = 3,
    [switch]$DryRun
)

$root = Join-Path $PSScriptRoot "results"
if (-not (Test-Path $root)) {
    Write-Output "results dir 無し: $root"
    exit 0
}

$dirs = Get-ChildItem $root -Directory | Sort-Object Name -Descending
Write-Output "総 run 数: $($dirs.Count) / 残す: $Keep"

$toDelete = $dirs | Select-Object -Skip $Keep
if (-not $toDelete) {
    Write-Output "削除対象なし"
    exit 0
}
foreach ($d in $toDelete) {
    if ($DryRun) {
        Write-Output "[dry-run] would delete: $($d.FullName)"
    } else {
        Write-Output "削除: $($d.Name)"
        Remove-Item $d.FullName -Recurse -Force
    }
}
