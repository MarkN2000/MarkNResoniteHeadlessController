package server

// Headless Config CRUD + 中央既定アカウント + last-used（Pre-7b）。
// ファイル操作は internal/hlconfig に委譲し、ここは薄い HTTP 層に留める。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/hlconfig"
)

// credResolveTimeout は認証情報保存時の UserID 解決（外部API）の上限（R12）。
// 解決失敗でも保存は続行するため、保存全体を長く待たせない短めの上限にする。
const credResolveTimeout = 5 * time.Second

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

// handleConfigPut: PUT /api/v1/headless-configs/{name}[?from={old}] → 保存（上書き/リネーム）
// from 指定（≠name）は「from の編集内容を name で保存し from を削除」＝保存リネーム。
// マスクされた loginPassword の解決先も from（通常保存は name 自身）。
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
	var err error
	if from := r.URL.Query().Get("from"); from != "" && from != name {
		err = hlconfig.WriteRenamed(s.configDir, from, name, body)
	} else {
		err = hlconfig.Write(s.configDir, name, body)
	}
	if err != nil {
		writeConfigErr(w, err)
		return
	}
	writeOK(w, map[string]any{"saved": name})
}

// handleConfigCreate: POST /api/v1/headless-configs → テンプレから即時作成 {name}
// 名前はサーバーが採番（new-config, new-config2, …）。新規/複製とも「押した瞬間に実体を作り、
// 保存＝上書き（名前変更時はリネーム）」の即時作成方式（複製は handleConfigDuplicate）。
func (s *Server) handleConfigCreate(w http.ResponseWriter, r *http.Request) {
	name, err := hlconfig.Create(s.configDir, s.dataDir)
	if err != nil {
		writeConfigErr(w, err)
		return
	}
	writeOK(w, map[string]any{"name": name})
}

// handleConfigDuplicate: POST /api/v1/headless-configs/{name}/duplicate → {name: 新名}
// サーバー側のバイトコピー（password を含む全内容がそのまま写る）。名前はサーバーが採番。
func (s *Server) handleConfigDuplicate(w http.ResponseWriter, r *http.Request) {
	newName, err := hlconfig.Duplicate(s.configDir, r.PathValue("name"))
	if err != nil {
		writeConfigErr(w, err)
		return
	}
	writeOK(w, map[string]any{"name": newName})
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

// handleConfigDefaults: GET /api/v1/headless-config-defaults → {dataFolder, cacheFolder}
// UI の新規 config 作成（フロント defaultConfig()）に入れる既定値。EnsureDefault（default.json
// 生成）と同じ導出＝単一情報源 hlconfig.DefaultFolders（UI改善⑤）。
// パスは /headless-configs/{name} と衝突させない（"defaults" は有効な config 名のため別パス）。
func (s *Server) handleConfigDefaults(w http.ResponseWriter, r *http.Request) {
	dataFolder, cacheFolder, err := hlconfig.DefaultFolders(s.dataDir)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	writeOK(w, map[string]any{"dataFolder": dataFolder, "cacheFolder": cacheFolder})
}

// writeConfigErr は hlconfig のセンチネルエラーを HTTP ステータスにマップする。
func writeConfigErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hlconfig.ErrNotFound):
		writeErr(w, http.StatusNotFound, "config_not_found", "指定のコンフィグが見つかりません")
	case errors.Is(err, hlconfig.ErrInvalidName), errors.Is(err, hlconfig.ErrReservedName):
		writeErr(w, http.StatusBadRequest, "invalid_config_name", err.Error())
	case errors.Is(err, hlconfig.ErrStartWorldsType), errors.Is(err, hlconfig.ErrInvalidJSON):
		writeErr(w, http.StatusBadRequest, "invalid_config", err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "config_error", err.Error())
	}
}

// --- 中央既定アカウント（mrhc.config.json の HeadlessCredentials）---

// handleCredentialsGet: GET /api/v1/headless-credentials → {username, hasPassword, userId}（password 非返却）
func (s *Server) handleCredentialsGet(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	writeOK(w, map[string]any{
		"username":    s.cfg.HeadlessCredentials.Username,
		"hasPassword": s.cfg.HeadlessCredentials.Password != "",
		"userId":      s.cfg.HeadlessCredentials.UserID, // 解決済 UserID（空=未解決・R12）
	})
}

