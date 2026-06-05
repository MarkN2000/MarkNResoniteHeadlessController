package server

// R-A / 一本化: 既定パス導出（Steam.InstallDir 未設定でも {dataDir}/resonite から起動/更新先を導出）と
// 未DL 時の親切エラーの回帰テスト。

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

// newPathServer は Steam.InstallDir 未設定の Server を返す（dataDir=tmp）。steam は任意で注入。
func newPathServer(t *testing.T, steam *config.Steam) (s *Server, dataDir string) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "paths-test-secret",
		Port:              8080,
		HeadlessConfigDir: filepath.Join(tmp, "configs"),
		Steam:             steam,
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	return New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil), tmp
}

// resolveLaunch は Steam.InstallDir 未設定なら {dataDir}/resonite/Headless/<OS バイナリ> を導出する。
func TestResolveLaunch_DerivesDefaultPath(t *testing.T) {
	s, dataDir := newPathServer(t, nil)
	// config 不在で launchPath は ErrNotFound になるが headlessPath は導出前に計算される。
	headlessPath, _, _ := s.resolveLaunch("whatever")
	want := filepath.Join(dataDir, "resonite", "Headless", platform.HeadlessBinaryName())
	if headlessPath != want {
		t.Errorf("headlessPath=%q want %q", headlessPath, want)
	}
}

// steamParams は InstallDir 未設定でも既定 {dataDir}/resonite を埋め、資格が揃えば成功する。
func TestSteamParams_DefaultsInstallDir(t *testing.T) {
	s, dataDir := newPathServer(t, &config.Steam{Username: "u", Password: "p", BranchCode: "b"})
	p, err := s.steamParams()
	if err != nil {
		t.Fatalf("資格が揃えば成功すべき: %v", err)
	}
	if want := filepath.Join(dataDir, "resonite"); p.InstallDir != want {
		t.Errorf("InstallDir=%q want %q", p.InstallDir, want)
	}
}

// 資格欠如（install 先は導出できても）は ErrSteamNotConfigured。
func TestSteamParams_MissingCredentials(t *testing.T) {
	s, _ := newPathServer(t, &config.Steam{Username: "u"}) // password / branchCode 欠如
	if _, err := s.steamParams(); err == nil {
		t.Fatal("資格欠如は ErrSteamNotConfigured を返すべき")
	}
}

// 既定パスに Resonite が無い（未DL）状態の start は headless_not_installed を返す。
func TestHandleStart_HeadlessNotInstalled(t *testing.T) {
	ts, pw, _ := newConfigServer(t) // Steam.InstallDir 未設定・dataDir に resonite/ は無い
	// 起動できる config を1つ用意（これが無いと config_not_found で別経路になる）。
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-configs/w", pw, "application/json",
		`{"startWorlds":[{"sessionName":"W"}]}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config PUT status=%d", resp.StatusCode)
	}

	startResp := authReq(t, http.MethodPost, ts.URL+"/api/v1/start", pw, "application/json", `{"config":"w"}`)
	defer startResp.Body.Close()
	var got struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(startResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if startResp.StatusCode != http.StatusConflict || got.Error.Code != "headless_not_installed" {
		t.Fatalf("未DL start は 409 headless_not_installed を期待: code=%d body=%+v", startResp.StatusCode, got)
	}
}
