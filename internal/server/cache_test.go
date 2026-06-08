package server

// キャッシュ管理のテスト: 削除コア（evictOlderThan）・手動全削除（停止中ガード/中身削除/フォルダ無し）・
// 設定 CRUD と検証・停止時自動削除（maybeAutoEvictCache）。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

// newCacheServer は dataDir=tmp（cacheDir={tmp}/headless-cache）の停止中サーバーを返す。
func newCacheServer(t *testing.T) (ts *httptest.Server, pw, cacheDir string, srv *Server) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	cacheDir = filepath.Join(tmp, "headless-cache")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "cache-test-secret",
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	srv = New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, testPassword, cacheDir, srv
}

func mkCacheFile(t *testing.T, dir, rel, content string, mod time.Time) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestEvictOlderThan(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	mkCacheFile(t, dir, "old1.bin", "aaaa", now.Add(-40*24*time.Hour))
	mkCacheFile(t, dir, "sub/old2.bin", "bb", now.Add(-31*24*time.Hour))
	mkCacheFile(t, dir, "sub/new.bin", "cccc", now.Add(-1*24*time.Hour))
	mkCacheFile(t, dir, "recent.bin", "d", now)

	removed, freed, err := evictOlderThan(dir, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed=%d want 2", removed)
	}
	if want := int64(len("aaaa") + len("bb")); freed != want {
		t.Fatalf("freed=%d want %d", freed, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "old1.bin")); !os.IsNotExist(err) {
		t.Fatal("old1 should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "new.bin")); err != nil {
		t.Fatal("new should be kept")
	}
	if _, err := os.Stat(filepath.Join(dir, "recent.bin")); err != nil {
		t.Fatal("recent should be kept")
	}
	if _, err := os.Stat(filepath.Join(dir, "sub")); err != nil {
		t.Fatal("sub should remain (still has new.bin)")
	}
}

func TestEvictOlderThan_PrunesEmptiedDirsAndMissing(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour)
	mkCacheFile(t, dir, "a/b/x.bin", "x", old)
	mkCacheFile(t, dir, "a/y.bin", "y", old)
	removed, _, err := evictOlderThan(dir, time.Now().Add(-30*24*time.Hour))
	if err != nil || removed != 2 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatal("emptied subdir 'a' should be pruned")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("root cache dir must remain")
	}
	// 存在しないフォルダは no-op
	if r, _, err := evictOlderThan(filepath.Join(dir, "nope"), time.Now()); err != nil || r != 0 {
		t.Fatalf("missing dir should be no-op: r=%d err=%v", r, err)
	}
}

func TestCacheClear_StoppedEmptiesContents(t *testing.T) {
	ts, pw, cacheDir, _ := newCacheServer(t)
	mkCacheFile(t, cacheDir, "a.bin", "data", time.Now())
	mkCacheFile(t, cacheDir, "sub/b.bin", "more", time.Now())

	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/cache/clear", pw, "", "")
	code := resp.StatusCode
	resp.Body.Close()
	if code != http.StatusOK {
		t.Fatalf("clear code=%d", code)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("cache dir should still exist: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("cache dir should be empty, has %d entries", len(entries))
	}
}

func TestCacheClear_MissingDirOK(t *testing.T) {
	ts, pw, _, _ := newCacheServer(t) // headless-cache 未作成
	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/cache/clear", pw, "", "")
	code := resp.StatusCode
	resp.Body.Close()
	if code != http.StatusOK {
		t.Fatalf("clear on missing dir should be 200, got %d", code)
	}
}

func TestCacheClear_RunningGuard(t *testing.T) {
	ts, pw := newTestServer(t) // fakehl 稼働中
	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/cache/clear", pw, "", "")
	code := resp.StatusCode
	resp.Body.Close()
	if code != http.StatusConflict {
		t.Fatalf("running guard should be 409, got %d", code)
	}
}

func TestCacheConfig_GetPutValidation(t *testing.T) {
	ts, pw, _, _ := newCacheServer(t)
	var got okEnv[cacheConfigResp]
	authGet(t, ts.URL+"/api/v1/cache/config", pw, &got)
	if got.Data.Enabled || got.Data.MaxAgeDays != config.DefaultCacheMaxAgeDays {
		t.Fatalf("default not OFF/30: %+v", got.Data)
	}
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/cache/config", pw, "application/json", `{"enabled":true,"maxAgeDays":14}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put code=%d", resp.StatusCode)
	}
	resp.Body.Close()
	var got2 okEnv[cacheConfigResp]
	authGet(t, ts.URL+"/api/v1/cache/config", pw, &got2)
	if !got2.Data.Enabled || got2.Data.MaxAgeDays != 14 {
		t.Fatalf("not persisted: %+v", got2.Data)
	}
	// maxAgeDays<1 は 400
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/cache/config", pw, "application/json", `{"enabled":true,"maxAgeDays":0}`)
	code := resp.StatusCode
	resp.Body.Close()
	if code != http.StatusBadRequest {
		t.Fatalf("maxAgeDays<1 should be 400, got %d", code)
	}
}

func TestMaybeAutoEvictCache(t *testing.T) {
	_, _, cacheDir, srv := newCacheServer(t)
	now := time.Now()
	mkCacheFile(t, cacheDir, "old.bin", "old", now.Add(-40*24*time.Hour))
	mkCacheFile(t, cacheDir, "new.bin", "new", now)

	// OFF（既定）のときは何もしない
	srv.maybeAutoEvictCache()
	if _, err := os.Stat(filepath.Join(cacheDir, "old.bin")); err != nil {
		t.Fatal("disabled: old.bin should remain")
	}

	// ON にして実行 → 古いものだけ消える
	srv.cfg.CacheCleanup = &config.CacheCleanup{Enabled: true, MaxAgeDays: 30}
	srv.maybeAutoEvictCache()
	if _, err := os.Stat(filepath.Join(cacheDir, "old.bin")); !os.IsNotExist(err) {
		t.Fatal("enabled: old.bin should be removed")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "new.bin")); err != nil {
		t.Fatal("enabled: new.bin should remain")
	}
}
