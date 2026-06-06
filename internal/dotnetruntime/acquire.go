// Package dotnetruntime はヘッドレスの実行に必要な .NET ランタイムを公式ビルドフィードから
// 取得し、<installDir>/dotnet-runtime へ自己完結で設置する（sudo/管理者権限 不要）。
// 公式クライアントは初回起動時に LinuxBootstrap.sh（dotnet-install.sh）/ InstallScript.vdf で
// ランタイムを設置するが、DD の DL 品にはどちらの成果物も含まれないため MRHC が同等を担う。
// 取得 URL は dotnet-install.sh 自身が使う安定フィード（2026-06-07 実地検証）。チャンネルは
// platform.ReadRuntimeRequirement（runtimeconfig.json 正本）から呼び出し側が導出して渡す。
// 設計: docs/design/dotnet-runtime.md
package dotnetruntime

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// defaultFeedBase は公式ビルドフィード（dotnet-install.sh の azure_feed 既定値と同一）。
const defaultFeedBase = "https://builds.dotnet.microsoft.com/dotnet"

// defaultIdleTimeout は DL の無進捗打ち切り時間。steam.Manager の stall ウォッチドッグは
// DD 区間専用のため、本パッケージはハング対策を自前で持つ（進捗があれば何分でも続く）。
const defaultIdleTimeout = 60 * time.Second

// runtimeDirName は設置先ディレクトリ名（launcher の bundledDotnetPath と同じ規約）。
const runtimeDirName = "dotnet-runtime"

// MilestoneInstalling は設置区間の開始を表すマイルストーン名。
// steam.Manager の phase 表示・wizard の区間ラベル出力が本値で判定する。
const MilestoneInstalling = "Installing .NET Runtime"

// Event は設置中の進捗通知。steam.Event の最小同形（import 循環を避けるため別型。
// steam 側がフィールド同名で写して SSE へ流す）。
type Event struct {
	Kind    string            // "log" | "progress" | "milestone"
	Text    string            // log 行（ja 原文）/ milestone 名
	Percent float64           // progress: 0..100
	File    string            // progress: 対象ファイル名
	MsgKey  string            // log: 文言キー（表示層が locale 変換）
	MsgArgs map[string]string // log: MsgKey の補間引数
}

// platformKey は現在の実行プラットフォーム。テストで差し替えられるよう変数にする。
var platformKey = runtime.GOOS + "/" + runtime.GOARCH

// ridFor は OS/arch を .NET の Runtime Identifier とアーカイブ形式へ対応づける。
func ridFor(key string) (rid, ext string, err error) {
	switch key {
	case "windows/amd64":
		return "win-x64", ".zip", nil
	case "linux/amd64":
		return "linux-x64", ".tar.gz", nil
	case "linux/arm64":
		return "linux-arm64", ".tar.gz", nil
	}
	return "", "", fmt.Errorf("この OS/アーキテクチャ用の .NET ランタイム配布がありません: %s", key)
}

// Acquirer は .NET ランタイムの取得器。テストで FeedBase/Client/IdleTimeout を差し替えられる。
type Acquirer struct {
	FeedBase    string        // ビルドフィードの取得元
	Client      *http.Client  // HTTP クライアント
	IdleTimeout time.Duration // DL 無進捗の打ち切り（0 は既定値）
}

// NewAcquirer は既定（公式フィード・既定HTTPクライアント）の取得器を返す。
func NewAcquirer() *Acquirer {
	return &Acquirer{FeedBase: defaultFeedBase, Client: http.DefaultClient, IdleTimeout: defaultIdleTimeout}
}

