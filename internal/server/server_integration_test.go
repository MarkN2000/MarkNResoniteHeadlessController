package server

// HTTP レベル統合テスト: poc/fakehl を実プロセスとして起動し、
// Server (httptest) 経由で 構造化API + 認証(Bearer) + メソッド/エラーマッピング を検証する。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

var fakehlPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "mrhc-server-fakehl-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir tmp: %v\n", err)
		os.Exit(1)
	}
	fakehlPath = filepath.Join(tmp, "fakehl"+exeSuffix())
	build := exec.Command("go", "build", "-o", fakehlPath, "../../poc/fakehl")
	if out, err := build.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmp)
		fmt.Fprintf(os.Stderr, "build fakehl: %v\n%s\n", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

const testPassword = "test-pass"

// newTestServer は fakehl 起動済みの Server を httptest.Server で公開する。
// 認証は Bearer パスワード（testPassword）でテストする（cookie 経路は auth_test.go）。
func newTestServer(t *testing.T) (ts *httptest.Server, pw string) {
	t.Helper()
	ts, pw, _ = newTestServerFull(t)
	return ts, pw
}

// newTestServerFull は newTestServer に加えて *Server も返す
// （テンプレ取得元の差し替え・待機短縮などフィールド調整が要るテスト用）。
func newTestServerFull(t *testing.T) (ts *httptest.Server, pw string, srv *Server) {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "integration-test-secret",
	}

	drv := headless.NewDriver(nil) // UTF-8 passthrough
	if err := drv.Start(fakehlPath, "", ""); err != nil {
		t.Fatalf("start fakehl: %v", err)
	}
	// 起動直後に強制停止クリーンアップを登録する。readiness 失敗など以降のどの経路でも
	// fakehl を孤児化させないため（Windows は親=テストバイナリ終了時に子を自動 kill しない）。
	t.Cleanup(func() { stopFakehl(drv) })

	// readiness を待つ
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if drv.Status().Ready {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !drv.Status().Ready {
		t.Fatalf("fakehl never became ready")
	}

	srv = New(cfg, "", drv, resonite.NewClient(), nil) // webFS=nil → '/' ハンドラは未登録（テストでは不要）
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close) // LIFO: ts.Close → stopFakehl の順で走る
	return ts, testPassword, srv
}

// stopFakehl は fakehl を確実に終了させる（graceful shutdown → 猶予 → pid 強制終了）。
// Windows では親(テストバイナリ)終了で子は死なないため、shutdown 取りこぼし・hang を pid kill で潰す
// （孤児 fakehl の蓄積を防ぐ）。package server からは driver の cmd に触れないため Status().PID を使う。
func stopFakehl(drv *headless.Driver) {
	if drv.Status().State == headless.StateStopped {
		return
	}
	_ = drv.SendCommand("shutdown") // fakehl は shutdown で即 os.Exit する
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if drv.Status().State == headless.StateStopped {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid := drv.Status().PID; pid > 0 {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}
}

// decodeResp は resp の JSON を target へ読み、Body を閉じてステータスを返す
// （authGet の復路と、authReq/authPost で得た POST/PUT 応答の decode を共通化）。
func decodeResp(t *testing.T, resp *http.Response, target any) int {
	t.Helper()
	defer resp.Body.Close()
	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatalf("decode: %v status=%d", err, resp.StatusCode)
		}
	}
	return resp.StatusCode
}

// authGet は Bearer 認証付き GET → JSON decode → status を返す。
func authGet(t *testing.T, url, pw string, target any) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+pw)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return decodeResp(t, resp, target)
}

