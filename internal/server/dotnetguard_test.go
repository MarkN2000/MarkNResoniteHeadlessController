package server

// 起動時ガード（.NET ランタイム自動設置→起動）のテスト。
// startWithRuntimeGuard は handleStart から goroutine で包まれるため、ここでは同期に呼び、
// seam（readRuntimeReq/localSatisfies/systemSatisfies/installRuntime/steamRunning）を偽装して
// sys ログ（UI コンソール）と副作用を検証する。driver.Start は実在しないパスで必ず失敗する＝
// 「起動を試みた」ことの観測点として sysStartFailed 行を使う。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/steam"
)

var testReq = platform.RuntimeRequirement{Major: 10, Minor: 0, Patch: 0, Raw: "10.0.0"}

// newGuardServer は newConfigServer と同型だが、seam を差し替えるため *Server も返す。
func newGuardServer(t *testing.T) (ts *httptest.Server, pw string, srv *Server, dataDir string) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "guardtest-secret",
		HeadlessConfigDir: filepath.Join(tmp, "configs"),
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	srv = New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, testPassword, srv, tmp
}

// writeHeadlessStub は headless_not_installed 判定を通すための実体ファイルを置く。
func writeHeadlessStub(t *testing.T, dataDir string) {
	t.Helper()
	p := filepath.Join(dataDir, "resonite", "Headless", platform.HeadlessBinaryName())
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// guardSeams は Server のガード seam を一括で偽装する。
type guardSeams struct {
	readOK       bool
	local        bool
	system       bool
	installErr   error
	installCalls *int
	installDir   *string
	steamBusy    bool
}

func applyGuardSeams(s *Server, g guardSeams) {
	s.readRuntimeReq = func(string) (platform.RuntimeRequirement, bool) { return testReq, g.readOK }
	s.localSatisfies = func(string, platform.RuntimeRequirement, string) bool { return g.local }
	s.systemSatisfies = func(string, string, platform.RuntimeRequirement) bool { return g.system }
	s.installRuntime = func(_ context.Context, dir string) error {
		if g.installCalls != nil {
			*g.installCalls++
		}
		if g.installDir != nil {
			*g.installDir = dir
		}
		return g.installErr
	}
	s.steamRunning = func() bool { return g.steamBusy }
}

// drainSysLines は購読チャンネルに溜まった sys 行を全て取り出す（同期呼び出し後に使う）。
func drainSysLines(ch chan headless.LogLine) []string {
	var lines []string
	for {
		select {
		case l := <-ch:
			if l.Kind == "sys" {
				lines = append(lines, l.Text)
			}
		default:
			return lines
		}
	}
}

func hasLine(lines []string, substr string) bool {
	for _, l := range lines {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func TestRuntimeGuardNeeded_Matrix(t *testing.T) {
	headlessPath := filepath.Join(t.TempDir(), "resonite", "Headless", "Resonite.exe")
	cases := []struct {
		name string
		g    guardSeams
		want bool
	}{
		{"要求が読めない（fakehl 等）→ 同期経路", guardSeams{readOK: false}, false},
		{"ローカル充足 → 同期経路", guardSeams{readOK: true, local: true}, false},
		{"不足 → ガード経路", guardSeams{readOK: true, local: false}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newPathServer(t, nil)
			applyGuardSeams(s, tc.g)
			if got := s.runtimeGuardNeeded(headlessPath); got != tc.want {
				t.Errorf("runtimeGuardNeeded = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("システム充足キャッシュ命中 → 同期経路", func(t *testing.T) {
		s, _ := newPathServer(t, nil)
		applyGuardSeams(s, guardSeams{readOK: true, local: false})
		installDir := filepath.Dir(filepath.Dir(headlessPath))
		s.cacheSysDotnet(installDir, testReq)
		if s.runtimeGuardNeeded(headlessPath) {
			t.Error("キャッシュ命中でもガード経路に入った")
		}
	})
}

func TestStartWithRuntimeGuard_SystemSatisfiedCachesAndStarts(t *testing.T) {
	s, dataDir := newPathServer(t, nil)
	calls := 0
	applyGuardSeams(s, guardSeams{readOK: true, local: false, system: true, installCalls: &calls})
	headlessPath := filepath.Join(dataDir, "resonite", "Headless", "Resonite.exe") // 実在しない
	ch, _ := s.driver.SubscribeLog(16)
	defer s.driver.UnsubscribeLog(ch)

	s.startWithRuntimeGuard("w", headlessPath, "")

	lines := drainSysLines(ch)
	if calls != 0 {
		t.Error("システム充足なのに installRuntime が呼ばれた")
	}
	if !hasLine(lines, "起動に失敗") {
		t.Errorf("起動を試みるべき（sysStartFailed が無い）: %v", lines)
	}
	if s.runtimeGuardNeeded(headlessPath) {
		t.Error("システム充足がキャッシュされていない（次回も非同期経路になる）")
	}
}

func TestStartWithRuntimeGuard_InstallSuccessThenStart(t *testing.T) {
	s, dataDir := newPathServer(t, nil)
	calls := 0
	gotDir := ""
	applyGuardSeams(s, guardSeams{readOK: true, local: false, system: false, installCalls: &calls, installDir: &gotDir})
	headlessPath := filepath.Join(dataDir, "resonite", "Headless", "Resonite.exe")
	ch, _ := s.driver.SubscribeLog(16)
	defer s.driver.UnsubscribeLog(ch)

	s.startWithRuntimeGuard("w", headlessPath, "")

	lines := drainSysLines(ch)
	if calls != 1 {
		t.Errorf("installRuntime 呼び出し回数 = %d, want 1", calls)
	}
	if want := filepath.Join(dataDir, "resonite"); gotDir != want {
		t.Errorf("installDir = %q, want %q", gotDir, want)
	}
	if !hasLine(lines, "設置します") || !hasLine(lines, "設置が完了") {
		t.Errorf("設置中/完了の sys ログが無い: %v", lines)
	}
	if !hasLine(lines, "起動に失敗") {
		t.Errorf("設置成功後に起動を試みるべき: %v", lines)
	}
}

func TestStartWithRuntimeGuard_InstallFailedBestEffortStart(t *testing.T) {
	s, dataDir := newPathServer(t, nil)
	applyGuardSeams(s, guardSeams{readOK: true, local: false, installErr: errors.New("HTTP 503")})
	headlessPath := filepath.Join(dataDir, "resonite", "Headless", "Resonite.exe")
	ch, _ := s.driver.SubscribeLog(16)
	defer s.driver.UnsubscribeLog(ch)

	s.startWithRuntimeGuard("w", headlessPath, "")

	lines := drainSysLines(ch)
	if !hasLine(lines, "手動導入") {
		t.Errorf("失敗＋手動導入の案内が無い: %v", lines)
	}
	// 失敗しても best-effort で起動を試みる（システム .NET で動く環境を壊さない）
	if !hasLine(lines, "起動に失敗") {
		t.Errorf("設置失敗でも起動を試みるべき: %v", lines)
	}
}

func TestStartWithRuntimeGuard_CancelledStopsQuietly(t *testing.T) {
	s, dataDir := newPathServer(t, nil)
	applyGuardSeams(s, guardSeams{readOK: true, local: false, installErr: steam.ErrCancelled})
	headlessPath := filepath.Join(dataDir, "resonite", "Headless", "Resonite.exe")
	ch, _ := s.driver.SubscribeLog(16)
	defer s.driver.UnsubscribeLog(ch)

	s.startWithRuntimeGuard("w", headlessPath, "")

	lines := drainSysLines(ch)
	if !hasLine(lines, "中止しました") {
		t.Errorf("中止の中立文言が無い: %v", lines)
	}
	if hasLine(lines, "起動に失敗") {
		t.Errorf("中止後に起動を試みるべきでない: %v", lines)
	}
}

func TestStartWithRuntimeGuard_UpdateInProgressStops(t *testing.T) {
	s, dataDir := newPathServer(t, nil)
	applyGuardSeams(s, guardSeams{readOK: true, local: false, installErr: steam.ErrUpdateInProgress})
	headlessPath := filepath.Join(dataDir, "resonite", "Headless", "Resonite.exe")
	ch, _ := s.driver.SubscribeLog(16)
	defer s.driver.UnsubscribeLog(ch)

	s.startWithRuntimeGuard("w", headlessPath, "")

	lines := drainSysLines(ch)
	if !hasLine(lines, "進行中のため起動を見送りました") {
		t.Errorf("更新進行中の案内が無い: %v", lines)
	}
	if hasLine(lines, "起動に失敗") {
		t.Errorf("更新進行中は起動を試みるべきでない: %v", lines)
	}
}

func TestStartWithRuntimeGuard_SteamBusyDefersStart(t *testing.T) {
	s, dataDir := newPathServer(t, nil)
	applyGuardSeams(s, guardSeams{readOK: true, local: false, steamBusy: true})
	headlessPath := filepath.Join(dataDir, "resonite", "Headless", "Resonite.exe")
	ch, _ := s.driver.SubscribeLog(16)
	defer s.driver.UnsubscribeLog(ch)

	s.startWithRuntimeGuard("w", headlessPath, "")

	lines := drainSysLines(ch)
	if !hasLine(lines, "始まったため起動を見送りました") {
		t.Errorf("更新割込時の見送り案内が無い: %v", lines)
	}
	if hasLine(lines, "起動に失敗") {
		t.Errorf("更新中に起動を試みるべきでない: %v", lines)
	}
}

// TestHandleStart_RuntimeGuardAccepted は不足時の handleStart が受付（runtimePrepare）を返すことを
// HTTP 経由で確認する。goroutine 側は installRuntime=ErrUpdateInProgress で安全に打ち切らせ、
// sys ログの到着を完了の同期点に使う。
func TestHandleStart_RuntimeGuardAccepted(t *testing.T) {
	ts, pw, srv, dataDir := newGuardServer(t)
	calls := 0
	applyGuardSeams(srv, guardSeams{readOK: true, local: false, installErr: steam.ErrUpdateInProgress, installCalls: &calls})

	// 起動できる config と、headless_not_installed を回避する実体ファイルを用意。
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-configs/w", pw, "application/json",
		`{"startWorlds":[{"sessionName":"W"}]}`)
	resp.Body.Close()
	writeHeadlessStub(t, dataDir)

	ch, _ := srv.driver.SubscribeLog(16)
	defer srv.driver.UnsubscribeLog(ch)

	startResp := authReq(t, http.MethodPost, ts.URL+"/api/v1/start", pw, "application/json", `{"config":"w"}`)
	defer startResp.Body.Close()
	var got okEnv[map[string]any]
	if err := json.NewDecoder(startResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if startResp.StatusCode != http.StatusOK || got.Data["accepted"] != true || got.Data["runtimePrepare"] != true {
		t.Fatalf("受付応答が想定外: status=%d data=%+v", startResp.StatusCode, got.Data)
	}

	// goroutine の完了（sys ログ到着）を待つ。
	deadline := time.After(5 * time.Second)
	for {
		select {
		case l := <-ch:
			if l.Kind == "sys" && strings.Contains(l.Text, "進行中のため起動を見送りました") {
				if calls != 1 {
					t.Errorf("installRuntime 呼び出し回数 = %d, want 1", calls)
				}
				return
			}
		case <-deadline:
			t.Fatal("ガード goroutine の sys ログが届かない")
		}
	}
}

// TestHandleStart_UpdateBeforeStartAccepted は UpdateBeforeManualStart ON＋Steam 設定済みのとき、
// コールド起動が {accepted, updating:true} を返し、goroutine が更新（seam）を実行することを確認する。
func TestHandleStart_UpdateBeforeStartAccepted(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	rc := config.DefaultRestart() // UpdateBeforeManualStart=true
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "guardtest-secret",
		HeadlessConfigDir: filepath.Join(tmp, "configs"),
		Restart:           &rc,
		Steam:             &config.Steam{Username: "u", Password: "p", BranchCode: "b"},
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	srv := New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	applyGuardSeams(srv, guardSeams{readOK: false}) // .NET ガードは素通り・steamRunning=false
	updatedCh := make(chan struct{}, 1)
	srv.updateResonite = func(context.Context, steam.UpdateParams) error { updatedCh <- struct{}{}; return nil }

	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/headless-configs/w", testPassword, "application/json",
		`{"startWorlds":[{"sessionName":"W"}]}`)
	resp.Body.Close()

	startResp := authReq(t, http.MethodPost, ts.URL+"/api/v1/start", testPassword, "application/json", `{"config":"w"}`)
	defer startResp.Body.Close()
	var got okEnv[map[string]any]
	if err := json.NewDecoder(startResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if startResp.StatusCode != http.StatusOK || got.Data["accepted"] != true || got.Data["updating"] != true {
		t.Fatalf("更新付き起動の受付応答が想定外: status=%d data=%+v", startResp.StatusCode, got.Data)
	}

	select {
	case <-updatedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("コールド起動の goroutine が更新（seam）を呼ばない")
	}
}