// Ensure は <installDir>/dotnet-runtime に channel（例 "10.0"）の最新ランタイムを設置する。
//  1. latest.version で版を確定（prerelease は設置せずエラー＝再DLループ防止）
//  2. 確定版が既に設置済みなら何もせず成功（再DL防止・オフラインでも latest 照会だけで済む）
//  3. アーカイブ DL → SHA-512 突合（フィード併設の .sha512。公式スクリプトのサイズ比較より強い）
//  4. .dotnet-runtime.new へ全展開 → 既存と2段 rename でスワップ（全置換・マージしない）
//
// 進捗・ログは onEvent（nil 可）へ通知する。戻り値は設置（確認）した版。
func (a *Acquirer) Ensure(ctx context.Context, installDir, channel string, onEvent func(Event)) (string, error) {
	rid, ext, err := ridFor(platformKey)
	if err != nil {
		return "", err
	}

	version, err := a.latestVersion(ctx, channel)
	if err != nil {
		return "", err
	}

	finalDir := filepath.Join(installDir, runtimeDirName)
	recoverStaleSwap(installDir, finalDir)
	if installedHas(finalDir, version) {
		return version, nil // 設置済み（冪等スキップ・ログも出さない）
	}

	emit(onEvent, Event{Kind: "milestone", Text: MilestoneInstalling})
	file := "dotnet-runtime-" + version + "-" + rid + ext
	emit(onEvent, Event{
		Kind: "log", Text: fmt.Sprintf(".NET ランタイム %s を取得します（%s）", version, file),
		MsgKey: "dotnetInstall", MsgArgs: map[string]string{"version": version, "file": file},
	})

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("ディレクトリ作成に失敗: %w", err)
	}
	// 一時アーカイブへDL（installDir と同一FSに置く＝後段の rename を原子的にできるよう近接させる）。
	tmpArc, err := os.CreateTemp(installDir, ".dotnet-*"+ext)
	if err != nil {
		return "", fmt.Errorf("一時ファイル作成に失敗: %w", err)
	}
	tmpArcPath := tmpArc.Name()
	defer os.Remove(tmpArcPath) // 成否にかかわらずアーカイブは残さない

	url := a.FeedBase + "/Runtime/" + version + "/" + file
	sum, dlErr := a.download(ctx, url, tmpArc, file, onEvent)
	closeErr := tmpArc.Close()
	if dlErr != nil {
		return "", dlErr
	}
	if closeErr != nil {
		return "", fmt.Errorf("一時ファイルのクローズに失敗: %w", closeErr)
	}

	expected, err := a.fetchSHA512(ctx, url+".sha512")
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(sum, expected) {
		return "", fmt.Errorf("SHA-512 が一致しません（期待=%s 実際=%s）", expected, sum)
	}

	stageDir := filepath.Join(installDir, ".dotnet-runtime.new")
	if err := os.RemoveAll(stageDir); err != nil {
		return "", fmt.Errorf("stale な展開先の掃除に失敗: %w", err)
	}
	if err := extractArchive(tmpArcPath, ext, stageDir); err != nil {
		os.RemoveAll(stageDir)
		return "", err
	}
	if err := swapRuntimeDir(installDir, stageDir, finalDir); err != nil {
		os.RemoveAll(stageDir)
		return "", err
	}

	emit(onEvent, Event{
		Kind: "log", Text: fmt.Sprintf(".NET ランタイム %s の設置が完了しました", version),
		MsgKey: "dotnetInstalled", MsgArgs: map[string]string{"version": version},
	})
	return version, nil
}

// latestVersion はチャンネルの最新確定版を取得する。
func (a *Acquirer) latestVersion(ctx context.Context, channel string) (string, error) {
	body, err := a.getSmall(ctx, a.FeedBase+"/Runtime/"+channel+"/latest.version")
	if err != nil {
		return "", fmt.Errorf("最新版の取得に失敗: %w", err)
	}
	version, err := parseLatestVersion(body)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(version, "-+") {
		// GA 前チャンネルは prerelease を返す（実測 11.0 → 11.0.0-preview.…）。.NET ホストは
		// release 要求から prerelease へ roll-forward しないため、設置しても起動に使えない。
		return "", fmt.Errorf("チャンネル %s の最新版 %s はプレリリースです（設置をスキップ）", channel, version)
	}
	for _, r := range version {
		if r != '.' && (r < '0' || r > '9') {
			return "", fmt.Errorf("最新版の表記が不正です: %q", version)
		}
	}
	return version, nil
}

