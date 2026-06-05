package server

// Steam（DepotDownloader）HTTP 層のテスト（P9-B）。実ダウンロードは走らせず、
// config CRUD・秘密マスキング・検証・未設定/idle のエラー経路のみ検証する
// （実ダウンロード/進行管理は internal/steam の単体・e2e で網羅）。

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

func newSteamServer(t *testing.T) (ts *httptest.Server, pw, cfgPath string) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath = filepath.Join(tmp, "mrhc.config.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "steam-test-secret",
		Port:              8080,
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

func TestSteamConfig_GetPutMasking(t *testing.T) {
	ts, pw, cfgPath := newSteamServer(t)

	// 初期は空
	var got okEnv[steamConfigResp]
	if code := authGet(t, ts.URL+"/api/v1/steam/config", pw, &got); code != http.StatusOK {
		t.Fatalf("GET status=%d", code)
	}
	if got.Data.HasPassword || got.Data.HasBranchCode || got.Data.Username != "" {
		t.Fatalf("初期が空でない: %+v", got.Data)
	}

	// PUT 設定（installDir は trim される）
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/steam/config", pw, "application/json",
		`{"username":"spareacct","password":"s3cret-ascii","branchCode":"betacode","installDir":"  /opt/Resonite  "}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	// GET 応答本文に秘密が出ないこと
	resp = authReq(t, http.MethodGet, ts.URL+"/api/v1/steam/config", pw, "", "")
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if body := string(bodyBytes); strings.Contains(body, "s3cret-ascii") || strings.Contains(body, "betacode") {
		t.Fatalf("秘密が応答に漏れている: %s", body)
	}

	var got2 okEnv[steamConfigResp]
	authGet(t, ts.URL+"/api/v1/steam/config", pw, &got2)
	if !got2.Data.HasPassword || !got2.Data.HasBranchCode {
		t.Fatalf("hasXxx が反映されない: %+v", got2.Data)
	}
	if got2.Data.Username != "spareacct" || got2.Data.InstallDir != "/opt/Resonite" {
		t.Fatalf("値が想定外（trim含む）: %+v", got2.Data)
	}

	// ファイルには秘密が保存されている（子プロセスへ渡すため復元可能保存）
	reloaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Steam == nil || reloaded.Steam.Password != "s3cret-ascii" || reloaded.Steam.BranchCode != "betacode" {
		t.Fatalf("秘密がファイルに保存されていない: %+v", reloaded.Steam)
	}

	// PUT で password/branchCode 空 → 既存維持（秘密を空で潰さない）
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/steam/config", pw, "application/json",
		`{"username":"spareacct2","password":"","branchCode":"","installDir":"/opt/Resonite"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT2 status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	reloaded2, _ := config.LoadFrom(cfgPath)
	if reloaded2.Steam.Password != "s3cret-ascii" || reloaded2.Steam.BranchCode != "betacode" {
		t.Fatalf("空PUTで秘密が消えた: %+v", reloaded2.Steam)
	}
	if reloaded2.Steam.Username != "spareacct2" {
		t.Fatalf("username が更新されていない: %+v", reloaded2.Steam)
	}
}

func TestSteamConfig_PasswordValidation(t *testing.T) {
	ts, pw, _ := newSteamServer(t)
	// 非ASCII
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/steam/config", pw, "application/json",
		`{"username":"u","password":"パスワード","branchCode":"b","installDir":"/x"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("非ASCII PW で 400 にならない: %d", resp.StatusCode)
	}
	resp.Body.Close()
	// 65文字
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/steam/config", pw, "application/json",
		`{"username":"u","password":"`+strings.Repeat("a", 65)+`","branchCode":"b","installDir":"/x"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("64超 PW で 400 にならない: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSteamDownload_NotConfigured(t *testing.T) {
	ts, pw, _ := newSteamServer(t)
	// driver 未起動(stopped) かつ Steam 未設定 → 400 steam_not_configured（ネットワークに行かない）
	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/steam/download", pw, "application/json", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("未設定 download は 400: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSteamCancel_Idle(t *testing.T) {
	ts, pw, _ := newSteamServer(t)
	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/steam/cancel", pw, "application/json", "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("idle の cancel は 409: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSteamStatus_Idle(t *testing.T) {
	ts, pw, _ := newSteamServer(t)
	var got okEnv[struct {
		State string `json:"state"`
	}]
	if code := authGet(t, ts.URL+"/api/v1/steam/status", pw, &got); code != http.StatusOK {
		t.Fatalf("status GET=%d", code)
	}
	if got.Data.State != "idle" {
		t.Fatalf("初期状態は idle のはず: %q", got.Data.State)
	}
}

// TestMaybeScheduledUpdate_NoopGating は更新を「走らせない」3条件を検証する（実DLしないことを担保）。
// 実際に更新する経路（scheduled+ON+設定済み）は internal/steam の偽DD e2e で網羅する。
func TestMaybeScheduledUpdate_NoopGating(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	fullSteam := &config.Steam{Username: "u", Password: "p", BranchCode: "b", InstallDir: "/i"}

	mk := func(steamCfg *config.Steam, toggle bool) *Server {
		rc := config.DefaultRestart()
		rc.UpdateOnScheduledRestart = toggle
		cfg := &config.Config{
			Version: config.SchemaVersion, AdminPasswordHash: string(hash),
			SessionSecret: "x", Port: 8080,
			Restart: &rc, Steam: steamCfg,
		}
		if err := cfg.SaveTo(cfgPath); err != nil {
			t.Fatal(err)
		}
		return New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	}

	cases := []struct {
		name    string
		trigger string
		steam   *config.Steam
		toggle  bool
	}{
		{"manual は対象外", "manual", fullSteam, true},
		{"トグル OFF は更新しない", "scheduled", fullSteam, false},
		{"Steam 未設定は更新しない", "scheduled", nil, true},
	}
	for _, c := range cases {
		s := mk(c.steam, c.toggle)
		s.maybeScheduledUpdate(context.Background(), c.trigger)
		if st := s.steam.Status().State; st != "idle" {
			t.Errorf("%s: 更新が走ってしまった（state=%q・期待 idle）", c.name, st)
		}
	}
}

// InstallDir 導出の単体は config.TestInstallDirOrDefault へ移設（deriveInstallDir は config に統合）。
