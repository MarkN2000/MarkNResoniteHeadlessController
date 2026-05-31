package server

// 設定タブ（7-5）のバックエンド: 管理パスワード変更 + アプリ設定 CRUD。
// 中央 Resonite アカウント（headless-credentials）は configs.go・headless config CRUD も configs.go。
// cfg の書き換えは全て cfgMu 下で行い、失敗時は in-memory をロールバックしてディスクと整合させる。

import (
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// handlePasswordChange: POST /api/v1/password {currentPassword, newPassword}
// 現PWを検証→新ハッシュを保存→新Cookieを再発行。adminPasswordHash が変わるため署名鍵も変わり、
// 既存トークンは全て無効化される（＝他端末はログアウト）。操作中ブラウザは再発行Cookieで継続する。
func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	if strings.TrimSpace(body.NewPassword) == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "新しいパスワードを入力してください")
		return
	}
	// 現PW検証（checkPassword は cfgMu RLock。この後の Lock とは順次なので nested にならない）。
	if !s.auth.checkPassword(body.CurrentPassword) {
		writeErr(w, http.StatusUnauthorized, "invalid_password", "現在のパスワードが違います")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash_failed", err.Error())
		return
	}
	s.cfgMu.Lock()
	old := s.cfg.AdminPasswordHash
	s.cfg.AdminPasswordHash = string(hash)
	saveErr := s.cfg.SaveTo(s.cfgPath)
	if saveErr != nil {
		s.cfg.AdminPasswordHash = old
	}
	s.cfgMu.Unlock()
	if saveErr != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", saveErr.Error())
		return
	}
	// 新ハッシュで署名された新トークンを発行（issueToken は cfgMu RLock。Unlock 済みなので安全）。
	s.setSessionCookie(w, r, s.auth.issueToken())
	writeOK(w, map[string]any{"changed": true})
}

// appSettings は GET/PUT で扱うアプリ設定の公開サブセット（秘密・encoding は含めない）。
type appSettings struct {
	Port              int    `json:"port"`
	ResoniteHeadless  string `json:"resoniteHeadlessPath"`
	HeadlessConfigDir string `json:"headlessConfigDir"`
}

// handleAppSettingsGet: GET /api/v1/app-settings
func (s *Server) handleAppSettingsGet(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	out := appSettings{
		Port:              s.cfg.Port,
		ResoniteHeadless:  s.cfg.ResoniteHeadless,
		HeadlessConfigDir: s.cfg.HeadlessConfigDir,
	}
	s.cfgMu.RUnlock()
	writeOK(w, out)
}

// handleAppSettingsPut: PUT /api/v1/app-settings {port, resoniteHeadlessPath, headlessConfigDir}
// port/headlessConfigDir は MRHC 再起動後に反映（ポートバインド・configDir 解決は起動時）。
// resoniteHeadlessPath は次回ヘッドレス起動で反映（handleStart が cfg をライブ参照）。
func (s *Server) handleAppSettingsPut(w http.ResponseWriter, r *http.Request) {
	var body appSettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	if body.Port < 1 || body.Port > 65535 {
		writeErr(w, http.StatusBadRequest, "bad_request", "ポートは 1〜65535 で指定してください")
		return
	}
	path := strings.TrimSpace(body.ResoniteHeadless)
	dir := strings.TrimSpace(body.HeadlessConfigDir)
	s.cfgMu.Lock()
	oldPort, oldPath, oldDir := s.cfg.Port, s.cfg.ResoniteHeadless, s.cfg.HeadlessConfigDir
	s.cfg.Port = body.Port
	s.cfg.ResoniteHeadless = path
	s.cfg.HeadlessConfigDir = dir
	saveErr := s.cfg.SaveTo(s.cfgPath)
	if saveErr != nil {
		s.cfg.Port, s.cfg.ResoniteHeadless, s.cfg.HeadlessConfigDir = oldPort, oldPath, oldDir
	}
	s.cfgMu.Unlock()
	if saveErr != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", saveErr.Error())
		return
	}
	writeOK(w, appSettings{Port: body.Port, ResoniteHeadless: path, HeadlessConfigDir: dir})
}
