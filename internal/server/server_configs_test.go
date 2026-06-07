package server

// Headless Config CRUD + credentials + start-by-name の HTTP 統合テスト（Pre-7b）。
// CRUD/credentials は driver 不要（fakehl を起動しない軽量テスト）。
// start-by-name のみ fakehl を使う（fakehlPath は server_integration_test.go の TestMain で用意）。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

// newConfigServer は driver 未起動の Server を立て、temp の cfgPath/configDir を使う。
func newConfigServer(t *testing.T) (ts *httptest.Server, pw, configDir string) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	configDir = filepath.Join(tmp, "configs")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "cfgtest-secret",
		HeadlessConfigDir: configDir,
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	drv := headless.NewDriver(nil)
	srv := New(cfg, cfgPath, drv, resonite.NewClient(), nil)
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, testPassword, configDir
}

// authReq は Bearer 付きで任意メソッドのリクエストを送る。
func authReq(t *testing.T, method, url, pw, contentType, body string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Authorization", "Bearer "+pw)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

// GET /api/v1/headless-config-defaults は dataDir 配下の headless-data/headless-cache（絶対パス）を返す（UI改善⑤）。
func TestConfigDefaults(t *testing.T) {
	ts, pw, _ := newConfigServer(t)
	var got struct {
		Data struct {
			DataFolder  string `json:"dataFolder"`
			CacheFolder string `json:"cacheFolder"`
		} `json:"data"`
	}
	if code := authGet(t, ts.URL+"/api/v1/headless-config-defaults", pw, &got); code != http.StatusOK {
		t.Fatalf("code = %d", code)
	}
	if !filepath.IsAbs(got.Data.DataFolder) || filepath.Base(got.Data.DataFolder) != "headless-data" {
		t.Fatalf("dataFolder wrong: %q", got.Data.DataFolder)
	}
	if !filepath.IsAbs(got.Data.CacheFolder) || filepath.Base(got.Data.CacheFolder) != "headless-cache" {
		t.Fatalf("cacheFolder wrong: %q", got.Data.CacheFolder)
	}
}

func TestConfigs_PutGetMask(t *testing.T) {
	ts, pw, _ := newConfigServer(t)
	body := `{"loginCredential":"u@e.com","loginPassword":"secret","comment":"c","startWorlds":[{"sessionName":"W"}]}`
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-configs/myworld", pw, "application/json", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d", resp.StatusCode)
	}

	var got okEnv[map[string]any]
	code := authGet(t, ts.URL+"/api/v1/headless-configs/myworld", pw, &got)
	if code != http.StatusOK || !got.OK {
		t.Fatalf("GET status=%d body=%+v", code, got)
	}
	if got.Data["loginPassword"] != "" {
		t.Fatalf("password should be masked, got %v", got.Data["loginPassword"])
	}
	if got.Data["loginCredential"] != "u@e.com" {
		t.Fatalf("username should be returned, got %v", got.Data["loginCredential"])
	}
	if got.Data["comment"] != "c" {
		t.Fatalf("comment lost: %v", got.Data["comment"])
	}
}

func TestConfigs_List_SaveAs_Delete(t *testing.T) {
	ts, pw, _ := newConfigServer(t)
	put := func(name, body string) {
		resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-configs/"+name, pw, "application/json", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT %s: %d", name, resp.StatusCode)
		}
	}
	put("alpha", `{"comment":"A","startWorlds":[]}`)
	// Save As: GET alpha 相当を別名 beta で保存（ここでは別 body を PUT）
	put("beta", `{"comment":"B","startWorlds":[{"sessionName":"x"}]}`)

	var list okEnv[[]map[string]any]
	code := authGet(t, ts.URL+"/api/v1/headless-configs", pw, &list)
	if code != http.StatusOK || len(list.Data) != 2 {
		t.Fatalf("list: status=%d data=%+v", code, list.Data)
	}

	// delete alpha
	resp := authReq(t, http.MethodDelete, ts.URL+"/api/v1/headless-configs/alpha", pw, "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE expected 200, got %d", resp.StatusCode)
	}
	// alpha は 404
	if code := authGet(t, ts.URL+"/api/v1/headless-configs/alpha", pw, nil); code != http.StatusNotFound {
		t.Fatalf("deleted config GET expected 404, got %d", code)
	}
}

func TestConfigs_InvalidName(t *testing.T) {
	ts, pw, _ := newConfigServer(t)
	// 空白入りの不正名 → 400
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-configs/bad%20name", pw, "application/json", `{"startWorlds":[]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid name, got %d", resp.StatusCode)
	}
}

func TestConfigs_RequiresAuth(t *testing.T) {
	ts, _, _ := newConfigServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/headless-configs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestCredentials_PutGet(t *testing.T) {
	ts, pw, _ := newConfigServer(t)
	// 初期は username 空・hasPassword false
	var c0 okEnv[map[string]any]
	authGet(t, ts.URL+"/api/v1/headless-credentials", pw, &c0)
	if c0.Data["hasPassword"] != false {
		t.Fatalf("initial hasPassword should be false: %+v", c0.Data)
	}
	// 設定
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-credentials", pw, "application/json", `{"username":"bot@e.com","password":"botpw"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT creds expected 200, got %d", resp.StatusCode)
	}
	var c1 okEnv[map[string]any]
	authGet(t, ts.URL+"/api/v1/headless-credentials", pw, &c1)
	if c1.Data["username"] != "bot@e.com" || c1.Data["hasPassword"] != true {
		t.Fatalf("creds not stored: %+v", c1.Data)
	}
	// password 空で username だけ更新 → password 保持
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-credentials", pw, "application/json", `{"username":"bot2@e.com","password":""}`)
	resp.Body.Close()
	var c2 okEnv[map[string]any]
	authGet(t, ts.URL+"/api/v1/headless-credentials", pw, &c2)
	if c2.Data["username"] != "bot2@e.com" || c2.Data["hasPassword"] != true {
		t.Fatalf("password should be preserved on empty: %+v", c2.Data)
	}
}

