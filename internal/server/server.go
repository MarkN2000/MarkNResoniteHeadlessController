// Package server はHTTP/SSE層。単一の公開APIをWeb UIもスクリプトも共用する。
// 認証は2経路（人間=stateless HMAC Cookie / スクリプト=Bearer パスワード）。
// 状態変更系（start/stop/command）は POST 限定。長時間操作（start/stop）は
// 即「受付」を返し、進捗・状態はSSEで配信する。
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/hlconfig"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

type Server struct {
	cfg       *config.Config
	cfgPath   string
	dataDir   string // cfgPath のディレクトリ（runtime-state / .run / 既定configDir の基点）
	configDir string // headless config 格納ディレクトリ（解決済み）
	driver    *headless.Driver
	worlds    headless.WorldsService
	auth      *authManager
	webFS     fs.FS
	resonite  *resonite.Client // Resonite 公開API（ユーザー検索）

	// credMu は cfg.HeadlessCredentials の更新(credentials PUT)と起動時読取の競合を防ぐ。
	// auth は別フィールド（SessionSecret/AdminPasswordHash）しか読まないため対象外。
	credMu sync.RWMutex
}

func New(cfg *config.Config, cfgPath string, driver *headless.Driver, reso *resonite.Client, webFS fs.FS) *Server {
	dataDir := ""
	if cfgPath != "" {
		dataDir = filepath.Dir(cfgPath)
	}
	return &Server{
		cfg:       cfg,
		cfgPath:   cfgPath,
		dataDir:   dataDir,
		configDir: cfg.HeadlessConfigDirOrDefault(dataDir),
		driver:    driver,
		worlds:    headless.NewWorldsService(driver),
		auth:      newAuthManager(cfg),
		webFS:     webFS,
		resonite:  reso,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 既存（プロセスライフサイクル・raw コマンド・SSE）
	mux.HandleFunc("POST /api/v1/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/v1/status", s.requireAuth(s.handleStatus))
	mux.HandleFunc("POST /api/v1/start", s.requireAuth(s.handleStart))     // 状態変更=POST限定
	mux.HandleFunc("POST /api/v1/stop", s.requireAuth(s.handleStop))       // 状態変更=POST限定
	mux.HandleFunc("POST /api/v1/command", s.requireAuth(s.handleCommand)) // 副作用あり=POST限定
	mux.HandleFunc("GET /api/v1/events", s.requireAuth(s.handleEvents))

	// 構造化API（Phase 4: Exec/WorldsService を介して構造化データを返す）
	mux.HandleFunc("GET /api/v1/sessions", s.requireAuth(s.handleSessions))
	mux.HandleFunc("GET /api/v1/sessions/{idx}/status", s.requireAuth(s.handleSessionStatus))
	mux.HandleFunc("GET /api/v1/sessions/{idx}/users", s.requireAuth(s.handleSessionUsers))
	mux.HandleFunc("GET /api/v1/sessions/{idx}/detail", s.requireAuth(s.handleSessionDetail))
	mux.HandleFunc("GET /api/v1/listbans", s.requireAuth(s.handleListBans))
	mux.HandleFunc("GET /api/v1/friendrequests", s.requireAuth(s.handleFriendRequests))

	// Headless Config CRUD（Pre-7b）。{name} ワイルドカードより literal の last-used が優先される。
	mux.HandleFunc("GET /api/v1/headless-configs", s.requireAuth(s.handleConfigList))
	mux.HandleFunc("GET /api/v1/headless-configs/last-used", s.requireAuth(s.handleConfigLastUsed))
	mux.HandleFunc("GET /api/v1/headless-configs/{name}", s.requireAuth(s.handleConfigGet))
	mux.HandleFunc("PUT /api/v1/headless-configs/{name}", s.requireAuth(s.handleConfigPut))
	mux.HandleFunc("DELETE /api/v1/headless-configs/{name}", s.requireAuth(s.handleConfigDelete))
	mux.HandleFunc("GET /api/v1/headless-credentials", s.requireAuth(s.handleCredentialsGet))
	mux.HandleFunc("PUT /api/v1/headless-credentials", s.requireAuth(s.handleCredentialsPut))

	// write API（Pre-7c）。全 POST・認証必須・idx は path・引数は JSON body。
	// セッション内ユーザー操作（focus idx → <cmd> "<user>"）
	mux.HandleFunc("POST /api/v1/sessions/{idx}/kick", s.requireAuth(s.sessionUserOp("kick")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/ban", s.requireAuth(s.sessionUserOp("ban")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/silence", s.requireAuth(s.sessionUserOp("silence")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/unsilence", s.requireAuth(s.sessionUserOp("unsilence")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/respawn", s.requireAuth(s.sessionUserOp("respawn")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/invite", s.requireAuth(s.sessionUserOp("invite")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/role", s.requireAuth(s.handleSessionRole))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/message", s.requireAuth(s.handleSessionMessage))
	// セッション設定（focus idx → <cmd>）
	mux.HandleFunc("POST /api/v1/sessions/{idx}/accesslevel", s.requireAuth(s.handleSessionAccessLevel))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/maxusers", s.requireAuth(s.handleSessionMaxUsers))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/name", s.requireAuth(s.handleSessionName))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/description", s.requireAuth(s.handleSessionDescription))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/hidefromlisting", s.requireAuth(s.handleSessionHideFromListing))
	// セッションライフサイクル（focus idx → <cmd>）
	mux.HandleFunc("POST /api/v1/sessions/{idx}/restart", s.requireAuth(s.sessionCmdOp("restart", headless.WithTimeout(restartTimeout))))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/save", s.requireAuth(s.sessionCmdOp("save", headless.WithTimeout(saveTimeout))))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/close", s.requireAuth(s.sessionCmdOp("close", headless.WithTimeout(closeTimeout))))
	// 新規セッション（focus 不要・長 timeout）。literal "start" は {idx} と段数が違うため衝突しない。
	mux.HandleFunc("POST /api/v1/sessions/start", s.requireAuth(s.handleSessionStart))
	// グローバル（フレンド/BAN・focus 不要）
	mux.HandleFunc("POST /api/v1/friendrequests/accept", s.requireAuth(s.globalUserOp("acceptfriendrequest")))
	mux.HandleFunc("POST /api/v1/friends/add", s.requireAuth(s.globalUserOp("sendFriendRequest")))
	mux.HandleFunc("POST /api/v1/friends/remove", s.requireAuth(s.globalUserOp("removeFriend")))
	mux.HandleFunc("POST /api/v1/bans/unban", s.requireAuth(s.handleBanUnban))

	// Resonite 公開API（ユーザー検索）。フレンド申請/招待の相手探しに使う（無認証プロキシ・P9-A）。
	mux.HandleFunc("GET /api/v1/resonite/users", s.requireAuth(s.handleResoniteUserSearch))

	// フロントエンド（埋め込み静的資産）。テストでは nil 渡しで未登録にできる。
	if s.webFS != nil {
		mux.Handle("/", http.FileServerFS(s.webFS))
	}
	return mux
}

// --- レスポンスヘルパ（統一形式 {ok, data} / {ok:false, error}） ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func writeErr(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": map[string]string{"code": errCode, "message": msg}})
}

