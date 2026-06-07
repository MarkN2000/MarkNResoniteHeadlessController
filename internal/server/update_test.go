package server

// 自己更新 HTTP 層のテスト。スワップ・抽出の実体は internal/selfupdate の単体で網羅済みのため、
// ここでは HTTP セマンティクス（応答形・errCode マップ・staged・shutdown 依頼）のみ検証する。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/selfupdate"
)

// newUpdateTestServer はセッター注入のため *Server も返す（newSteamServer と同構成）。
func newUpdateTestServer(t *testing.T) (ts *httptest.Server, pw string, srv *Server) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "update-test-secret",
		Port:              8080,
		HeadlessConfigDir: filepath.Join(tmp, "configs"),
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	srv = New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, testPassword, srv
}

// fakeLatest は releases/latest だけを模す（tag 空なら 404）。
func fakeLatest(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" || tag == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Location", "/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testUpdater(baseURL, version string) *selfupdate.Updater {
	return &selfupdate.Updater{
		BaseURL: baseURL,
		Version: version,
		CheckClient: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		DLClient: &http.Client{},
	}
}

// assertErrCode は {ok:false, error:{code}} の code を検証する（body は consume される）。
func assertErrCode(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != want {
		t.Errorf("errCode = %q, want %q", env.Error.Code, want)
	}
}

func TestUpdateCheck(t *testing.T) {
	ts, pw, srv := newUpdateTestServer(t)
	gh := fakeLatest(t, "v2.1.0")

	// updater 未注入（テスト既定）→ 503
	var env okEnv[updateCheckResp]
	if code := authGet(t, ts.URL+"/api/v1/update/check", pw, &env); code != http.StatusServiceUnavailable {
		t.Fatalf("未注入で status=%d, want 503", code)
	}

	// 注入後: current/latest/updateAvailable/goos が返る
	srv.SetUpdater(testUpdater(gh.URL, "v2.0.0"))
	if code := authGet(t, ts.URL+"/api/v1/update/check", pw, &env); code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	d := env.Data
	if d.Current != "v2.0.0" || d.Latest != "v2.1.0" || !d.UpdateAvailable || !d.CurrentIsRelease {
		t.Errorf("data = %+v", d)
	}
	if d.Goos == "" || d.Staged != "" {
		t.Errorf("goos/staged = %q/%q", d.Goos, d.Staged)
	}
}

func TestUpdateCheckNoRelease(t *testing.T) {
	ts, pw, srv := newUpdateTestServer(t)
	srv.SetUpdater(testUpdater(fakeLatest(t, "").URL, "v2.0.0")) // リリース未公開

	resp := authReq(t, http.MethodGet, ts.URL+"/api/v1/update/check", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
	assertErrCode(t, resp, "no_release")
}

func TestUpdateApplyUpToDate(t *testing.T) {
	ts, pw, srv := newUpdateTestServer(t)
	srv.SetUpdater(testUpdater(fakeLatest(t, "v2.0.0").URL, "v2.0.0")) // 最新 == 現行

	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/update/apply", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409", resp.StatusCode)
	}
	assertErrCode(t, resp, "up_to_date")
}

func TestUpdateApplyNotReleaseBuild(t *testing.T) {
	ts, pw, srv := newUpdateTestServer(t)
	srv.SetUpdater(testUpdater(fakeLatest(t, "v2.1.0").URL, "dev"))

	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/update/apply", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409", resp.StatusCode)
	}
	assertErrCode(t, resp, "not_release_build")
}

func TestShutdownRequest(t *testing.T) {
	ts, pw, srv := newUpdateTestServer(t)

	// 未注入（テスト既定）→ 503
	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/shutdown", pw, "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("未注入で status=%d, want 503", resp.StatusCode)
	}

	called := make(chan struct{}, 1)
	srv.SetShutdownRequest(func() { called <- struct{}{} })
	resp = authReq(t, http.MethodPost, ts.URL+"/api/v1/shutdown", pw, "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	select {
	case <-called:
	default:
		t.Error("終了依頼コールバックが呼ばれていません")
	}
}

func TestUpdateRequiresAuth(t *testing.T) {
	ts, _, _ := newUpdateTestServer(t)
	for _, ep := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/update/check"},
		{http.MethodPost, "/api/v1/update/apply"},
		{http.MethodPost, "/api/v1/shutdown"},
	} {
		req, _ := http.NewRequest(ep.method, ts.URL+ep.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s: status=%d, want 401", ep.method, ep.path, resp.StatusCode)
		}
	}
}
