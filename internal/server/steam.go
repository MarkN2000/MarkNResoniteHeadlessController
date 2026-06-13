package server

// Steam（DepotDownloader）による Resonite 入手/更新の HTTP/SSE 層（P9-B）。
// 実体の取得/実行/進行管理は internal/steam.Manager が担い、ここは薄い HTTP 層に留める。
// 秘密（Steam パスワード・branch コード）は返さず hasXxx(bool) で表す（headless-credentials と同じ流儀）。
// 設計: docs/design/steam-depotdownloader.md §6/§8

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/steam"
)

// steamConfigResp は GET/PUT /steam/config の公開表現（秘密は出さない）。
type steamConfigResp struct {
	Username      string `json:"username"`
	InstallDir    string `json:"installDir"`
	HasPassword   bool   `json:"hasPassword"`
	HasBranchCode bool   `json:"hasBranchCode"`
}

func steamConfigRespFrom(st *config.Steam) steamConfigResp {
	if st == nil {
		return steamConfigResp{}
	}
	return steamConfigResp{
		Username:      st.Username,
		InstallDir:    st.InstallDir,
		HasPassword:   st.Password != "",
		HasBranchCode: st.BranchCode != "",
	}
}

// handleSteamConfigGet: GET /api/v1/steam/config
func (s *Server) handleSteamConfigGet(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	resp := steamConfigRespFrom(s.cfg.Steam)
	s.cfgMu.RUnlock()
	writeOK(w, resp)
}

