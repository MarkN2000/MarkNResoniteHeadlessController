package server

// authManager の unit テスト（Phase 7 前レビューで追加）。
// 主要 API の cover を 0% → 90%+ に上げる。実プロセス不要、httptest 不要。

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

func newTestAuth(password, apiKey string) *authManager {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	return newAuthManager(&config.Config{
		AdminPasswordHash: string(hash),
		APIKey:            apiKey,
	})
}

func TestAuth_CheckPassword(t *testing.T) {
	a := newTestAuth("correct-horse", "")
	if !a.checkPassword("correct-horse") {
		t.Fatal("正しいパスワードが通らない")
	}
	if a.checkPassword("wrong") {
		t.Fatal("誤ったパスワードが通ってしまった")
	}
}

func TestAuth_Session(t *testing.T) {
	a := newTestAuth("pw", "")
	tok := a.newSession()
	if tok == "" {
		t.Fatal("空 token")
	}
	if !a.validSession(tok) {
		t.Fatal("発行直後の session が無効扱い")
	}
	a.dropSession(tok)
	if a.validSession(tok) {
		t.Fatal("dropSession 後も session が有効")
	}
}

func TestAuth_SessionExpiry(t *testing.T) {
	// 内部直接操作で期限切れを再現
	a := newTestAuth("pw", "")
	tok := a.newSession()
	a.mu.Lock()
	a.sessions[tok] = time.Now().Add(-time.Minute) // 1分前に失効
	a.mu.Unlock()
	if a.validSession(tok) {
		t.Fatal("期限切れ session が有効扱い")
	}
	// validSession は失効 session を削除するはず
	a.mu.Lock()
	_, exists := a.sessions[tok]
	a.mu.Unlock()
	if exists {
		t.Fatal("期限切れ session が cleanup されていない")
	}
}

func TestAuth_LoginLockout(t *testing.T) {
	a := newTestAuth("pw", "")
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
	// 成功 → ロック解除（前提: 直前の failures はリセットされる）
	a.recordLoginResult(true)
	// 注: ロック自体は時間経過で解除される設計なので、直後はまだロック中。
	//     ここでは「成功で failures カウンタがリセットされる」ことだけ確認。
	if a.failures != 0 {
		t.Fatalf("成功後 failures が 0 にリセットされていない: %d", a.failures)
	}
}

func TestAuth_APIKeyValidation(t *testing.T) {
	a := newTestAuth("pw", "my-secret-key")
	if !a.validAPIKey("my-secret-key") {
		t.Fatal("正しい APIKey が通らない")
	}
	if a.validAPIKey("wrong") {
		t.Fatal("誤った APIKey が通った")
	}
	if a.validAPIKey("") {
		t.Fatal("空文字 APIKey が通った")
	}
	// APIKey 未設定時は常に false
	a2 := newTestAuth("pw", "")
	if a2.validAPIKey("anything") {
		t.Fatal("APIKey 未設定なのに valid 扱い")
	}
}

func TestAuth_Authorized_CookieAndAPIKey(t *testing.T) {
	a := newTestAuth("pw", "key1")
	s := &Server{auth: a}

	// 1) 認証情報なし → false
	r1 := httptest.NewRequest("GET", "/x", nil)
	if s.authorized(r1) {
		t.Fatal("認証情報無しで authorized が true")
	}
	// 2) APIKey クエリ → true
	r2 := httptest.NewRequest("GET", "/x?apiKey=key1", nil)
	if !s.authorized(r2) {
		t.Fatal("APIKey クエリで認証されない")
	}
	// 3) Bearer ヘッダ → true
	r3 := httptest.NewRequest("GET", "/x", nil)
	r3.Header.Set("Authorization", "Bearer key1")
	if !s.authorized(r3) {
		t.Fatal("Bearer ヘッダで認証されない")
	}
	// 4) 不正な APIKey → false
	r4 := httptest.NewRequest("GET", "/x?apiKey=wrong", nil)
	if s.authorized(r4) {
		t.Fatal("不正な APIKey で認証された")
	}
	// 5) 有効な session cookie → true
	tok := a.newSession()
	r5 := httptest.NewRequest("GET", "/x", nil)
	r5.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	if !s.authorized(r5) {
		t.Fatal("有効 cookie で認証されない")
	}
	// 6) 無効な cookie → false
	r6 := httptest.NewRequest("GET", "/x", nil)
	r6.AddCookie(&http.Cookie{Name: sessionCookie, Value: "junk"})
	if s.authorized(r6) {
		t.Fatal("無効 cookie で認証された")
	}
}
