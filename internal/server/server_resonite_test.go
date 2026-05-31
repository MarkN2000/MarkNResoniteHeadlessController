package server

// GET /api/v1/resonite/users（Resonite 公開API プロキシ・P9-A）の HTTP テスト。
// 上流の api.resonite.com を stub（httptest）に差し替えて外部依存なしで検証する。
// driver は停止のまま（検索ハンドラは driver を使わない）。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

func newResoniteTestServer(t *testing.T, apiBase string) (*httptest.Server, string) {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "reso-test-secret",
	}
	drv := headless.NewDriver(nil) // 停止のまま
	srv := New(cfg, "", drv, resonite.NewClientWithBase(apiBase), nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, testPassword
}

// 正常系: 名前検索 → 上流の配列を整形して返す（iconUrl 正規化込み）。
func TestResoniteUserSearch_ByName(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "xyz" {
			t.Errorf("upstream got unexpected query: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`[{"id":"U-x","username":"Xyz","profile":{"iconUrl":"resdb:///hash.png"}}]`))
	}))
	defer api.Close()
	ts, pw := newResoniteTestServer(t, api.URL)

	var env okEnv[[]resonite.User]
	code := authGet(t, ts.URL+"/api/v1/resonite/users?q=xyz", pw, &env)
	if code != http.StatusOK || !env.OK {
		t.Fatalf("search failed: code=%d env=%+v", code, env)
	}
	if len(env.Data) != 1 || env.Data[0].ID != "U-x" || env.Data[0].Username != "Xyz" {
		t.Fatalf("unexpected data: %+v", env.Data)
	}
	if env.Data[0].IconURL != "https://assets.resonite.com/hash" {
		t.Errorf("iconUrl not normalized: %q", env.Data[0].IconURL)
	}
}

// 入力検証: q 無し → 400 missing_query。
func TestResoniteUserSearch_MissingQuery_400(t *testing.T) {
	ts, pw := newResoniteTestServer(t, "http://unused.invalid")
	var env okEnv[[]resonite.User]
	code := authGet(t, ts.URL+"/api/v1/resonite/users", pw, &env)
	if code != http.StatusBadRequest || env.Error.Code != "missing_query" {
		t.Fatalf("want 400 missing_query, got code=%d env=%+v", code, env)
	}
}

// 認証: ヘッダ無し → 401。
func TestResoniteUserSearch_RequiresAuth_401(t *testing.T) {
	ts, _ := newResoniteTestServer(t, "http://unused.invalid")
	resp, err := http.Get(ts.URL + "/api/v1/resonite/users?q=x")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}