// authPost は Bearer 認証付き POST を行う（呼び出し側で Body.Close する）。
func authPost(t *testing.T, url, pw, contentType, body string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(http.MethodPost, url, r)
	req.Header.Set("Authorization", "Bearer "+pw)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// 共通レスポンス形 {ok, data}
type okEnv[T any] struct {
	OK    bool `json:"ok"`
	Data  T    `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// --- 構造化API: 取得系 5 つ ---

func TestServer_Sessions_List(t *testing.T) {
	ts, pw := newTestServer(t)
	var body okEnv[[]headless.World]
	code := authGet(t, ts.URL+"/api/v1/sessions", pw, &body)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if !body.OK || len(body.Data) != 2 {
		t.Fatalf("body=%+v", body)
	}
	if body.Data[0].Name != "Fake World 0" || body.Data[1].Name != "Fake World 1" {
		t.Fatalf("unexpected: %+v", body.Data)
	}
}

func TestServer_SessionStatus(t *testing.T) {
	ts, pw := newTestServer(t)
	var body okEnv[headless.WorldStatus]
	code := authGet(t, ts.URL+"/api/v1/sessions/1/status", pw, &body)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if !body.OK || body.Data.Name != "Fake World 1" {
		t.Fatalf("body=%+v", body)
	}
	if body.Data.MaxUsers != 4 || body.Data.ResoniteLink != "off" {
		t.Fatalf("unexpected: %+v", body.Data)
	}
}

func TestServer_SessionUsers(t *testing.T) {
	ts, pw := newTestServer(t)
	var body okEnv[[]headless.UserInfo]
	code := authGet(t, ts.URL+"/api/v1/sessions/0/users", pw, &body)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if !body.OK || len(body.Data) != 1 || body.Data[0].Name != "FakeUser" {
		t.Fatalf("body=%+v", body)
	}
	if body.Data[0].Role != "Admin" {
		t.Fatalf("role: %v", body.Data[0])
	}
}

// sessionDetail は /detail のレスポンス data 部（status + users）。
type sessionDetail struct {
	Status headless.WorldStatus `json:"status"`
	Users  []headless.UserInfo  `json:"users"`
}

func TestServer_SessionDetail(t *testing.T) {
	ts, pw := newTestServer(t)
	var body okEnv[sessionDetail]
	code := authGet(t, ts.URL+"/api/v1/sessions/1/detail", pw, &body)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if !body.OK || body.Data.Status.Name != "Fake World 1" {
		t.Fatalf("status part: %+v", body.Data.Status)
	}
	if len(body.Data.Users) != 1 || body.Data.Users[0].Name != "FakeUser" {
		t.Fatalf("users part: %+v", body.Data.Users)
	}
}

func TestServer_ListBans_Empty(t *testing.T) {
	ts, pw := newTestServer(t)
	var body okEnv[[]headless.BanEntry]
	code := authGet(t, ts.URL+"/api/v1/listbans", pw, &body)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if !body.OK || len(body.Data) != 0 {
		t.Fatalf("expected empty bans, got %+v", body)
	}
}

func TestServer_FriendRequests_Empty(t *testing.T) {
	ts, pw := newTestServer(t)
	var body okEnv[[]string]
	code := authGet(t, ts.URL+"/api/v1/friendrequests", pw, &body)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if !body.OK || len(body.Data) != 0 {
		t.Fatalf("expected empty requests, got %+v", body)
	}
}

// --- 認証・メソッド・エラーマッピング ---

func TestServer_Sessions_RequiresAuth(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/sessions") // 認証ヘッダ無し
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// /start は POST 限定 → GET は 405（誤発火防止）。
func TestServer_Start_GetRejected(t *testing.T) {
	ts, pw := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/start", nil)
	req.Header.Set("Authorization", "Bearer "+pw)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

// config 空での起動は 400 config_required（無config起動は公開化するため不可）。
func TestServer_Start_RequiresConfig(t *testing.T) {
	ts, pw := newTestServer(t)
	resp := authPost(t, ts.URL+"/api/v1/start", pw, "application/json", `{"config":""}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var body okEnv[any]
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.OK || body.Error.Code != "config_required" {
		t.Fatalf("expected code=config_required, got %+v", body)
	}
}

func TestServer_SessionStatus_BadIdx(t *testing.T) {
	ts, pw := newTestServer(t)
	code := authGet(t, ts.URL+"/api/v1/sessions/notanumber/status", pw, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
}

func TestServer_SessionStatus_NegativeIdx(t *testing.T) {
	ts, pw := newTestServer(t)
	// 負の index も 400 が望ましい（パス自体は通る／ハンドラで検証）。
	code := authGet(t, ts.URL+"/api/v1/sessions/-1/status", pw, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
}

// --- ExecGroup の原子性（HTTP 経由 2 並行 status 要求）---

func TestServer_SessionStatus_Concurrent(t *testing.T) {
	// 2 並行 /sessions/0/status と /sessions/1/status が混ざらず結果を返すこと。
	ts, pw := newTestServer(t)
	type pair struct {
		idx  int
		body okEnv[headless.WorldStatus]
	}
	resCh := make(chan pair, 2)
	for _, i := range []int{0, 1} {
		go func(i int) {
			var b okEnv[headless.WorldStatus]
			_ = authGet(t, fmt.Sprintf("%s/api/v1/sessions/%d/status", ts.URL, i), pw, &b)
			resCh <- pair{i, b}
		}(i)
	}
	for n := 0; n < 2; n++ {
		p := <-resCh
		want := fmt.Sprintf("Fake World %d", p.idx)
		if !p.body.OK || p.body.Data.Name != want {
			t.Errorf("idx=%d: expected Name=%q, got %+v", p.idx, want, p.body)
		}
	}
}

// --- writeExecErr のエラーマッピング ---

func TestServer_ExecError_NotReady_After_Stop(t *testing.T) {
	// driver を停止 → /sessions が 409 (ErrNotReady→409 Conflict)
	ts, pw := newTestServer(t)

	// stop（POST）でヘッドレスを止める
	resp := authPost(t, ts.URL+"/api/v1/stop", pw, "", "")
	resp.Body.Close()

	// state=stopped まで待つ
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var b okEnv[headless.Status]
		_ = authGet(t, ts.URL+"/api/v1/status", pw, &b)
		if b.Data.State == headless.StateStopped {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// /sessions → 409
	var sb okEnv[[]headless.World]
	code := authGet(t, ts.URL+"/api/v1/sessions", pw, &sb)
	if code != http.StatusConflict {
		t.Fatalf("expected 409 (ErrNotReady), got %d", code)
	}
}

func TestServer_ExecError_Timeout(t *testing.T) {
	// fakehl の hang コマンドで stdin 処理を停止 → 後続 /sessions は timeout
	// (writeExecErr 2区分化: NotReady以外は 500 + code=timeout)
	ts, pw := newTestServer(t)

	r := authPost(t, ts.URL+"/api/v1/command?cmd=hang", pw, "", "")
	r.Body.Close()
	time.Sleep(150 * time.Millisecond)

	var body okEnv[[]headless.World]
	code := authGet(t, ts.URL+"/api/v1/sessions", pw, &body)
	if code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", code)
	}
	if body.OK || body.Error.Code != "timeout" {
		t.Fatalf("expected code=timeout, got %+v", body)
	}
}

// --- raw /command（POST限定）---

func TestServer_RawCommand_StillWorks(t *testing.T) {
	ts, pw := newTestServer(t)
	resp := authPost(t, ts.URL+"/api/v1/command?cmd=worlds", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// cmd を URL query / JSON body の 2 経路で受理（どちらも POST）。
// form-urlencoded body は対応外。
func TestServer_RawCommand_AcceptsTwoBodyForms(t *testing.T) {
	ts, pw := newTestServer(t)
	cases := []struct {
		name        string
		url         string
		contentType string
		body        string
	}{
		{"URL_query", ts.URL + "/api/v1/command?cmd=worlds", "", ""},
		{"JSON_body", ts.URL + "/api/v1/command", "application/json", `{"cmd":"worlds"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := authPost(t, tc.url, pw, tc.contentType, tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

// form-urlencoded body は対応外であることを明示的に確認 (400 が返る)
func TestServer_RawCommand_FormBodyRejected(t *testing.T) {
	ts, pw := newTestServer(t)
	resp := authPost(t, ts.URL+"/api/v1/command", pw, "application/x-www-form-urlencoded", "cmd=worlds")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 (form-urlencoded is not supported), got %d", resp.StatusCode)
	}
}

func TestServer_RawCommand_MissingCmd(t *testing.T) {
	ts, pw := newTestServer(t)
	resp := authPost(t, ts.URL+"/api/v1/command", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing cmd, got %d", resp.StatusCode)
	}
}
