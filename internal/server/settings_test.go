package server

// 設定タブ（7-5）バックエンドの HTTP テスト: 管理パスワード変更 + アプリ設定 CRUD。
// driver は未起動でよい（これらのエンドポイントは driver に触れない）。
// 共通ヘルパ（authGet/authReq/okEnv/testPassword/sessionCookie）は同パッケージの既存テストを再利用。

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

// newSettingsServer は実 cfgPath（SaveTo が成功する）を持つ driver 未起動 Server を立てる。
func newSettingsServer(t *testing.T) (ts *httptest.Server, pw, cfgPath string) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath = filepath.Join(tmp, "mrhc.config.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "settings-test-secret",
		Port:              8080,
		ResoniteHeadless:  "/orig/Resonite",
		HeadlessConfigDir: filepath.Join(tmp, "configs"),
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	srv := New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, testPassword, cfgPath
}

func TestAppSettings_GetPut(t *testing.T) {
	ts, pw, cfgPath := newSettingsServer(t)

	// GET 初期値
	var got okEnv[appSettings]
	if code := authGet(t, ts.URL+"/api/v1/app-settings", pw, &got); code != http.StatusOK {
		t.Fatalf("GET status=%d", code)
	}
	if got.Data.Port != 8080 || got.Data.ResoniteHeadless != "/orig/Resonite" {
		t.Fatalf("GET 初期値が想定外: %+v", got.Data)
	}

	// PUT 更新
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/app-settings", pw, "application/json",
		`{"port":9090,"resoniteHeadlessPath":"  /new/Resonite  ","headlessConfigDir":"/cfgs"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	// GET が反映を返す（path は trim される）
	var got2 okEnv[appSettings]
	authGet(t, ts.URL+"/api/v1/app-settings", pw, &got2)
	if got2.Data.Port != 9090 || got2.Data.ResoniteHeadless != "/new/Resonite" || got2.Data.HeadlessConfigDir != "/cfgs" {
		t.Fatalf("PUT 後の GET が想定外: %+v", got2.Data)
	}

	// ファイルにも永続化されている
	reloaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Port != 9090 || reloaded.ResoniteHeadless != "/new/Resonite" || reloaded.HeadlessConfigDir != "/cfgs" {
		t.Fatalf("ファイル未反映: %+v", reloaded)
	}

	// 不正ポートは 400
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/app-settings", pw, "application/json",
		`{"port":0,"resoniteHeadlessPath":"/x","headlessConfigDir":""}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("port=0 で 400 にならない: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPasswordChange(t *testing.T) {
	ts, pw, _ := newSettingsServer(t)

	// 現PW誤り → 401
	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/password", pw, "application/json",
		`{"currentPassword":"wrong","newPassword":"brandnew-pw"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("誤った現PWで 401 にならない: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 新PW空 → 400
	resp = authReq(t, http.MethodPost, ts.URL+"/api/v1/password", pw, "application/json",
		`{"currentPassword":"`+testPassword+`","newPassword":"  "}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("空の新PWで 400 にならない: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 正しい現PW → 200 + 新Cookie 再発行
	resp = authReq(t, http.MethodPost, ts.URL+"/api/v1/password", pw, "application/json",
		`{"currentPassword":"`+testPassword+`","newPassword":"brandnew-pw"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("正しい現PWで 200 にならない: %d", resp.StatusCode)
	}
	var hasCookie bool
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			hasCookie = true
		}
	}
	resp.Body.Close()
	if !hasCookie {
		t.Fatal("PW変更応答に新セッション Cookie が無い（操作中ブラウザが継続できない）")
	}

	// 旧PW(Bearer) は失効 / 新PW は有効
	if code := authGet(t, ts.URL+"/api/v1/app-settings", testPassword, nil); code != http.StatusUnauthorized {
		t.Fatalf("旧PW(Bearer) が失効していない: %d", code)
	}
	if code := authGet(t, ts.URL+"/api/v1/app-settings", "brandnew-pw", nil); code != http.StatusOK {
		t.Fatalf("新PW(Bearer) が通らない: %d", code)
	}
}
