package server

// Headless Config CRUD + 中央既定アカウント + last-used（Pre-7b）。
// ファイル操作は internal/hlconfig に委譲し、ここは薄い HTTP 層に留める。

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/hlconfig"
)

// --- Headless Config CRUD ---

// handleConfigList: GET /api/v1/headless-configs → []Summary
func (s *Server) handleConfigList(w http.ResponseWriter, r *http.Request) {
	list, err := hlconfig.List(s.configDir)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	writeOK(w, list)
}

// handleConfigGet: GET /api/v1/headless-configs/{name} → config（loginPassword マスク）
func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	m, err := hlconfig.ReadMasked(s.configDir, name)
	if err != nil {
		writeConfigErr(w, err)
		return
	}
	writeOK(w, m)
}

// handleConfigPut: PUT /api/v1/headless-configs/{name} → 保存（新規/上書き）
func (s *Server) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := hlconfig.SanitizeName(name); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_config_name", err.Error())
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正な JSON")
		return
	}
	if err := hlconfig.Write(s.configDir, name, body); err != nil {
		writeConfigErr(w, err)
		return
	}
	writeOK(w, map[string]any{"saved": name})
}

// handleConfigDelete: DELETE /api/v1/headless-configs/{name}
func (s *Server) handleConfigDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := hlconfig.Delete(s.configDir, name); err != nil {
		writeConfigErr(w, err)
		return
	}
	writeOK(w, map[string]any{"deleted": name})
}

// handleConfigLastUsed: GET /api/v1/headless-configs/last-used → {lastUsed}
func (s *Server) handleConfigLastUsed(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]any{"lastUsed": s.loadLastUsed()})
}

// writeConfigErr は hlconfig のセンチネルエラーを HTTP ステータスにマップする。
func writeConfigErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hlconfig.ErrNotFound):
		writeErr(w, http.StatusNotFound, "config_not_found", "指定のコンフィグが見つかりません")
	case errors.Is(err, hlconfig.ErrInvalidName):
		writeErr(w, http.StatusBadRequest, "invalid_config_name", err.Error())
	case errors.Is(err, hlconfig.ErrStartWorldsType), errors.Is(err, hlconfig.ErrInvalidJSON):
		writeErr(w, http.StatusBadRequest, "invalid_config", err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "config_error", err.Error())
	}
}

// --- 中央既定アカウント（mrhc.config.json の HeadlessCredentials）---

// handleCredentialsGet: GET /api/v1/headless-credentials → {username, hasPassword}（password 非返却）
func (s *Server) handleCredentialsGet(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	writeOK(w, map[string]any{
		"username":    s.cfg.HeadlessCredentials.Username,
		"hasPassword": s.cfg.HeadlessCredentials.Password != "",
	})
}

// handleCredentialsPut: PUT /api/v1/headless-credentials {username, password}
// password 空なら既存を保持（username のみ更新）。mrhc.config.json に保存。
func (s *Server) handleCredentialsPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	s.cfgMu.Lock()
	// 変更前を退避し、SaveTo 失敗時は in-memory を巻き戻してディスクとの整合を保つ。
	oldU, oldP := s.cfg.HeadlessCredentials.Username, s.cfg.HeadlessCredentials.Password
	s.cfg.HeadlessCredentials.Username = body.Username
	if body.Password != "" {
		s.cfg.HeadlessCredentials.Password = body.Password
	}
	saveErr := s.cfg.SaveTo(s.cfgPath)
	if saveErr != nil {
		s.cfg.HeadlessCredentials.Username = oldU
		s.cfg.HeadlessCredentials.Password = oldP
	}
	uname := s.cfg.HeadlessCredentials.Username
	hasPw := s.cfg.HeadlessCredentials.Password != ""
	s.cfgMu.Unlock()
	if saveErr != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", saveErr.Error())
		return
	}
	writeOK(w, map[string]any{"username": uname, "hasPassword": hasPw})
}

// --- runtime-state（last-used config / 最終再起動）---
// 複数フィールドを持つため read-modify-write で保全する（recordLastUsed が lastRestart を消さない）。
// orchestrator/crash-monitor（goroutine）と handleStart（HTTP）から並行に書かれるため runtimeMu で直列化。

type runtimeState struct {
	LastUsedConfig     string `json:"lastUsedConfig"`
	LastRestartAt      string `json:"lastRestartAt,omitempty"`      // RFC3339・最終再起動時刻（§3.16(9)）
	LastRestartTrigger string `json:"lastRestartTrigger,omitempty"` // manual | scheduled | crash
}

func (s *Server) runtimeStatePath() string {
	return filepath.Join(s.dataDir, "runtime-state.json")
}

// loadRuntimeStateLocked / saveRuntimeStateLocked は runtimeMu 保持前提の素の read/write。
func (s *Server) loadRuntimeStateLocked() runtimeState {
	var st runtimeState
	if s.dataDir == "" {
		return st
	}
	b, err := os.ReadFile(s.runtimeStatePath())
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	return st
}

func (s *Server) saveRuntimeStateLocked(st runtimeState) {
	if s.dataDir == "" {
		return
	}
	if b, err := json.MarshalIndent(st, "", "  "); err == nil {
		_ = os.WriteFile(s.runtimeStatePath(), b, 0o600)
	}
}

func (s *Server) loadLastUsed() string {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	return s.loadRuntimeStateLocked().LastUsedConfig
}

func (s *Server) recordLastUsed(name string) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	st := s.loadRuntimeStateLocked()
	st.LastUsedConfig = name
	s.saveRuntimeStateLocked(st)
}

// recordLastRestart は最終再起動の時刻/トリガー種別を記録する（§3.16(9)・orchestrator/crash-monitor から）。
func (s *Server) recordLastRestart(trigger, at string) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	st := s.loadRuntimeStateLocked()
	st.LastRestartAt = at
	st.LastRestartTrigger = trigger
	s.saveRuntimeStateLocked(st)
}

func (s *Server) loadLastRestart() (at, trigger string) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	st := s.loadRuntimeStateLocked()
	return st.LastRestartAt, st.LastRestartTrigger
}
