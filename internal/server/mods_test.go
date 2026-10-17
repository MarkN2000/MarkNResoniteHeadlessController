package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/mods"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/steam"
)

func seedMod(t *testing.T, s *Server) string {
	t.Helper()
	// APIテスト用保存済み原本。DLL構造の検査はmodsパッケージで検証する。
	b := []byte("saved mod fixture")
	sum := sha256.Sum256(b)
	hash := hex.EncodeToString(sum[:])
	id := strings.Repeat("a", 32)
	st := mods.State{Entries: []mods.Entry{{ID: id, Enabled: true, Current: mods.Version{Files: []mods.File{{Name: "Example.dll", Hash: hash}}, AddedAt: time.Now()}}}}
	meta, _ := json.Marshal(st)
	for p, data := range map[string][]byte{".mrhc-mods/blobs/" + hash + ".dll": b, ".mrhc-mods/state.json": meta, "rml_mods/Example.dll": b, "rml_config/Example.json": []byte("settings")} {
		full := filepath.Join(s.modDir(), filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestModsHTTP(t *testing.T) {
	ts, pw, _, s := newSteamServer(t)
	id := seedMod(t, s)
	r, err := http.Get(ts.URL + "/api/v1/mods")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("未認証を受理した", r.StatusCode)
	}
	for _, enabled := range []bool{false, true} {
		body, _ := json.Marshal(map[string]bool{"enabled": enabled})
		r = authReq(t, http.MethodPatch, ts.URL+"/api/v1/mods/"+id, pw, "application/json", string(body))
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatal(r.StatusCode)
		}
		_, err = os.Stat(filepath.Join(s.modDir(), "rml_mods", "Example.dll"))
		if enabled && err != nil || !enabled && !errors.Is(err, os.ErrNotExist) {
			t.Fatal("ON/OFF未反映", err)
		}
	}
	r = authReq(t, http.MethodPost, ts.URL+"/api/v1/mods/upload?name=Bad.dll", pw, "application/octet-stream", "not a DLL")
	r.Body.Close()
	if r.StatusCode != 400 {
		t.Fatal(r.StatusCode)
	}
	st, err := s.modRead()
	if err != nil || len(st.Entries) != 1 {
		t.Fatal("不正アップロードで既存構成が変化", err)
	}
	r = authReq(t, http.MethodDelete, ts.URL+"/api/v1/mods/"+id, pw, "", "")
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	if _, err = os.Stat(filepath.Join(s.modDir(), "rml_config", "Example.json")); err != nil {
		t.Fatal("設定ファイルを削除した", err)
	}
}

func TestModChangesExcludeStartUpdateAndRestart(t *testing.T) {
	ts, pw, _, s := newSteamServer(t)
	id := seedMod(t, s)
	s.operationMu.Lock()
	for _, p := range []string{"/mods/" + id + "/restore", "/start", "/steam/download"} {
		r := authReq(t, http.MethodPost, ts.URL+"/api/v1"+p, pw, "application/json", `{"config":"w"}`)
		r.Body.Close()
		if r.StatusCode != 409 {
			t.Error(p, r.StatusCode)
		}
	}
	if err := s.crashMon.start("w"); !errors.Is(err, errModBusy) {
		t.Error("クラッシュ復帰が排他されない", err)
	}
	if err := s.restart.start("x", "y", "w"); !errors.Is(err, errModBusy) {
		t.Error("予定起動が排他されない", err)
	}
	if err := s.restart.startFlow(config.Restart{}, "w", "manual", false); !errors.Is(err, errModBusy) {
		t.Error("再起動受付が排他されない", err)
	}
	s.operationMu.Unlock()
	for _, cause := range []string{"steam", "restart"} {
		s.steamRunning = func() bool { return cause == "steam" }
		s.restart.mu.Lock()
		s.restart.p.inProgress = cause == "restart"
		s.restart.mu.Unlock()
		r := authReq(t, http.MethodPatch, ts.URL+"/api/v1/mods/"+id, pw, "application/json", `{"enabled":false}`)
		r.Body.Close()
		if r.StatusCode != 409 {
			t.Error(cause, r.StatusCode)
		}
	}
}

func TestModChangeRejectedWhileRunning(t *testing.T) {
	ts, pw, _, s := newSteamServer(t)
	id := seedMod(t, s)
	if err := s.driver.Start(fakehlPath, "", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopFakehl(s.driver) })
	r := authReq(t, http.MethodPatch, ts.URL+"/api/v1/mods/"+id, pw, "application/json", `{"enabled":false}`)
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatal("稼働中にMod変更できる", r.StatusCode)
	}
}