// handleSteamConfigPut: PUT /api/v1/steam/config {username, password, branchCode, installDir}
// password / branchCode は空なら既存維持（秘密を空で上書きしない）。保存失敗時は in-memory を巻き戻す。
func (s *Server) handleSteamConfigPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		BranchCode string `json:"branchCode"`
		InstallDir string `json:"installDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	if body.Password != "" {
		if err := steam.ValidatePassword(body.Password); err != nil {
			// 専用 code でフロントが locale 変換できるようにする（生メッセージは ja のため）。
			writeErr(w, http.StatusBadRequest, "steam_password_invalid", err.Error())
			return
		}
	}

	s.cfgMu.Lock()
	wasNil := s.cfg.Steam == nil
	if wasNil {
		s.cfg.Steam = &config.Steam{}
	}
	old := *s.cfg.Steam
	s.cfg.Steam.Username = strings.TrimSpace(body.Username)
	s.cfg.Steam.InstallDir = strings.TrimSpace(body.InstallDir)
	if body.Password != "" {
		s.cfg.Steam.Password = body.Password
	}
	if body.BranchCode != "" {
		s.cfg.Steam.BranchCode = strings.TrimSpace(body.BranchCode)
	}
	saveErr := s.cfg.SaveTo(s.cfgPath)
	if saveErr != nil {
		if wasNil {
			s.cfg.Steam = nil
		} else {
			*s.cfg.Steam = old
		}
	}
	resp := steamConfigRespFrom(s.cfg.Steam)
	s.cfgMu.Unlock()
	if saveErr != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", saveErr.Error())
		return
	}
	writeOK(w, resp)
}

// handleSteamDownload: POST /api/v1/steam/download
// 入手/更新を非同期開始する。更新は「停止中のみ」（稼働中は 409）＝統一原則（設計 §2）。
func (s *Server) handleSteamDownload(w http.ResponseWriter, r *http.Request) {
	if s.driver.Status().State != headless.StateStopped {
		writeErr(w, http.StatusConflict, "headless_running",
			"ヘッドレスが停止中のときのみ更新できます。先に停止してください")
		return
	}
	params, err := s.steamParams()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "steam_not_configured", err.Error())
		return
	}
	if err := s.steam.Start(params); err != nil {
		writeSteamErr(w, err)
		return
	}
	writeOK(w, map[string]any{"accepted": true})
}

// handleSteamCancel: POST /api/v1/steam/cancel
func (s *Server) handleSteamCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.steam.Cancel(); err != nil {
		writeSteamErr(w, err)
		return
	}
	writeOK(w, map[string]any{"accepted": true})
}

// handleSteamStatus: GET /api/v1/steam/status
func (s *Server) handleSteamStatus(w http.ResponseWriter, r *http.Request) {
	writeOK(w, s.steam.Status())
}

// handleSteamEvents: GET /api/v1/steam/events （SSE: steam-status/steam-progress/steam-log/steam-milestone/steam-result）
func (s *Server) handleSteamEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	ch, history := s.steam.Subscribe(256)
	defer s.steam.Unsubscribe(ch)

	writeSSE(w, "steam-status", s.steam.Status())
	for _, e := range history {
		writeSSE(w, steamEventName(e), e)
	}
	fl.Flush()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, steamEventName(e), e)
			fl.Flush()
		}
	}
}

// steamEventName は Event.Kind を SSE イベント名へ写す（progress→steam-progress 等）。
func steamEventName(e steam.Event) string {
	return "steam-" + e.Kind
}

// steamParams は config から更新パラメータを組む。InstallDir は常に config.InstallDirOrDefault で
// 解決する（明示 Steam.InstallDir→既定 {dataDir}/resonite）ため install 先が未設定でも欠けない。
// 利用時に "~" を展開する（R-A）。組み立て本体はウィザード S5 と共有の steam.BuildUpdateParams
// （資格のいずれかが欠ければ ErrSteamNotConfigured・VerifyRelPath の導出込み）。
func (s *Server) steamParams() (steam.UpdateParams, error) {
	s.cfgMu.RLock()
	var user, pw, code string
	if s.cfg.Steam != nil {
		user, pw, code = s.cfg.Steam.Username, s.cfg.Steam.Password, s.cfg.Steam.BranchCode
	}
	installDir := s.cfg.InstallDirOrDefault(s.dataDir)
	s.cfgMu.RUnlock()
	return steam.BuildUpdateParams(user, pw, code, platform.ExpandHome(installDir), platform.HeadlessBinaryName())
}

// maybeUpdate は起動/再起動の前に Resonite を更新する共通フック。triggerType と対応するトグル
// （scheduled→UpdateOnScheduledRestart / manual→UpdateBeforeManualStart）が ON かつ Steam 設定済みの
// ときだけ DepotDownloader を実行する（＝最新確認＋適用）。それ以外（stop・クラッシュ復帰など）は no-op。
// 更新の成否（失敗・キャンセル）は err で返し、起動を続けるか中止するかは呼び出し側が決める
// （予定/通常再起動は失敗でも起動継続＝可用性優先・設計 §7／コールド起動はキャンセル時のみ見送り）。
// onUpdating は「実際に更新を開始する」直前に1回だけ呼ぶ（進行表示の差し込み用・nil 可）。
// ctx は呼び出し側の親（shutdown で更新も中断）。同期的に完了まで待つ。
func (s *Server) maybeUpdate(ctx context.Context, triggerType string, onUpdating func()) error {
	s.cfgMu.RLock()
	rc := s.cfg.RestartOrDefault()
	s.cfgMu.RUnlock()
	var enabled bool
	switch triggerType {
	case "scheduled":
		enabled = rc.UpdateOnScheduledRestart
	case "manual":
		enabled = rc.UpdateBeforeManualStart
	default:
		return nil // stop・クラッシュ復帰などは対象外（素早さ優先で更新を挟まない）
	}
	if !enabled {
		return nil
	}
	params, err := s.steamParams()
	if err != nil {
		return nil // Steam 未設定 → 更新せず通常どおり起動を続行
	}
	if onUpdating != nil {
		onUpdating()
	}
	log.Printf("[update] %s: Resonite の更新を実行します", triggerType)
	if err := s.updateResonite(ctx, params); err != nil {
		log.Printf("[update] %s: 更新に失敗（古い版のまま）: %v", triggerType, err)
		return err
	}
	log.Printf("[update] %s: 更新が完了しました", triggerType)
	return nil
}

// beforeFlowStart は再起動フロー（予定／手動「通常再起動」）の停止後・起動前フック
// （orchestrator.beforeStart）。maybeUpdate を進行表示（phaseUpdating）付きで呼ぶ。更新の失敗・
// キャンセルがあっても起動は継続する（doRestart は本関数の後で必ず起動する・可用性優先）。
func (s *Server) beforeFlowStart(ctx context.Context, triggerType string) {
	_ = s.maybeUpdate(ctx, triggerType, func() { s.restart.setPhase(phaseUpdating) })
}

// writeSteamErr は steam パッケージのセンチネルエラーを HTTP ステータスへ写す。
func writeSteamErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, steam.ErrUpdateInProgress):
		writeErr(w, http.StatusConflict, "update_in_progress", err.Error())
	case errors.Is(err, steam.ErrNoUpdateInProgress):
		writeErr(w, http.StatusConflict, "no_update", err.Error())
	case errors.Is(err, steam.ErrSteamNotConfigured):
		writeErr(w, http.StatusBadRequest, "steam_not_configured", err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "steam_error", err.Error())
	}
}