// --- ハンドラ ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if locked, remain := s.auth.loginLocked(); locked {
		writeErr(w, http.StatusTooManyRequests, "rate_limited",
			fmt.Sprintf("ログイン試行が多すぎます。約%d秒後に再試行してください", int(remain.Seconds())+1))
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	ok := s.auth.checkPassword(body.Password)
	s.auth.recordLoginResult(ok)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid_password", "パスワードが違います")
		return
	}
	tok := s.auth.issueToken()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil, // HTTPS提供時のみ Secure（平文LAN httpでは付けない＝cookieが送られなくなるため）
		MaxAge:   int(s.cfg.SessionTTL().Seconds()),
	})
	writeOK(w, map[string]any{"loggedIn": true})
}

// handleLogout は Cookie をクリアする。stateless 設計のためサーバー側に
// 失効させる状態は無く、このブラウザの Cookie 削除のみ（他端末は失効しない）。
// 全端末の失効が必要なときはパスワード変更で署名鍵を変える。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeOK(w, map[string]any{"loggedIn": false})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeOK(w, s.driver.Status())
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Config string `json:"config"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	name := strings.TrimSpace(body.Config)
	if name == "" {
		// 無config起動はワールドが公開(Anyone)になり危険なため不可。
		writeErr(w, http.StatusBadRequest, "config_required",
			"起動するコンフィグ名を指定してください（無config起動はワールドが公開になるため不可）")
		return
	}
	if err := hlconfig.SanitizeName(name); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_config_name", err.Error())
		return
	}
	// 起動時注入: config の creds が空なら中央アカウントを注入した一時 config を生成。
	s.credMu.RLock()
	central := hlconfig.Credentials{
		Username: s.cfg.HeadlessCredentials.Username,
		Password: s.cfg.HeadlessCredentials.Password,
	}
	s.credMu.RUnlock()
	runDir := filepath.Join(s.dataDir, ".run")
	launchPath, err := hlconfig.ResolveForLaunch(s.configDir, name, central, runDir)
	if err != nil {
		if errors.Is(err, hlconfig.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "config_not_found", "指定のコンフィグが見つかりません")
			return
		}
		writeErr(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	if err := s.driver.Start(s.cfg.ResoniteHeadless, launchPath, name); err != nil {
		writeErr(w, http.StatusConflict, "start_failed", err.Error())
		return
	}
	s.recordLastUsed(name)
	writeOK(w, map[string]any{"accepted": true})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if err := s.driver.Stop(); err != nil {
		writeErr(w, http.StatusConflict, "stop_failed", err.Error())
		return
	}
	writeOK(w, map[string]any{"accepted": true})
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	// POST限定。cmd は URL query または JSON body で受理。
	// form-urlencoded body は対応外（必要なら呼び出し側で URL query を使う）。
	cmd := r.URL.Query().Get("cmd")
	if cmd == "" {
		var body struct {
			Cmd string `json:"cmd"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd = body.Cmd
	}
	if cmd == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "cmd がありません")
		return
	}
	if err := s.driver.SendCommand(cmd); err != nil {
		writeErr(w, http.StatusConflict, "command_failed", err.Error())
		return
	}
	writeOK(w, map[string]any{"sent": cmd})
}

