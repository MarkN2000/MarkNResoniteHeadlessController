package server

// HTTP レベル統合テスト: poc/fakehl を実プロセスとして起動し、
// Server (httptest) 経由で 5 つの構造化API + 認証 + エラーマッピング を検証する。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
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

// newTestServer は fakehl 起動済みの Server を httptest.Server で公開する。
// 認証は APIKey 経由（クエリ ?apiKey=...）でテストする（cookie 経路は別途）。
func newTestServer(t *testing.T) (ts *httptest.Server, apiKey string) {
	t.Helper()
	apiKey = "test-key"
	cfg := &config.Config{APIKey: apiKey}

	drv := headless.NewDriver(nil) // UTF-8 passthrough
	if err := drv.Start(fakehlPath, ""); err != nil {
		t.Fatalf("start fakehl: %v", err)
	}

	// readiness を待つ
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if drv.Status().Ready {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !drv.Status().Ready {
		_ = drv.SendCommand("shutdown")
		t.Fatalf("fakehl never became ready")
	}

	srv := New(cfg, "", drv, nil) // webFS=nil → '/' ハンドラは未登録（テストでは不要）
	ts = httptest.NewServer(srv.Handler())

	t.Cleanup(func() {
		ts.Close()
		_ = drv.SendCommand("shutdown")
		// 軽く待つ。fakehl は shutdown で即 os.Exit する。
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if drv.Status().State == headless.StateStopped {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
	return ts, apiKey
}

// getJSON は GET → JSON decode → status を返す。
func getJSON(t *testing.T, url string, target any) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatalf("decode: %v body status=%d", err, resp.StatusCode)
		}
	}
	return resp.StatusCode
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
	ts, key := newTestServer(t)
	var body okEnv[[]headless.World]
	code := getJSON(t, ts.URL+"/api/v1/sessions?apiKey="+key, &body)
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
	ts, key := newTestServer(t)
	var body okEnv[headless.WorldStatus]
	code := getJSON(t, ts.URL+"/api/v1/sessions/1/status?apiKey="+key, &body)
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
	ts, key := newTestServer(t)
	var body okEnv[[]headless.UserInfo]
	code := getJSON(t, ts.URL+"/api/v1/sessions/0/users?apiKey="+key, &body)
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

func TestServer_ListBans_Empty(t *testing.T) {
	ts, key := newTestServer(t)
	var body okEnv[[]headless.BanEntry]
	code := getJSON(t, ts.URL+"/api/v1/listbans?apiKey="+key, &body)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if !body.OK || len(body.Data) != 0 {
		t.Fatalf("expected empty bans, got %+v", body)
	}
}

func TestServer_FriendRequests_Empty(t *testing.T) {
	ts, key := newTestServer(t)
	var body okEnv[[]string]
	code := getJSON(t, ts.URL+"/api/v1/friendrequests?apiKey="+key, &body)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if !body.OK || len(body.Data) != 0 {
		t.Fatalf("expected empty requests, got %+v", body)
	}
}

// --- 認証・エラーマッピング ---

func TestServer_Sessions_RequiresAuth(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/sessions") // apiKey 無し
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestServer_SessionStatus_BadIdx(t *testing.T) {
	ts, key := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/sessions/notanumber/status?apiKey=" + key)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestServer_SessionStatus_NegativeIdx(t *testing.T) {
	ts, key := newTestServer(t)
	// 負の index も 400 が望ましい（パス自体は通る／ハンドラで検証）。
	// ただしGo 1.22+ の path patterns では "{idx}" は "-1" を含む文字列に
	// マッチするので、ハンドラの strconv.Atoi で受けて検証する。
	resp, err := http.Get(ts.URL + "/api/v1/sessions/-1/status?apiKey=" + key)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// --- ExecGroup の原子性（HTTP 経由 2 並行 status 要求）---

func TestServer_SessionStatus_Concurrent(t *testing.T) {
	// 2 並行 /sessions/0/status と /sessions/1/status が混ざらず結果を返すこと。
	// （内部の ExecGroup + execMu で直列化されるはず）
	ts, key := newTestServer(t)
	type pair struct {
		idx  int
		body okEnv[headless.WorldStatus]
	}
	resCh := make(chan pair, 2)
	for _, i := range []int{0, 1} {
		go func(i int) {
			var b okEnv[headless.WorldStatus]
			_ = getJSON(t, fmt.Sprintf("%s/api/v1/sessions/%d/status?apiKey=%s", ts.URL, i, key), &b)
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

// --- writeExecErr のエラーマッピング確認（Phase 7 前レビューで追加）---

func TestServer_ExecError_NotReady_After_Stop(t *testing.T) {
	// driver を停止 → /sessions が 409 (ErrNotReady→409 Conflict)
	ts, key := newTestServer(t)

	// stop 経由でヘッドレスを止める（cleanup より先に明示停止）
	resp, err := http.Post(ts.URL+"/api/v1/stop?apiKey="+key, "", nil)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	resp.Body.Close()

	// state=stopped まで待つ
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := http.Get(ts.URL + "/api/v1/status?apiKey=" + key)
		if err != nil {
			break
		}
		var b struct {
			Data struct {
				State string `json:"state"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		r.Body.Close()
		if b.Data.State == "stopped" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// /sessions → 409
	r, err := http.Get(ts.URL + "/api/v1/sessions?apiKey=" + key)
	if err != nil {
		t.Fatalf("GET /sessions: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 (ErrNotReady), got %d", r.StatusCode)
	}
}

func TestServer_ExecError_Timeout(t *testing.T) {
	// fakehl の hang コマンドで stdin 処理を停止 → 後続 /sessions は timeout
	// (writeExecErr 2区分化: NotReady以外は 500 + code=timeout)
	ts, key := newTestServer(t)

	r, err := http.Post(ts.URL+"/api/v1/command?apiKey="+key+"&cmd=hang", "", nil)
	if err != nil {
		t.Fatalf("send hang: %v", err)
	}
	r.Body.Close()
	time.Sleep(150 * time.Millisecond)

	r2, err := http.Get(ts.URL + "/api/v1/sessions?apiKey=" + key)
	if err != nil {
		t.Fatalf("GET /sessions: %v", err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", r2.StatusCode)
	}
	// error code が "timeout" であること
	var body struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(r2.Body).Decode(&body)
	if body.OK || body.Error.Code != "timeout" {
		t.Fatalf("expected code=timeout, got %+v", body)
	}
}

// --- raw /command は変更なし（既存仕様の回帰確認）---

func TestServer_RawCommand_StillWorks(t *testing.T) {
	ts, key := newTestServer(t)
	// raw /command は SendCommand 経由（fire-and-forget・応答取らず）
	resp, err := http.Post(ts.URL+"/api/v1/command?apiKey="+key+"&cmd=worlds", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// /command の cmd を URL query / JSON body の 2 経路で受理することを確認。
// form-urlencoded body は対応外（呼び出し側は URL query を使う）。
func TestServer_RawCommand_AcceptsTwoBodyForms(t *testing.T) {
	ts, key := newTestServer(t)
	cases := []struct {
		name        string
		url         string
		contentType string
		body        string
	}{
		{"URL_query", ts.URL + "/api/v1/command?apiKey=" + key + "&cmd=worlds", "", ""},
		{"JSON_body", ts.URL + "/api/v1/command?apiKey=" + key, "application/json", `{"cmd":"worlds"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp *http.Response
			var err error
			if tc.body == "" {
				resp, err = http.Post(tc.url, "", nil)
			} else {
				resp, err = http.Post(tc.url, tc.contentType, strings.NewReader(tc.body))
			}
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

// form-urlencoded body は対応外であることを明示的に確認 (400 が返る)
func TestServer_RawCommand_FormBodyRejected(t *testing.T) {
	ts, key := newTestServer(t)
	resp, err := http.Post(
		ts.URL+"/api/v1/command?apiKey="+key,
		"application/x-www-form-urlencoded",
		strings.NewReader("cmd=worlds"),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 (form-urlencoded is not supported), got %d", resp.StatusCode)
	}
}

func TestServer_RawCommand_MissingCmd(t *testing.T) {
	ts, key := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/v1/command?apiKey="+key, "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing cmd, got %d", resp.StatusCode)
	}
}
