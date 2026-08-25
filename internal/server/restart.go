package server

// スケジュール（Phase 8・§3.16）バックエンド: 自動再起動の設定/状態。
// P8-1 はデータ構造 + 設定/状態API のみ。scheduler(P8-2)/orchestrator(P8-3)/crash-monitor(P8-4)
// は後続フェーズで追加し、restart-status の next/lastStart/進行状態を埋めていく。
// cfg 書き換えは cfgMu 下・SaveTo 失敗はロールバック（settings.go と同方針）。

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/hlconfig"
)

// handleRestartConfigGet: GET /api/v1/restart-config
// 未設定なら DefaultRestart を返す（フロントは常に完全な restart オブジェクトを編集する）。
func (s *Server) handleRestartConfigGet(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	rc := s.cfg.RestartOrDefault()
	s.cfgMu.RUnlock()
	writeOK(w, rc)
}

// handleRestartConfigPut: PUT /api/v1/restart-config
// 完成形の restart を受け取り、範囲/enum/条件付き必須を検証して保存（upsert・ロールバック付き）。
func (s *Server) handleRestartConfigPut(w http.ResponseWriter, r *http.Request) {
	var body config.Restart
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	if body.Scheduled == nil {
		body.Scheduled = []config.ScheduledRestart{} // null→[] に正規化（保存JSONを安定させる）
	}
	if err := body.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	// テンプレ参照の告知は templateId の実在を検証（有効時のみ＝テンプレが消えても無効化保存は妨げない）。
	// リスト取得不能時もビルトインまで連鎖するため、オフラインでも既定テンプレは通る。
	if an := body.PreActions.Announce; an.Enabled {
		var tpl *itemTemplate
		if an.TemplateID != "" {
			found, ok := s.itemTpl.lookup(r.Context(), an.TemplateID, templateActionAnnounce)
			if !ok {
				writeErr(w, http.StatusBadRequest, "bad_request", "告知テンプレートが見つかりません: "+an.TemplateID)
				return
			}
			tpl = &found
		}
		if _, err := impulseValueForTemplate(tpl, an.Message, an.SpeakerID); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
	}
	// configName は非空ならフォーマットのみ検証（実在は問わない＝後で削除され得るため・起動時に解決）。
	// 検証後は NFC 正準形へそろえて保存する（発火時にディスク上の正準名と一致させる）。
	for i := range body.Scheduled {
		if body.Scheduled[i].ConfigName != "" {
			if err := hlconfig.SanitizeName(body.Scheduled[i].ConfigName); err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", "予定の config 名が不正です: "+err.Error())
				return
			}
			body.Scheduled[i].ConfigName = hlconfig.NormalizeName(body.Scheduled[i].ConfigName)
		}
	}
	s.cfgMu.Lock()
	old := s.cfg.Restart
	rc := body
	s.cfg.Restart = &rc
	saveErr := s.cfg.SaveTo(s.cfgPath)
	if saveErr != nil {
		s.cfg.Restart = old
	}
	s.cfgMu.Unlock()
	if saveErr != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", saveErr.Error())
		return
	}
	s.scheduler.Reload() // 予定変更を scheduler に即反映（次回発火を再計算）
	writeOK(w, body)
}

// restartStatus は restart-status の応答。
// 次回予定（NextScheduled*）は P8-2（scheduler）。進行状態（InProgress/Phase/Restart*）は
// P8-3（orchestrator）。最終起動（LastStart）は永続化（P8-4・§3.16(9)）で埋める。
type restartStatus struct {
	Running              bool   `json:"running"`
	UptimeSeconds        int64  `json:"uptimeSeconds"`
	CrashRecoveryEnabled bool   `json:"crashRecoveryEnabled"`
	InProgress           bool   `json:"inProgress"`
	Phase                string `json:"phase"` // idle | preparing | waiting | announcing | restarting
	// 進行中の再起動（InProgress=true のときのみ意味を持つ）。
	RestartTriggerType string  `json:"restartTriggerType,omitempty"` // manual | scheduled | stop
	RestartConfigName  string  `json:"restartConfigName,omitempty"`  // 進行中の対象 config（空=前回）
	DeadlineAt         *string `json:"deadlineAt"`                   // ② 待機の締切（RFC3339）/ null
	// 最終起動（§3.16(9)・runtime-state.json 由来。手動起動/再起動/予定/crash 共通）。未記録なら null/空。
	LastStartAt      *string `json:"lastStartAt"`                // RFC3339 / null
	LastStartTrigger string  `json:"lastStartTrigger,omitempty"` // manual | scheduled | crash
	// 次回予定再起動（有効予定の最も近い発火）。予定が無ければ全て null/空。
	NextScheduledAt         *string `json:"nextScheduledAt"`         // RFC3339（サーバーローカルTZ）/ null=予定なし
	NextScheduledConfigName string  `json:"nextScheduledConfigName"` // 空=前回config
	NextScheduledID         string  `json:"nextScheduledId"`
	NextScheduledType       string  `json:"nextScheduledType"` // once | weekly | daily
}

