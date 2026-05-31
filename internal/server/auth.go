package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

const sessionCookie = "mrhc_session"

// tokenVersion はセッショントークンのフォーマット版（将来変更用）。
const tokenVersion = "v1"

// authManager は人間向けセッション（stateless HMAC Cookie）と、
// スクリプト向け Bearer パスワード認証を扱う。
//
// stateless 設計（サーバー側にセッション状態を持たない）:
//   - 署名鍵 = HMAC-SHA256(SessionSecret, adminPasswordHash) を導出。
//     パスワード変更で adminPasswordHash が変われば署名鍵も変わり、
//     既存トークンが全て無効化される（= 全端末ログアウト）。
//   - トークン = "v1.<expiryUnix>.<sig>"（絶対失効。既定30日）。
//   - 検証は署名再計算 + 定数時間比較 + 期限チェックのみ。
//
// cfg.SessionSecret / cfg.AdminPasswordHash の読みは cfgMu（Server と共有）で保護する。
// UI からのパスワード変更（POST /password）が AdminPasswordHash を書き換えるため、
// 署名鍵の読みと競合しないよう RLock を取る。レート制限状態は別ロック（mu）。
type authManager struct {
	cfg       *config.Config
	cfgMu     *sync.RWMutex // Server と共有。cfg.AdminPasswordHash/SessionSecret の読みを保護
	mu        sync.Mutex    // レート制限状態（failures/lockUntil）専用ロック
	failures  int           // 連続ログイン失敗回数
	lockUntil time.Time     // ロックアウト解除時刻
}

func newAuthManager(cfg *config.Config, cfgMu *sync.RWMutex) *authManager {
	return &authManager{cfg: cfg, cfgMu: cfgMu}
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

func (a *authManager) checkPassword(pw string) bool {
	a.cfgMu.RLock()
	hash := a.cfg.AdminPasswordHash
	a.cfgMu.RUnlock()
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// signingKey はトークン署名鍵を導出する。
// SessionSecret を鍵、adminPasswordHash をメッセージとした HMAC。
// → パスワード変更で adminPasswordHash が変わると署名鍵も変わり、既存トークンが全無効化される。
func (a *authManager) signingKey() []byte {
	a.cfgMu.RLock()
	secret, hash := a.cfg.SessionSecret, a.cfg.AdminPasswordHash
	a.cfgMu.RUnlock()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(hash))
	return mac.Sum(nil)
}

// sign は payload に対する base64url 署名（HMAC-SHA256）を返す。
func (a *authManager) sign(payload string) string {
	mac := hmac.New(sha256.New, a.signingKey())
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// issueToken は新しいセッショントークンを発行する（絶対失効 = now + SessionTTL）。
func (a *authManager) issueToken() string {
	exp := time.Now().Add(a.cfg.SessionTTL()).Unix()
	payload := tokenVersion + "." + strconv.FormatInt(exp, 10)
	return payload + "." + a.sign(payload)
}

// verifyToken はトークンの署名と有効期限を検証する（サーバー状態は参照しない）。
func (a *authManager) verifyToken(tok string) bool {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return false
	}
	ver, expStr, sig := parts[0], parts[1], parts[2]
	if ver != tokenVersion {
		return false
	}
	want := a.sign(ver + "." + expStr)
	// 定数時間比較（タイミング攻撃対策）
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

// authorized は Cookie セッション or Bearer パスワードで認証可否を返す。
func (s *Server) authorized(r *http.Request) bool {
	if c, err := r.Cookie(sessionCookie); err == nil && s.auth.verifyToken(c.Value) {
		return true
	}
	if pw := bearerToken(r); pw != "" {
		return s.auth.checkPassword(pw)
	}
	return false
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

// setSessionCookie は人間向けセッション Cookie を発行/更新する（login と password 変更で共用）。
// 平文 LAN(http) では Secure を付けない（付けると cookie が送られず認証できなくなるため）。
func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, tok string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		MaxAge:   int(s.cfg.SessionTTL().Seconds()),
	})
}

func bearerToken(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, p) {
		return strings.TrimPrefix(h, p)
	}
	return ""
}