// parseLatestVersion は latest.version 本文から版を取り出す。
// 1行（版のみ）と2行（commit hash＋版）の両形式があり得るため「最後の非空行」を採る
// （dotnet-install.sh の tail -n 1 と同じ規則）。
func parseLatestVersion(body string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if v := strings.TrimSpace(lines[i]); v != "" {
			return v, nil
		}
	}
	return "", errors.New("latest.version が空です")
}

// installedHas は確定版 version が設置済み（shared 配下に版ディレクトリ・host も存在）かを返す。
func installedHas(finalDir, version string) bool {
	if _, err := os.Stat(filepath.Join(finalDir, "shared", "Microsoft.NETCore.App", version)); err != nil {
		return false
	}
	for _, name := range []string{"dotnet", "dotnet.exe"} {
		if fi, err := os.Stat(filepath.Join(finalDir, name)); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

// fetchSHA512 は併設チェックサム（"<hex128>  <filename>" 形式）から先頭フィールドを取り出す。
func (a *Acquirer) fetchSHA512(ctx context.Context, url string) (string, error) {
	body, err := a.getSmall(ctx, url)
	if err != nil {
		return "", fmt.Errorf("チェックサムの取得に失敗: %w", err)
	}
	fields := strings.Fields(body)
	if len(fields) == 0 || len(fields[0]) != sha512.Size*2 {
		return "", fmt.Errorf("チェックサムの形式が不正です: %q", strings.TrimSpace(body))
	}
	return fields[0], nil
}

// getSmall は小さなテキスト資源（latest.version / .sha512）を取得する。
func (a *Acquirer) getSmall(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "MRHC")
	resp, err := a.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d (%s)", resp.StatusCode, url)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// download は url を w へストリームコピーしつつ SHA-512（小文字hex）を計算して返す。
// Content-Length が分かる場合は 1% 刻みで progress を通知する。
// 無進捗が IdleTimeout 続いたら打ち切る（リクエストを cancel して「無進捗」と報告）。
func (a *Acquirer) download(ctx context.Context, url string, w io.Writer, file string, onEvent func(Event)) (string, error) {
	idle := a.IdleTimeout
	if idle <= 0 {
		idle = defaultIdleTimeout
	}
	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "MRHC")
	resp, err := a.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ダウンロードに失敗: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ダウンロードに失敗: HTTP %d (%s)", resp.StatusCode, url)
	}

	// 無進捗ウォッチドッグ: 最終読み取り時刻を進捗側が更新し、idle 超過で DL の ctx を cancel する。
	var lastRead atomic.Int64
	lastRead.Store(time.Now().UnixNano())
	var stalled atomic.Bool
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		ticker := time.NewTicker(min(idle/4, time.Second))
		defer ticker.Stop()
		for {
			select {
			case <-watchDone:
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastRead.Load())) > idle {
					stalled.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	h := sha512.New()
	pw := &progressWriter{total: resp.ContentLength, file: file, onEvent: onEvent, lastRead: &lastRead}
	if _, err := io.Copy(io.MultiWriter(w, h, pw), resp.Body); err != nil {
		if stalled.Load() {
			return "", fmt.Errorf("ダウンロードが無進捗のため中断しました（%s 無応答）", idle)
		}
		return "", fmt.Errorf("ダウンロード中の読み込みに失敗: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// progressWriter は受信バイト数から 1% 刻みの progress イベントを生成する io.Writer。
type progressWriter struct {
	total    int64 // Content-Length（不明は -1）
	written  int64
	lastPct  int
	file     string
	onEvent  func(Event)
	lastRead *atomic.Int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	p.lastRead.Store(time.Now().UnixNano())
	if p.total > 0 {
		if pct := int(float64(p.written) / float64(p.total) * 100); pct > p.lastPct {
			p.lastPct = pct
			emit(p.onEvent, Event{Kind: "progress", Percent: float64(pct), File: p.file})
		}
	}
	return len(b), nil
}

func emit(onEvent func(Event), e Event) {
	if onEvent != nil {
		onEvent(e)
	}
}
