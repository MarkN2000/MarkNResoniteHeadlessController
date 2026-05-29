package server

// write API（Pre-7c）の HTTP 統合テスト。poc/fakehl を実プロセスとして起動し、
// 代表エンドポイントの正常系（方針A: executed=true + 状態再取得）・入力検証（400）・
// NotReady（409）を検証する。helper（newTestServer/authGet/authPost/okEnv）は
// server_integration_test.go と共有。

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
)

// postJSON は Bearer 認証付き JSON POST → status と {ok,data} を返す。
func postJSON(t *testing.T, url, pw, body string) (int, okEnv[map[string]any]) {
	t.Helper()
	resp := authPost(t, url, pw, "application/json", body)
	defer resp.Body.Close()
	var env okEnv[map[string]any]
	_ = json.NewDecoder(resp.Body).Decode(&env)
	return resp.StatusCode, env
}

// 正常系: kick が executed=true を返し、再取得した users から対象が消える（状態反映）。
func TestServer_Write_Kick_RemovesUser(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/0/kick", pw, `{"user":"FakeUser"}`)
	if code != http.StatusOK || !env.OK || env.Data["executed"] != true {
		t.Fatalf("kick failed: code=%d env=%+v", code, env)
	}
	// 再取得で実状態を確認（方針A）。
	var users okEnv[[]headless.UserInfo]
	authGet(t, ts.URL+"/api/v1/sessions/0/users", pw, &users)
	if len(users.Data) != 0 {
		t.Fatalf("kicked user should be gone, got %+v", users.Data)
	}
}

// 正常系: accesslevel が反映される（status 再取得で確認）。
func TestServer_Write_AccessLevel(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/0/accesslevel", pw, `{"level":"LAN"}`)
	if code != http.StatusOK || !env.OK {
		t.Fatalf("accesslevel failed: code=%d env=%+v", code, env)
	}
	var st okEnv[headless.WorldStatus]
	authGet(t, ts.URL+"/api/v1/sessions/0/status", pw, &st)
	if st.Data.AccessLevel != "LAN" {
		t.Fatalf("accessLevel not applied: %+v", st.Data)
	}
}

// 正常系: maxusers が反映される。
func TestServer_Write_MaxUsers(t *testing.T) {
	ts, pw := newTestServer(t)
	code, _ := postJSON(t, ts.URL+"/api/v1/sessions/0/maxusers", pw, `{"maxUsers":8}`)
	if code != http.StatusOK {
		t.Fatalf("maxusers failed: code=%d", code)
	}
	var st okEnv[headless.WorldStatus]
	authGet(t, ts.URL+"/api/v1/sessions/0/status", pw, &st)
	if st.Data.MaxUsers != 8 {
		t.Fatalf("maxUsers not applied: %+v", st.Data)
	}
}

// 正常系: sessions/start (url) で新規ワールドが追加される。
func TestServer_Write_Start_AddsWorld(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/start", pw, `{"mode":"url","url":"res-steam://link/S-xyz"}`)
	if code != http.StatusOK || !env.OK {
		t.Fatalf("start failed: code=%d env=%+v", code, env)
	}
	var list okEnv[[]headless.World]
	authGet(t, ts.URL+"/api/v1/sessions", pw, &list)
	if len(list.Data) != 3 { // 初期 2 + 追加 1
		t.Fatalf("expected 3 worlds, got %d: %+v", len(list.Data), list.Data)
	}
}

// 正常系: グローバル unban（focus 不要）。bans は空だが executed=true を返す（方針A）。
func TestServer_Write_Unban(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/bans/unban", pw, `{"userId":"U-1NzqeqewOpM"}`)
	if code != http.StatusOK || env.Data["executed"] != true {
		t.Fatalf("unban failed: code=%d env=%+v", code, env)
	}
}

// 正常系: グローバル friend 追加（focus 不要）。
func TestServer_Write_FriendAdd(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/friends/add", pw, `{"user":"someone"}`)
	if code != http.StatusOK || env.Data["executed"] != true {
		t.Fatalf("friend add failed: code=%d env=%+v", code, env)
	}
}

// 入力検証: 空 user → 400 missing_field。
func TestServer_Write_EmptyUser_400(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/0/kick", pw, `{"user":""}`)
	if code != http.StatusBadRequest || env.Error.Code != "missing_field" {
		t.Fatalf("expected 400 missing_field, got code=%d env=%+v", code, env)
	}
}

// 入力検証: accesslevel に不正トークン（空白・記号）→ 400 invalid_value。
func TestServer_Write_InvalidLevel_400(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/0/accesslevel", pw, `{"level":"bad level!"}`)
	if code != http.StatusBadRequest || env.Error.Code != "invalid_value" {
		t.Fatalf("expected 400 invalid_value, got code=%d env=%+v", code, env)
	}
}

// 入力検証: maxUsers <= 0 → 400 invalid_value。
func TestServer_Write_NonPositiveMaxUsers_400(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/0/maxusers", pw, `{"maxUsers":0}`)
	if code != http.StatusBadRequest || env.Error.Code != "invalid_value" {
		t.Fatalf("expected 400 invalid_value, got code=%d env=%+v", code, env)
	}
}

// 入力検証: sessions/start の mode 不正 → 400 invalid_value。
func TestServer_Write_Start_BadMode_400(t *testing.T) {
	ts, pw := newTestServer(t)
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/start", pw, `{"mode":"bogus"}`)
	if code != http.StatusBadRequest || env.Error.Code != "invalid_value" {
		t.Fatalf("expected 400 invalid_value, got code=%d env=%+v", code, env)
	}
}

// メソッド: write は POST 限定 → GET は 405。
func TestServer_Write_GetRejected_405(t *testing.T) {
	ts, pw := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/sessions/0/kick", nil)
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

// 認証: 認証ヘッダ無し → 401。
func TestServer_Write_RequiresAuth_401(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/v1/sessions/0/kick", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// NotReady: driver 停止後の write は 409（ErrNotReady→409）。
func TestServer_Write_NotReady_409(t *testing.T) {
	ts, pw := newTestServer(t)

	resp := authPost(t, ts.URL+"/api/v1/stop", pw, "", "")
	resp.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var b okEnv[headless.Status]
		_ = authGet(t, ts.URL+"/api/v1/status", pw, &b)
		if b.Data.State == headless.StateStopped {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	code, env := postJSON(t, ts.URL+"/api/v1/sessions/0/kick", pw, `{"user":"x"}`)
	if code != http.StatusConflict || env.Error.Code != "not_ready" {
		t.Fatalf("expected 409 not_ready, got code=%d env=%+v", code, env)
	}
}