// handleRestartStatus: GET /api/v1/restart-status
func (s *Server) handleRestartStatus(w http.ResponseWriter, r *http.Request) {
	st := s.driver.Status()
	s.cfgMu.RLock()
	rc := s.cfg.RestartOrDefault()
	s.cfgMu.RUnlock()
	out := restartStatus{
		Running:              st.State == headless.StateRunning,
		CrashRecoveryEnabled: rc.CrashRecovery.Enabled,
		Phase:                phaseIdle,
	}
	if st.StartedAt != nil {
		out.UptimeSeconds = int64(time.Since(*st.StartedAt).Seconds())
	}
	// 進行中の再起動（P8-3b）。
	if snap := s.restart.snapshot(); snap.inProgress {
		out.InProgress = true
		out.Phase = snap.phase
		out.RestartTriggerType = snap.triggerType
		out.RestartConfigName = snap.configName
		if !snap.deadlineAt.IsZero() {
			d := snap.deadlineAt.Format(time.RFC3339)
			out.DeadlineAt = &d
		}
	}
	// 最終起動（P8-5・§3.16(9)・手動起動を含む）。
	if at, trigger := s.loadLastStart(); at != "" {
		a := at
		out.LastStartAt = &a
		out.LastStartTrigger = trigger
	}
	// 次回予定再起動（P8-2）。
	if next, sc, ok := rc.NextScheduled(time.Now()); ok {
		at := next.Format(time.RFC3339)
		out.NextScheduledAt = &at
		out.NextScheduledConfigName = sc.ConfigName
		out.NextScheduledID = sc.ID
		out.NextScheduledType = sc.Type
	}
	writeOK(w, out)
}

// handleRestartTrigger: POST /api/v1/restart/trigger {configName?}
// 手動「通常再起動」を即受付（非同期）。configName 空＝前回 config（§3.16(1)）。
func (s *Server) handleRestartTrigger(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConfigName string `json:"configName"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	name := strings.TrimSpace(body.ConfigName)
	if name != "" {
		if err := hlconfig.SanitizeName(name); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_config_name", err.Error())
			return
		}
		name = hlconfig.NormalizeName(name) // 空（=前回config）はそのまま・非空は正準形へ
	}
	if err := s.restart.Trigger("manual", name); err != nil {
		switch {
		case errors.Is(err, errRestartInProgress):
			writeErr(w, http.StatusConflict, "restart_in_progress", err.Error())
		case errors.Is(err, errRestartNotRunning):
			writeErr(w, http.StatusConflict, "not_running", err.Error())
		case errors.Is(err, errRestartNoConfig):
			writeErr(w, http.StatusBadRequest, "config_required", err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "trigger_failed", err.Error())
		}
		return
	}
	writeOK(w, map[string]any{"accepted": true})
}

// handleRestartCancel: POST /api/v1/restart/cancel
// 進行中の再起動を中止（①②③のみ・ヘッドレスは継続）。④以降は 409。
func (s *Server) handleRestartCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.restart.Cancel(); err != nil {
		switch {
		case errors.Is(err, errNoRestartInProgress):
			writeErr(w, http.StatusConflict, "no_restart", err.Error())
		case errors.Is(err, errRestartNotCancellable):
			writeErr(w, http.StatusConflict, "not_cancellable", err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "cancel_failed", err.Error())
		}
		return
	}
	writeOK(w, map[string]any{"accepted": true})
}

// handleGracefulStop: POST /api/v1/stop/graceful
// 「通常停止」を即受付（非同期・R7）。事前アクション→固定1分の猶予→停止（起動しない）。
// orchestrator の統一フローを終端=停止で流用。進行/中止は restart-status・restart/cancel と共通。
func (s *Server) handleGracefulStop(w http.ResponseWriter, r *http.Request) {
	if err := s.restart.TriggerStop(); err != nil {
		switch {
		case errors.Is(err, errRestartInProgress):
			writeErr(w, http.StatusConflict, "restart_in_progress", err.Error())
		case errors.Is(err, errRestartNotRunning):
			writeErr(w, http.StatusConflict, "not_running", err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "trigger_failed", err.Error())
		}
		return
	}
	writeOK(w, map[string]any{"accepted": true})
}