// --- 構造化APIハンドラ（Phase 4） ---

// handleSessions: GET /api/v1/sessions → []World （worlds 一覧）
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	worlds, err := s.worlds.List(r.Context())
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, worlds)
}

// handleSessionStatus: GET /api/v1/sessions/{idx}/status → WorldStatus
// 内部で ExecGroup により focus → status を原子的に実行（他リクエストの割込防止）。
func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	idx, err := parseSessionIdx(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var got headless.WorldStatus
	err = s.driver.ExecGroup(r.Context(), func(tx headless.Tx) error {
		if _, e := tx.Exec(fmt.Sprintf("focus %d", idx)); e != nil {
			return e
		}
		lines, e := tx.Exec("status")
		if e != nil {
			return e
		}
		got = headless.ParseStatus(lines)
		return nil
	})
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, got)
}

// handleSessionUsers: GET /api/v1/sessions/{idx}/users → []UserInfo
func (s *Server) handleSessionUsers(w http.ResponseWriter, r *http.Request) {
	idx, err := parseSessionIdx(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var got []headless.UserInfo
	err = s.driver.ExecGroup(r.Context(), func(tx headless.Tx) error {
		if _, e := tx.Exec(fmt.Sprintf("focus %d", idx)); e != nil {
			return e
		}
		lines, e := tx.Exec("users")
		if e != nil {
			return e
		}
		got = headless.ParseUsers(lines)
		return nil
	})
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, got)
}

// sessionDetailResp は status と users を1回の取得で返すための封筒（B1）。
type sessionDetailResp struct {
	Status headless.WorldStatus `json:"status"`
	Users  []headless.UserInfo  `json:"users"`
}

// handleSessionDetail: GET /api/v1/sessions/{idx}/detail → {status, users}
// 1回の ExecGroup(focus → status → users) で取得し、focus 往復を1回に集約する。
// status と users が同一フォーカス時点のスナップショットになる（仕様 §3.4）。
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	idx, err := parseSessionIdx(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var resp sessionDetailResp
	err = s.driver.ExecGroup(r.Context(), func(tx headless.Tx) error {
		if _, e := tx.Exec(fmt.Sprintf("focus %d", idx)); e != nil {
			return e
		}
		statusLines, e := tx.Exec("status")
		if e != nil {
			return e
		}
		resp.Status = headless.ParseStatus(statusLines)
		userLines, e := tx.Exec("users")
		if e != nil {
			return e
		}
		resp.Users = headless.ParseUsers(userLines)
		return nil
	})
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, resp)
}

