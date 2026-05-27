package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

const sessionCookie = "mrhc_session"
const sessionTTL = 24 * time.Hour

// authManager は人間向けのセッション（Cookie）と、スクリプト向けのAPIキー認証を扱う。
type authManager struct {
	cfg       *config.Config
	mu        sync.Mutex
	sessions  map[string]time.Time // token -> 失効時刻
	failures  int                  // 連続ログイン失敗回数
	lockUntil time.Time            // ロックアウト解除時刻
}

// loginLocked はログインがレート制限でロック中かを返す。
func (a *authManager) loginLocked() (bool, time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Now().Before(a.lockUntil) {
		return true, time.Until(a.lockUntil)
	}
	return false, 0
}

// recordLoginResult はログイン結果を記録し、連続失敗が一定数を超えたら短時間ロックする。
func (a *authManager) recordLoginResult(ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ok {
		a.failures = 0
		a.lockUntil = time.Time{}
		return
	}
	a.failures++
	if a.failures >= 10 {
		a.lockUntil = time.Now().Add(time.Minute)
		a.failures = 0
	}
}

func newAuthManager(cfg *config.Config) *authManager {
	return &authManager{cfg: cfg, sessions: make(map[string]time.Time)}
}

func (a *authManager) checkPassword(pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(a.cfg.AdminPasswordHash), []byte(pw)) == nil
}

func (a *authManager) newSession() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	a.mu.Lock()
	a.sessions[tok] = time.Now().Add(sessionTTL)
	a.mu.Unlock()
	return tok
}

func (a *authManager) validSession(tok string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[tok]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(a.sessions, tok)
		return false
	}
	return true
}

func (a *authManager) dropSession(tok string) {
	a.mu.Lock()
	delete(a.sessions, tok)
	a.mu.Unlock()
}

func (a *authManager) validAPIKey(key string) bool {
	if key == "" || a.cfg.APIKey == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(key), []byte(a.cfg.APIKey)) == 1
}

// authorized は Cookieセッション or APIキー（ヘッダ優先・クエリも許容）で認証可否を返す。
func (s *Server) authorized(r *http.Request) bool {
	if c, err := r.Cookie(sessionCookie); err == nil && s.auth.validSession(c.Value) {
		return true
	}
	key := bearerToken(r)
	if key == "" {
		key = r.URL.Query().Get("apiKey")
	}
	return s.auth.validAPIKey(key)
}

func (s *Server) requireAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "認証が必要です")
			return
		}
		h(w, r)
	}
}

func bearerToken(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, p) {
		return strings.TrimPrefix(h, p)
	}
	return ""
}
