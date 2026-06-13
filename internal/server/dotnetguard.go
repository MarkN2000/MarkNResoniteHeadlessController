// dotnetguard.go は起動時ガード（.NET ランタイムの自動設置→起動）を担う。
// DD の DL 品に .NET ランタイムは含まれないため、初回 DL を経ない経路（DL 中断後の再起動・
// 手動 installDir・設置失敗後のリトライ）でも「起動」ワンアクションで復旧できるようにする。
// 通常は steam 更新の完了後フック（steam/dotnet.go）が設置済みにするため、本ガードは
// ローカル判定（ms オーダー）で素通りする。設計: docs/design/dotnet-runtime.md
package server

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/steam"
)

// runtimeGuardNeeded は起動前にランタイム設置の非同期経路へ入るべきかを判定する
// （オフライン安全・ローカル列挙のみ。サブプロセスやネットへは出ない）。
// false ＝従来の同期起動（既存挙動そのまま）。
func (s *Server) runtimeGuardNeeded(headlessPath string) bool {
	headlessDir := filepath.Dir(headlessPath)
	req, ok := s.readRuntimeReq(headlessDir)
	if !ok {
		return false // 要求が読めない（fakehl 等）＝楽観・従来挙動
	}
	installDir := filepath.Dir(headlessDir)
	if s.localSatisfies(installDir, req, runtime.GOARCH) {
		return false
	}
	return !s.sysDotnetCached(installDir, req)
}

// startWithUpdate はコールド起動（停止状態からの手動起動）の「更新→起動」を非同期で行う。
// 先に maybeUpdate("manual") で Resonite を更新し（失敗は古い版のまま続行）、その後は既存の
// startWithRuntimeGuard（.NET ガード→起動→記録）に委ねる＝共通化。更新をユーザーが中止したときだけ
// 起動を見送る（明確な中止意思を尊重・.NET ガードの中止作法に揃える）。HTTP には受付返済済み。
func (s *Server) startWithUpdate(name, headlessPath, launchPath string) {
	lang := s.langSnapshot()
	// 進捗は Steam SSE（設定タブ）に出る。コールド起動なので orchestrator の進行表示は使わない（onUpdating=nil）。
	err := s.maybeUpdate(s.backgroundCtx(), "manual", nil)
	if errors.Is(err, steam.ErrCancelled) || errors.Is(err, context.Canceled) {
		s.driver.PublishSys(i18n.T(lang, "update.sysStartCancelled"))
		return
	}
	s.startWithRuntimeGuard(name, headlessPath, launchPath)
}

// startWithRuntimeGuard は「（必要なら）設置→起動」を非同期で行う。HTTP には受付を返済済みの
// ため、以後の進捗は steam SSE（設定タブ）、結果・失敗は sys ログ（UI コンソール）で示す。
func (s *Server) startWithRuntimeGuard(name, headlessPath, launchPath string) {
	headlessDir := filepath.Dir(headlessPath)
	installDir := filepath.Dir(headlessDir)
	lang := s.langSnapshot()
	req, ok := s.readRuntimeReq(headlessDir)
	if !ok {
		s.finishGuardStart(name, headlessPath, launchPath, lang)
		return
	}

	// システム .NET の確認はサブプロセス実行（最大10s）を含むため goroutine 側で行う。
	// 充足ならキャッシュして以後の起動を同期経路に戻す（毎回の probe・accepted 応答を避ける）。
	if s.systemSatisfies(runtime.GOOS, runtime.GOARCH, req) {
		s.cacheSysDotnet(installDir, req)
		s.finishGuardStart(name, headlessPath, launchPath, lang)
		return
	}

	s.driver.PublishSys(i18n.T(lang, "dotnet.sysInstalling", req.Channel()))
	err := s.installRuntime(s.backgroundCtx(), installDir)
	switch {
	case err == nil:
		s.driver.PublishSys(i18n.T(lang, "dotnet.sysInstalled"))
	case errors.Is(err, steam.ErrCancelled):
		// ユーザー自身の中止（/steam/cancel）や shutdown を「失敗」として案内しない（中立文言）。
		s.driver.PublishSys(i18n.T(lang, "dotnet.sysCancelled"))
		return
	case errors.Is(err, steam.ErrUpdateInProgress):
		s.driver.PublishSys(i18n.T(lang, "dotnet.sysUpdateInProgress"))
		return
	default:
		// 設置失敗でも起動は best-effort で試みる: システム .NET が在るのに probe が機能しない
		// （Unknown）環境や installDir 書込不可の環境で、従来成功していた起動を壊さない
		// ＝可用性の下限は現行挙動。失敗理由と手動導入手段は先に案内しておく。
		s.driver.PublishSys(i18n.T(lang, "dotnet.sysFailed", err,
			platform.ManualDotnetInstallHint(runtime.GOOS, req.Channel())))
	}
	s.finishGuardStart(name, headlessPath, launchPath, lang)
}

// finishGuardStart はガード経路の最終段＝ヘッドレス起動と成功時の記録（同期経路と同じ副作用）。
func (s *Server) finishGuardStart(name, headlessPath, launchPath string, lang i18n.Lang) {
	// 設置完了→起動の隙間に手動の Steam 更新が begin した場合は起動を見送る
	// （起動してしまうと「稼働中に DD が install を書き換える」状態になる。
	// 更新側の headless_running ガードはこの順序では効かないため、こちら側で避ける）。
	if s.steamRunning() {
		s.driver.PublishSys(i18n.T(lang, "dotnet.sysStartDeferred"))
		return
	}
	go s.publishDepGuide() // 同期経路と同じ予防ガイド（R-C 経路③）
	if err := s.driver.Start(headlessPath, launchPath, name); err != nil {
		// accepted 応答後の失敗を黙殺しない（sys ログで明示）。
		s.driver.PublishSys(i18n.T(lang, "dotnet.sysStartFailed", err))
		return
	}
	s.recordLastUsed(name)
	s.recordLastStart("manual", time.Now().Format(time.RFC3339))
}

// langSnapshot は表示言語を cfgMu 下で読む（ガード goroutine 用のスナップショット）。
func (s *Server) langSnapshot() i18n.Lang {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.LangOrDefault()
}

// backgroundCtx は Start() が設定した bg ctx（shutdown で中断）を返す。未設定（テスト等）は Background。
func (s *Server) backgroundCtx() context.Context {
	s.bgMu.Lock()
	defer s.bgMu.Unlock()
	if s.bgCtx != nil {
		return s.bgCtx
	}
	return context.Background()
}

// sysDotnetKey はシステム .NET 充足キャッシュのキー（installDir と要求版の組）。
func sysDotnetKey(installDir string, req platform.RuntimeRequirement) string {
	return installDir + "|" + req.Raw
}

func (s *Server) sysDotnetCached(installDir string, req platform.RuntimeRequirement) bool {
	s.sysDotnetMu.Lock()
	defer s.sysDotnetMu.Unlock()
	return s.sysDotnetOK[sysDotnetKey(installDir, req)]
}

func (s *Server) cacheSysDotnet(installDir string, req platform.RuntimeRequirement) {
	s.sysDotnetMu.Lock()
	defer s.sysDotnetMu.Unlock()
	if s.sysDotnetOK == nil {
		s.sysDotnetOK = map[string]bool{}
	}
	s.sysDotnetOK[sysDotnetKey(installDir, req)] = true
}