func TestModChangeRejectedDuringAsyncStartPreparation(t *testing.T) {
	ts, pw, s, dir := newGuardServer(t)
	id := seedMod(t, s)
	writeHeadlessStub(t, dir)
	applyGuardSeams(s, guardSeams{readOK: true})
	entered, release := make(chan struct{}), make(chan struct{})
	s.installRuntime = func(context.Context, string) error { close(entered); <-release; return steam.ErrCancelled }
	t.Cleanup(func() { close(release); s.operationMu.Lock(); s.operationMu.Unlock() })
	r := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-configs/w", pw, "application/json", `{"startWorlds":[{"sessionName":"W"}]}`)
	r.Body.Close()
	r = authReq(t, http.MethodPost, ts.URL+"/api/v1/start", pw, "application/json", `{"config":"w"}`)
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("起動準備に入らない")
	}
	r = authReq(t, http.MethodPatch, ts.URL+"/api/v1/mods/"+id, pw, "application/json", `{"enabled":false}`)
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatal("起動準備中にMod変更できる", r.StatusCode)
	}
}

type modTransport func(*http.Request) (*http.Response, error)

func (f modTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModCheckFailureAndUnknownVersion(t *testing.T) {
	ts, pw, _, s := newSteamServer(t)
	id := seedMod(t, s)
	s.modGitHub.Client = &http.Client{Transport: modTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"tag_name":"v2","assets":[{"name":"Example.dll"}]}`))}, nil
	})}
	r := authReq(t, http.MethodPatch, ts.URL+"/api/v1/mods/"+id, pw, "application/json", `{"repo":"https://github.com/owner/repo","tag":"v2","asset":"Example.dll"}`)
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	st, err := s.modRead()
	if err != nil || st.Entries[0].Current.Source != nil {
		t.Fatal("更新元登録で導入版が変わった", err)
	}
	r = authReq(t, http.MethodPost, ts.URL+"/api/v1/mods/check", pw, "application/json", `{"id":"`+id+`"}`)
	var got okEnv[map[string]any]
	_ = json.NewDecoder(r.Body).Decode(&got)
	r.Body.Close()
	if got.Data["versionUnknown"] != true || got.Data["asset"] != "Example.dll" {
		t.Fatal(got)
	}
	s.modGitHub.Client = &http.Client{Transport: modTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
	})}
	r = authReq(t, http.MethodPost, ts.URL+"/api/v1/mods/check", pw, "application/json", `{"id":"`+id+`"}`)
	r.Body.Close()
	if r.StatusCode != 502 {
		t.Fatal("取得失敗を更新なしにした", r.StatusCode)
	}
	r = authReq(t, http.MethodPost, ts.URL+"/api/v1/mods/github", pw, "application/json", `{"id":"`+id+`","repo":"owner/repo","tag":"v2","asset":"Example.dll"}`)
	r.Body.Close()
	if r.StatusCode != 502 {
		t.Fatal(r.StatusCode)
	}
	newState, err := s.modRead()
	if err != nil || newState.Entries[0].Current.Files[0].Hash != st.Entries[0].Current.Files[0].Hash {
		t.Fatal("取得失敗で構成を変更した", err)
	}
}
