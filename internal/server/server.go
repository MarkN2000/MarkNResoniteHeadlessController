// Package server はHTTP/SSE層。単一の公開APIをWeb UIもスクリプトも共用する。
// 認証は2経路（人間=Cookie+SameSite / スクリプト=APIキー）。長時間操作
// （start/stop）は即「受付」を返し、進捗・状態はSSEで配信する。
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
)

type Server struct {
	cfg     *config.Config
	cfgPath string
	driver  *headless.Driver
	auth    *authManager
	webFS   fs.FS
}

func New(cfg *config.Config, cfgPath string, driver *headless.Driver, webFS fs.FS) *Server {
	return &Server{
		cfg:     cfg,
		cfgPath: cfgPath,
		driver:  driver,
		auth:    newAuthManager(cfg),
		webFS:   webFS,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/v1/status", s.requireAuth(s.handleStatus))
	mux.HandleFunc("POST /api/v1/start", s.requireAuth(s.handleStart))
	mux.HandleFunc("POST /api/v1/stop", s.requireAuth(s.handleStop))
	mux.HandleFunc("/api/v1/command", s.requireAuth(s.handleCommand)) // GET/POST両対応
	mux.HandleFunc("GET /api/v1/events", s.requireAuth(s.handleEvents))

	// フロントエンド（埋め込み静的資産）
	mux.Handle("/", http.FileServerFS(s.webFS))
	return mux
}

func (s *Server) ListenAndServe() error {
	addr := ":" + strconv.Itoa(s.cfg.Port)
	log.Printf("MRHC listening on %s", addr)
	return http.ListenAndServe(addr, s.Handler())
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
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	if !s.auth.checkPassword(body.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid_password", "パスワードが違います")
		return
	}
	tok := s.auth.newSession()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeOK(w, map[string]any{"loggedIn": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.auth.dropSession(c.Value)
	}
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
	if err := s.driver.Start(s.cfg.ResoniteHeadless, body.Config); err != nil {
		writeErr(w, http.StatusConflict, "start_failed", err.Error())
		return
	}
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
	cmd := r.URL.Query().Get("cmd")
	if cmd == "" && r.Method == http.MethodPost {
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

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
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