// handleCredentialsPut: PUT /api/v1/headless-credentials {username, password}
// password 空なら既存を保持（username のみ更新）。mrhc.config.json に保存。
// 保存時に username → UserID を解決して併せて保持する（R12・customSessionId prefix 等で再利用）。
func (s *Server) handleCredentialsPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}

	// 現在値を読み、UserID 再解決の要否を決める（cfgMu 外でネットワークを叩くための事前判定）。
	s.cfgMu.RLock()
	prevU, prevID := s.cfg.HeadlessCredentials.Username, s.cfg.HeadlessCredentials.UserID
	s.cfgMu.RUnlock()
	newUserID := s.resolveOwnerUserID(r.Context(), body.Username, prevU, prevID)

	s.cfgMu.Lock()
	// 変更前を退避し、SaveTo 失敗時は in-memory を巻き戻してディスクとの整合を保つ。
	oldU, oldP, oldID := s.cfg.HeadlessCredentials.Username, s.cfg.HeadlessCredentials.Password, s.cfg.HeadlessCredentials.UserID
	s.cfg.HeadlessCredentials.Username = body.Username
	if body.Password != "" {
		s.cfg.HeadlessCredentials.Password = body.Password
	}
	s.cfg.HeadlessCredentials.UserID = newUserID
	saveErr := s.cfg.SaveTo(s.cfgPath)
	if saveErr != nil {
		s.cfg.HeadlessCredentials.Username = oldU
		s.cfg.HeadlessCredentials.Password = oldP
		s.cfg.HeadlessCredentials.UserID = oldID
	}
	uname := s.cfg.HeadlessCredentials.Username
	hasPw := s.cfg.HeadlessCredentials.Password != ""
	uid := s.cfg.HeadlessCredentials.UserID
	s.cfgMu.Unlock()
	if saveErr != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", saveErr.Error())
		return
	}
	writeOK(w, map[string]any{"username": uname, "hasPassword": hasPw, "userId": uid})
}

// resolveOwnerUserID は中央アカウント username から Resonite UserID を解決する（R12・cfgMu 外で呼ぶ）。
//   - username が空 / メール形式（"@" を含む）→ "" （解決対象外）。
//   - username が前回と不変かつ前回 UserID があれば再解決せず流用（保存のたびの無駄打ち回避）。
//   - それ以外は公開API で解決。失敗（ネットワーク/未一致）は "" を返し、保存自体は続行させる。
func (s *Server) resolveOwnerUserID(ctx context.Context, newUsername, prevUsername, prevUserID string) string {
	u := strings.TrimSpace(newUsername)
	if u == "" || strings.Contains(u, "@") {
		return ""
	}
	if u == strings.TrimSpace(prevUsername) && prevUserID != "" {
		return prevUserID
	}
	if s.resonite == nil {
		return ""
	}
	rctx, cancel := context.WithTimeout(ctx, credResolveTimeout)
	defer cancel()
	id, err := s.resonite.ResolveUserID(rctx, u)
	if err != nil {
		return "" // 解決失敗でも保存は続行（UserID 空＝prefix は手入力フォールバック）
	}
	return id
}

// --- runtime-state（last-used config / 最終起動）---
// 複数フィールドを持つため read-modify-write で保全する（recordLastUsed が lastStart を消さない）。
// orchestrator/crash-monitor（goroutine）と handleStart（HTTP）から並行に書かれるため runtimeMu で直列化。

type runtimeState struct {
	LastUsedConfig   string `json:"lastUsedConfig"`
	LastStartAt      string `json:"lastStartAt,omitempty"`      // RFC3339・最終起動時刻（§3.16(9)・手動起動/再起動/予定/crash 共通）
	LastStartTrigger string `json:"lastStartTrigger,omitempty"` // manual | scheduled | crash
}

func (s *Server) runtimeStatePath() string {
	return filepath.Join(s.dataDir, "runtime-state.json")
}

// loadRuntimeStateLocked / saveRuntimeStateLocked は runtimeMu 保持前提の素の read/write。
// 実体は jsonstate.go の汎用ヘルパ（favorites と共有）。dataDir 未設定（テスト等）は no-op。
func (s *Server) loadRuntimeStateLocked() runtimeState {
	if s.dataDir == "" {
		return runtimeState{}
	}
	return readJSONFile[runtimeState](s.runtimeStatePath())
}

func (s *Server) saveRuntimeStateLocked(st runtimeState) {
	if s.dataDir == "" {
		return
	}
	writeJSONFile(s.runtimeStatePath(), st)
}

func (s *Server) loadLastUsed() string {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	return s.loadRuntimeStateLocked().LastUsedConfig
}

func (s *Server) recordLastUsed(name string) {
	// 保存名は NFC 正準形にそろえる（ドロップダウンの選択肢=List も正準形のため、選択が一致する）。
	// handleStart/orchestrator/crash-monitor のどの経路から来ても、ここで一括正規化する。
	name = hlconfig.NormalizeName(name)
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	st := s.loadRuntimeStateLocked()
	st.LastUsedConfig = name
	s.saveRuntimeStateLocked(st)
}

// recordLastStart は最終起動の時刻/トリガー種別を記録する（§3.16(9)・手動起動=handleStart /
// 手動再起動・予定=orchestrator / crash=crash-monitor から）。
func (s *Server) recordLastStart(trigger, at string) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	st := s.loadRuntimeStateLocked()
	st.LastStartAt = at
	st.LastStartTrigger = trigger
	s.saveRuntimeStateLocked(st)
}

func (s *Server) loadLastStart() (at, trigger string) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	st := s.loadRuntimeStateLocked()
	return st.LastStartAt, st.LastStartTrigger
}
