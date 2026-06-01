package server

// スケジュール（Phase 8・§3.16）バックエンド: 自動再起動の設定/状態。
// P8-1 はデータ構造 + 設定/状態API のみ。scheduler(P8-2)/orchestrator(P8-3)/crash-monitor(P8-4)
// は後続フェーズで追加し、restart-status の next/lastRestart/進行状態を埋めていく。
// cfg 書き換えは cfgMu 下・SaveTo 失敗はロールバック（settings.go と同方針）。

import (
	"encoding/json"
	"net/http"
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
	// configName は非空ならフォーマットのみ検証（実在は問わない＝後で削除され得るため・起動時に解決）。
	for _, sc := range body.Scheduled {
		if sc.ConfigName != "" {
			if err := hlconfig.SanitizeName(sc.ConfigName); err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", "予定の config 名が不正です: "+err.Error())
				return
			}
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
	writeOK(w, body)
}

// restartStatus は restart-status の応答。
// P8-1 では実行時状態（次回予定・最終再起動・進行フェーズ）は未実装のため idle/ゼロを返す。
// NextScheduled は P8-2（scheduler）、InProgress/Phase/LastRestart は P8-3（orchestrator）で埋める。
type restartStatus struct {
	Running              bool   `json:"running"`
	UptimeSeconds        int64  `json:"uptimeSeconds"`
	CrashRecoveryEnabled bool   `json:"crashRecoveryEnabled"`
	InProgress           bool   `json:"inProgress"`
	Phase                string `json:"phase"` // idle | waiting | announcing | restarting
}

// handleRestartStatus: GET /api/v1/restart-status
func (s *Server) handleRestartStatus(w http.ResponseWriter, r *http.Request) {
	st := s.driver.Status()
	s.cfgMu.RLock()
	crashOn := s.cfg.RestartOrDefault().CrashRecovery.Enabled
	s.cfgMu.RUnlock()
	out := restartStatus{
		Running:              st.State == headless.StateRunning,
		CrashRecoveryEnabled: crashOn,
		Phase:                "idle",
	}
	if st.StartedAt != nil {
		out.UptimeSeconds = int64(time.Since(*st.StartedAt).Seconds())
	}
	writeOK(w, out)
}
