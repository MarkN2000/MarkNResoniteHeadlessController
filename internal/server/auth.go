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
// ⚠️ signingKey は cfg.SessionSecret / cfg.AdminPasswordHash を lock 無しで読む。
// 現状これらを実行時に書き換える経路は無い（パスワード再設定は別プロセスの CLI）。
// 将来 UI からのパスワード変更を実装する際は cfg アクセスの同期が必要。
type authManager struct {
	cfg       *config.Config
	mu        sync.Mutex
	failures  int       // 連続ログイン失敗回数
	lockUntil time.Time // ロックアウト解除時刻
}

func newAuthManager(cfg *config.Config) *authManager {
	return &authManager{cfg: cfg}
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
	return bcrypt.CompareHashAndPassword([]byte(a.cfg.AdminPasswordHash), []byte(pw)) == nil
}

// signingKey はトークン署名鍵を導出する。
// SessionSecret を鍵、adminPasswordHash をメッセージとした HMAC。
// → パスワード変更で adminPasswordHash が変わると署名鍵も変わり、既存トークンが全無効化される。
func (a *authManager) signingKey() []byte {
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	mac.Write([]byte(a.cfg.AdminPasswordHash))
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

func bearerToken(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, p) {
		return strings.TrimPrefix(h, p)
	}
	return ""
}
