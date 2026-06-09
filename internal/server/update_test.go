package server

// 自己更新 HTTP 層のテスト。スワップ・抽出の実体は internal/selfupdate の単体で網羅済みのため、
// ここでは HTTP セマンティクス（応答形・errCode マップ・staged・SSE 進捗・restart 依頼）のみ検証する。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/selfupdate"
)

// テストサーバーは steam_test.go の newSteamServer を共用する（同構成の重複を作らない）。

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

// applySSEResult は apply の SSE 応答を読み、最終の update-result/update-error の
// イベント名と data(JSON) を返す（body は consume される）。
func applySSEResult(t *testing.T, resp *http.Response) (event string, data map[string]any) {
	t.Helper()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type=%q, want text/event-stream", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range strings.Split(string(body), "\n\n") {
		var ev, d string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				ev = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				d += strings.TrimSpace(line[len("data:"):])
			}
		}
		if ev == "update-result" || ev == "update-error" {
			event = ev
			data = map[string]any{}
			_ = json.Unmarshal([]byte(d), &data)
		}
	}
	if event == "" {
		t.Fatalf("update-result/error イベントが見つかりません: %q", body)
	}
	return event, data
}

func TestUpdateCheck(t *testing.T) {
	ts, pw, _, srv := newSteamServer(t)
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

// GitHub 不達・リリース未公開でも check は 200 で返す（staged 等のローカル情報を消さない）。
func TestUpdateCheckNoRelease(t *testing.T) {
	ts, pw, _, srv := newSteamServer(t)
	srv.SetUpdater(testUpdater(fakeLatest(t, "").URL, "v2.0.0")) // リリース未公開

	var env okEnv[updateCheckResp]
	if code := authGet(t, ts.URL+"/api/v1/update/check", pw, &env); code != http.StatusOK {
		t.Fatalf("status=%d, want 200（ローカル情報は GitHub 不達でも返す）", code)
	}
	d := env.Data
	if !d.CheckFailed || d.CheckError != "no_release" {
		t.Errorf("checkFailed/checkError = %v/%q, want true/no_release", d.CheckFailed, d.CheckError)
	}
	if d.Current != "v2.0.0" || d.Goos == "" || d.UpdateAvailable {
		t.Errorf("ローカル情報が想定外: %+v", d)
	}
}

// 適用済み（staged）の表示と「今すぐ終了」導線は GitHub 不達でも UI に残るべき
// ＝check 失敗応答にも staged が載ることを検証する。
func TestUpdateCheckFailedKeepsStaged(t *testing.T) {
	ts, pw, _, srv := newSteamServer(t)
	srv.SetUpdater(testUpdater(fakeLatest(t, "").URL, "v2.0.0"))
	srv.setStagedVersion("v2.1.0")

	var env okEnv[updateCheckResp]
	if code := authGet(t, ts.URL+"/api/v1/update/check", pw, &env); code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	if env.Data.Staged != "v2.1.0" || !env.Data.CheckFailed {
		t.Errorf("staged/checkFailed = %q/%v, want v2.1.0/true", env.Data.Staged, env.Data.CheckFailed)
	}
}

// apply はエラーも SSE（update-error・HTTP 200）で返す（httptest の writer は http.Flusher）。
func TestUpdateApplyUpToDate(t *testing.T) {
	ts, pw, _, srv := newSteamServer(t)
	srv.SetUpdater(testUpdater(fakeLatest(t, "v2.0.0").URL, "v2.0.0")) // 最新 == 現行

	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/update/apply", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200（SSE）", resp.StatusCode)
	}
	ev, data := applySSEResult(t, resp)
	if ev != "update-error" || data["code"] != "up_to_date" {
		t.Errorf("event/code = %q/%v, want update-error/up_to_date", ev, data["code"])
	}
}

func TestUpdateApplyNotReleaseBuild(t *testing.T) {
	ts, pw, _, srv := newSteamServer(t)
	srv.SetUpdater(testUpdater(fakeLatest(t, "v2.1.0").URL, "dev"))

	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/update/apply", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200（SSE）", resp.StatusCode)
	}
	ev, data := applySSEResult(t, resp)
	if ev != "update-error" || data["code"] != "not_release_build" {
		t.Errorf("event/code = %q/%v, want update-error/not_release_build", ev, data["code"])
	}
}

// 適用済み（staged==latest）の再 apply は再DLせず update-result を即返す（冪等・SSE）。
func TestUpdateApplyStagedStreamsResult(t *testing.T) {
	ts, pw, _, srv := newSteamServer(t)
	srv.SetUpdater(testUpdater(fakeLatest(t, "v2.1.0").URL, "v2.0.0"))
	srv.setStagedVersion("v2.1.0")

	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/update/apply", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	ev, data := applySSEResult(t, resp)
	if ev != "update-result" || data["staged"] != "v2.1.0" {
		t.Errorf("event/staged = %q/%v, want update-result/v2.1.0", ev, data["staged"])
	}
}

// check は TTL 内はキャッシュで返す（GitHub を落としても latest を保つ）。
func TestUpdateCheckCached(t *testing.T) {
	ts, pw, _, srv := newSteamServer(t)
	gh := fakeLatest(t, "v2.1.0")
	srv.SetUpdater(testUpdater(gh.URL, "v2.0.0"))

	var env okEnv[updateCheckResp]
	if code := authGet(t, ts.URL+"/api/v1/update/check", pw, &env); code != http.StatusOK || !env.Data.UpdateAvailable {
		t.Fatalf("1回目の check が失敗: code=%d data=%+v", code, env.Data)
	}
	gh.Close() // GitHub 不達にしても TTL 内はキャッシュで返るはず
	if code := authGet(t, ts.URL+"/api/v1/update/check", pw, &env); code != http.StatusOK {
		t.Fatalf("2回目の check status=%d, want 200", code)
	}
	if env.Data.Latest != "v2.1.0" || !env.Data.UpdateAvailable || env.Data.CheckFailed {
		t.Errorf("キャッシュ応答が想定外: %+v", env.Data)
	}
}

func TestRestartRequest(t *testing.T) {
	ts, pw, _, srv := newSteamServer(t)

	// 未注入（テスト既定）→ 503
	resp := authReq(t, http.MethodPost, ts.URL+"/api/v1/restart", pw, "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("未注入で status=%d, want 503", resp.StatusCode)
	}

	called := make(chan struct{}, 1)
	srv.SetRestartRequest(func() { called <- struct{}{} })
	resp = authReq(t, http.MethodPost, ts.URL+"/api/v1/restart", pw, "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	select {
	case <-called:
	default:
		t.Error("再起動依頼コールバックが呼ばれていません")
	}
}

func TestUpdateRequiresAuth(t *testing.T) {
	ts, _, _, _ := newSteamServer(t)
	for _, ep := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/update/check"},
		{http.MethodPost, "/api/v1/update/apply"},
		{http.MethodPost, "/api/v1/restart"},
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