// handleListBans: GET /api/v1/listbans → []BanEntry （focus 不要・グローバル）
func (s *Server) handleListBans(w http.ResponseWriter, r *http.Request) {
	lines, err := s.driver.Exec(r.Context(), "listbans")
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, headless.ParseListBans(lines))
}

// handleFriendRequests: GET /api/v1/friendrequests → []string（focus 不要・グローバル）
// 注意: v1 互換の単純実装。boot 直後 ambient が多い時はノイズ混入可。
// 詳細: internal/headless/parser.go の ParseFriendRequests godoc 参照。
func (s *Server) handleFriendRequests(w http.ResponseWriter, r *http.Request) {
	lines, err := s.driver.Exec(r.Context(), "friendrequests")
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, headless.ParseFriendRequests(lines))
}

// handleResoniteUserSearch: GET /api/v1/resonite/users?q=<term> → []resonite.User
// Resonite 公開API（api.resonite.com/users）への無認証プロキシ。q が "U-" 始まりなら ID 検索、
// それ以外は名前検索。ヘッドレス稼働は不要（クラウドAPI直叩き）。
func (s *Server) handleResoniteUserSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "missing_query", "検索語 q は必須です")
		return
	}
	users, err := s.resonite.SearchUsers(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "resonite_api_error", err.Error())
		return
	}
	if users == nil {
		users = []resonite.User{} // null ではなく [] を返す
	}
	writeOK(w, users)
}

// parseSessionIdx は /api/v1/sessions/{idx}/... のパスパラメータを int に変換する。
func parseSessionIdx(r *http.Request) (int, error) {
	v := r.PathValue("idx")
	idx, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("不正なセッションindex: %q", v)
	}
	if idx < 0 {
		return 0, fmt.Errorf("セッションindexが負: %d", idx)
	}
	return idx, nil
}

// writeExecErr は headless パッケージのセンチネルエラーを HTTP ステータスにマップする。
// 2区分:
//   - ErrNotReady → 409 (UI は「起動してください」を出す)
//   - その他 (Timeout/ProcessGone/Canceled/内部エラー) → 500 (UI は「失敗、再試行」)
//
// クライアント側で原因の細かい区別は不要と判断（複雑化を避ける）。
// 必要なら error.code フィールドで内訳を返すので、UI 詳細表示は可能。
func writeExecErr(w http.ResponseWriter, err error) {
	if errors.Is(err, headless.ErrNotReady) {
		writeErr(w, http.StatusConflict, "not_ready", "ヘッドレスが起動中/停止中で構造化コマンドを受け付けられません")
		return
	}
	// その他は全て 500 + 詳細コードで区別可能に
	code := "exec_failed"
	switch {
	case errors.Is(err, headless.ErrTimeout):
		code = "timeout"
	case errors.Is(err, headless.ErrProcessGone):
		code = "process_gone"
	case errors.Is(err, headless.ErrCanceled):
		code = "canceled"
	}
	writeErr(w, http.StatusInternalServerError, code, err.Error())
}

// --- SSE ---

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	logCh, history := s.driver.SubscribeLog(256)
	defer s.driver.UnsubscribeLog(logCh)
	statusCh, cur := s.driver.SubscribeStatus(16)
	defer s.driver.UnsubscribeStatus(statusCh)

	writeSSE(w, "status", cur)
	for _, l := range history {
		writeSSE(w, "log", l)
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
			fmt.Fprint(w, ": ping\n\n") // keep-alive コメント
			fl.Flush()
		case l, ok := <-logCh:
			if !ok {
				return
			}
			writeSSE(w, "log", l)
			fl.Flush()
		case st, ok := <-statusCh:
			if !ok {
				return
			}
			writeSSE(w, "status", st)
			fl.Flush()
		}
	}
}

func writeSSE(w io.Writer, event string, data any) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}
