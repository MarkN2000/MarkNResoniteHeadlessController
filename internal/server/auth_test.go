package server

// authManager の unit テスト。stateless HMAC トークン + Bearer パスワード認証を検証する。
// 実プロセス不要・httptest 不要。

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

func newTestAuth(password string) *authManager {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	// cfgMu はテストでは単独の RWMutex（並行書き換えは無いのでこれで十分）。
	return newAuthManager(&config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "test-session-secret-0123456789",
		SessionTTLHours:   config.DefaultSessionTTLHours,
	}, &sync.RWMutex{})
}

func TestAuth_CheckPassword(t *testing.T) {
	a := newTestAuth("correct-horse")
	if !a.checkPassword("correct-horse") {
		t.Fatal("正しいパスワードが通らない")
	}
	if a.checkPassword("wrong") {
		t.Fatal("誤ったパスワードが通ってしまった")
	}
}

func TestAuth_Token_Valid(t *testing.T) {
	a := newTestAuth("pw")
	tok := a.issueToken()
	if tok == "" {
		t.Fatal("空 token")
	}
	if !a.verifyToken(tok) {
		t.Fatal("発行直後の token が無効扱い")
	}
}

func TestAuth_Token_Tampered(t *testing.T) {
	a := newTestAuth("pw")
	tok := a.issueToken()
	if a.verifyToken(tok + "x") {
		t.Fatal("署名改ざん token が有効扱い")
	}
	if a.verifyToken("junk") {
		t.Fatal("不正形式 token が有効扱い")
	}
	// expiry を未来に書き換えても署名不一致で無効
	parts := strings.Split(tok, ".")
	forged := parts[0] + "." + strconv.FormatInt(time.Now().Add(1000*time.Hour).Unix(), 10) + "." + parts[2]
	if a.verifyToken(forged) {
		t.Fatal("expiry 改ざん token が有効扱い")
	}
}

func TestAuth_Token_Expired(t *testing.T) {
	a := newTestAuth("pw")
	// 過去 expiry のトークンを正規署名で作る
	exp := time.Now().Add(-time.Minute).Unix()
	payload := tokenVersion + "." + strconv.FormatInt(exp, 10)
	tok := payload + "." + a.sign(payload)
	if a.verifyToken(tok) {
		t.Fatal("期限切れ token が有効扱い")
	}
}

func TestAuth_Token_InvalidatedOnPasswordChange(t *testing.T) {
	a := newTestAuth("pw")
	tok := a.issueToken()
	if !a.verifyToken(tok) {
		t.Fatal("発行直後が無効")
	}
	// パスワード（ハッシュ）変更 → 署名鍵が変わり既存トークン無効化
	hash, _ := bcrypt.GenerateFromPassword([]byte("newpw"), bcrypt.MinCost)
	a.cfg.AdminPasswordHash = string(hash)
	if a.verifyToken(tok) {
		t.Fatal("PW変更後も古い token が有効（全無効化されていない）")
	}
}

func TestAuth_LoginLockout(t *testing.T) {
	a := newTestAuth("pw")
	// 失敗を 10 回繰り返すとロック
	for i := 0; i < 10; i++ {
		a.recordLoginResult(false)
	}
	locked, remain := a.loginLocked()
	if !locked {
		t.Fatal("10 連続失敗でロックされていない")
	}
	if remain <= 0 || remain > time.Minute+time.Second {
		t.Fatalf("remain が想定外: %v", remain)
	}
	// 成功 → failures リセット
	a.recordLoginResult(true)
	if a.failures != 0 {
		t.Fatalf("成功後 failures が 0 にリセットされていない: %d", a.failures)
	}
}

func TestAuth_Authorized_CookieAndBearer(t *testing.T) {
	a := newTestAuth("secret-pw")
	s := &Server{auth: a}

	// 1) 認証情報なし → false
	r1 := httptest.NewRequest("GET", "/x", nil)
	if s.authorized(r1) {
		t.Fatal("認証情報無しで authorized が true")
	}
	// 2) Bearer 正しいパスワード → true
	r2 := httptest.NewRequest("GET", "/x", nil)
	r2.Header.Set("Authorization", "Bearer secret-pw")
	if !s.authorized(r2) {
		t.Fatal("正しい Bearer パスワードで認証されない")
	}
	// 3) Bearer 誤り → false
	r3 := httptest.NewRequest("GET", "/x", nil)
	r3.Header.Set("Authorization", "Bearer wrong")
	if s.authorized(r3) {
		t.Fatal("誤った Bearer で認証された")
	}
	// 4) 有効 cookie → true
	tok := a.issueToken()
	r4 := httptest.NewRequest("GET", "/x", nil)
	r4.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	if !s.authorized(r4) {
		t.Fatal("有効 cookie で認証されない")
	}
	// 5) 無効 cookie → false
	r5 := httptest.NewRequest("GET", "/x", nil)
	r5.AddCookie(&http.Cookie{Name: sessionCookie, Value: "junk"})
	if s.authorized(r5) {
		t.Fatal("無効 cookie で認証された")
	}
	// 6) 廃止した query-param ?apiKey= 経路 → false
	r6 := httptest.NewRequest("GET", "/x?apiKey=secret-pw", nil)
	if s.authorized(r6) {
		t.Fatal("廃止した apiKey クエリ経路で認証された")
	}
}