// PUT credentials が username→UserID を解決して保存・GET で返す（R12）。Resonite API は stub。
func TestCredentials_ResolvesUserID(t *testing.T) {
	apiHits := 0
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits++
		// 名前検索に normalizedUsername 完全一致を含めて返す。
		_, _ = w.Write([]byte(`[{"id":"U-MarkN","username":"MarkN","normalizedUsername":"markn"}]`))
	}))
	defer stub.Close()

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "cfgtest-secret",
		HeadlessConfigDir: filepath.Join(tmp, "configs"),
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	srv := New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClientWithBase(stub.URL), nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	pw := testPassword

	// username（非メール）→ 解決して UserID 保存。
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-credentials", pw, "application/json", `{"username":"MarkN","password":"pw"}`)
	resp.Body.Close()
	var c1 okEnv[map[string]any]
	authGet(t, ts.URL+"/api/v1/headless-credentials", pw, &c1)
	if c1.Data["userId"] != "U-MarkN" {
		t.Fatalf("userId not resolved/stored: %+v", c1.Data)
	}
	// ディスクにも保存されている。
	if disk, _ := os.ReadFile(cfgPath); !strings.Contains(string(disk), "U-MarkN") {
		t.Fatalf("userId not persisted to disk")
	}

	// 同じ username で再保存（password だけ変更）→ 再解決せず API を叩かない（流用）。
	hitsBefore := apiHits
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-credentials", pw, "application/json", `{"username":"MarkN","password":"pw2"}`)
	resp.Body.Close()
	if apiHits != hitsBefore {
		t.Fatalf("unchanged username should not re-resolve (api hit %d→%d)", hitsBefore, apiHits)
	}

	// メール形式 → 解決対象外・UserID 空（API も叩かない）。
	hitsBefore = apiHits
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-credentials", pw, "application/json", `{"username":"me@example.com","password":""}`)
	resp.Body.Close()
	var c3 okEnv[map[string]any]
	authGet(t, ts.URL+"/api/v1/headless-credentials", pw, &c3)
	if c3.Data["userId"] != "" {
		t.Fatalf("email username should clear userId: %+v", c3.Data)
	}
	if apiHits != hitsBefore {
		t.Fatalf("email username should not hit api")
	}
}

// start-by-name: config 名で起動 → name 解決 → 一時 config 生成 → fakehl 起動 → last-used 記録
func TestConfigs_StartByName(t *testing.T) {
	if runtime.GOOS != "windows" {
		// 一本化後、非 Windows は installDir/Headless/Resonite.dll を dotnet 経由で起動するため、
		// ネイティブ Go バイナリの fakehl を実行できない。パス導出自体は server_paths_test が
		// OS 非依存に検証済みなので、ここは Windows（.exe 直接実行）でのみ実プロセス起動を検証する。
		t.Skip("fakehl は .dll/dotnet 経由起動と非互換のため Windows のみ検証")
	}
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	configDir := filepath.Join(tmp, "configs")
	installDir := filepath.Join(tmp, "resonite")

	// 一本化後は起動パスを installDir から導出するため、fakehl を installDir/Headless/<OS バイナリ名> に配置する。
	headlessDir := filepath.Join(installDir, "Headless")
	if err := os.MkdirAll(headlessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakehlBin, err := os.ReadFile(fakehlPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(headlessDir, platform.HeadlessBinaryName()), fakehlBin, 0o755); err != nil {
		t.Fatal(err)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "start-secret",
		HeadlessConfigDir: configDir,
		Steam:             &config.Steam{InstallDir: installDir},
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	// config を1つ用意
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "myworld.json"),
		[]byte(`{"loginCredential":"u","loginPassword":"p","startWorlds":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	drv := headless.NewDriver(nil)
	srv := New(cfg, cfgPath, drv, resonite.NewClient(), nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		_ = drv.SendCommand("shutdown")
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if drv.Status().State == headless.StateStopped {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})

	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/start", testPassword, "application/json", `{"config":"myworld"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start by name expected 200, got %d", resp.StatusCode)
	}
	// 一時 config が生成されている
	if _, err := os.Stat(filepath.Join(tmp, ".run", "myworld.json")); err != nil {
		t.Fatalf(".run config not generated: %v", err)
	}
	// Status().Config は論理名 "myworld"（一時パスではない）
	// last-used が記録されている
	var lu okEnv[map[string]any]
	authGet(t, ts.URL+"/api/v1/headless-configs/last-used", testPassword, &lu)
	if lu.Data["lastUsed"] != "myworld" {
		t.Fatalf("last-used not recorded: %+v", lu.Data)
	}
}
