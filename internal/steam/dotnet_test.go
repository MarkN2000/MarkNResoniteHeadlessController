package steam

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/dotnetruntime"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

// fakeEnsurerOpts は runtimeEnsurer の偽 seam を組み立てるための設定。
type fakeEnsurerOpts struct {
	readOK       bool
	localResults []bool // local() が呼ばれるたびに先頭から返す（尽きたら最後の値）
	system       bool
	acquireErr   error
	acquireCalls *int    // acquire の呼び出し回数を記録（nil 可）
	gotChannel   *string // acquire に渡った channel を記録（nil 可）
}

func fakeEnsurer(o fakeEnsurerOpts) *runtimeEnsurer {
	req := platform.RuntimeRequirement{Major: 10, Minor: 0, Patch: 0, Raw: "10.0.0"}
	localIdx := 0
	return &runtimeEnsurer{
		goos: "linux", goarch: "amd64",
		read: func(string) (platform.RuntimeRequirement, bool) { return req, o.readOK },
		local: func(string, platform.RuntimeRequirement, string) bool {
			r := false
			if len(o.localResults) > 0 {
				if localIdx >= len(o.localResults) {
					localIdx = len(o.localResults) - 1
				}
				r = o.localResults[localIdx]
				localIdx++
			}
			return r
		},
		system: func(string, string, platform.RuntimeRequirement) bool { return o.system },
		acquire: func(ctx context.Context, installDir, channel string, onEvent func(dotnetruntime.Event)) (string, error) {
			if o.acquireCalls != nil {
				*o.acquireCalls++
			}
			if o.gotChannel != nil {
				*o.gotChannel = channel
			}
			if o.acquireErr != nil {
				return "", o.acquireErr
			}
			return "10.0.8", nil
		},
	}
}

func TestRuntimeEnsurer_SkipNoRequirement(t *testing.T) {
	calls := 0
	e := fakeEnsurer(fakeEnsurerOpts{readOK: false, acquireCalls: &calls})
	if err := e.ensure(context.Background(), t.TempDir(), func(Event) {}); err != nil {
		t.Fatalf("要求が読めない場合は楽観スキップすべき: %v", err)
	}
	if calls != 0 {
		t.Error("要求が無いのに acquire が呼ばれた")
	}
}

func TestRuntimeEnsurer_SkipLocalSatisfied(t *testing.T) {
	calls := 0
	e := fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{true}, acquireCalls: &calls})
	if err := e.ensure(context.Background(), t.TempDir(), func(Event) {}); err != nil {
		t.Fatalf("ローカル充足はスキップすべき: %v", err)
	}
	if calls != 0 {
		t.Error("ローカル充足なのに acquire が呼ばれた")
	}
}

func TestRuntimeEnsurer_SkipSystemSatisfied(t *testing.T) {
	calls := 0
	e := fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{false}, system: true, acquireCalls: &calls})
	if err := e.ensure(context.Background(), t.TempDir(), func(Event) {}); err != nil {
		t.Fatalf("システム充足はスキップすべき: %v", err)
	}
	if calls != 0 {
		t.Error("システム充足なのに acquire が呼ばれた")
	}
}

func TestRuntimeEnsurer_InstallSuccess(t *testing.T) {
	calls := 0
	channel := ""
	e := fakeEnsurer(fakeEnsurerOpts{
		readOK: true, localResults: []bool{false, true}, // 設置前=不足 / 設置後=充足
		acquireCalls: &calls, gotChannel: &channel,
	})
	if err := e.ensure(context.Background(), t.TempDir(), func(Event) {}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if calls != 1 {
		t.Errorf("acquire 呼び出し回数 = %d, want 1", calls)
	}
	if channel != "10.0" {
		t.Errorf("channel = %q, want 10.0（runtimeconfig 由来）", channel)
	}
}

func TestRuntimeEnsurer_InstallFailure(t *testing.T) {
	e := fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{false}, acquireErr: errors.New("HTTP 503")})
	err := e.ensure(context.Background(), t.TempDir(), func(Event) {})
	if !errors.Is(err, ErrDotnetInstallFailed) {
		t.Fatalf("ErrDotnetInstallFailed を返すべき: %v", err)
	}
	if errorCode(err) != "dotnet_install_failed" {
		t.Errorf("errorCode = %q", errorCode(err))
	}
	if !strings.Contains(errorDetail(err), "HTTP 503") {
		t.Errorf("errorDetail に内側原文が無い: %q", errorDetail(err))
	}
}

func TestRuntimeEnsurer_StillUnsatisfiedAfterInstall(t *testing.T) {
	// 設置は成功するが要求をなお満たさない（要求 patch > フィード最新等）→ 明示エラー
	e := fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{false, false}})
	err := e.ensure(context.Background(), t.TempDir(), func(Event) {})
	if !errors.Is(err, ErrDotnetInstallFailed) {
		t.Fatalf("設置後も不充足なら明示エラーにすべき（サイレント再DLループ防止）: %v", err)
	}
}

func TestRuntimeEnsurer_CancelNormalized(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{false}, acquireErr: context.Canceled})
	if err := e.ensure(ctx, t.TempDir(), func(Event) {}); !errors.Is(err, ErrCancelled) {
		t.Fatalf("中断は ErrCancelled へ正規化すべき: %v", err)
	}
}

func TestManager_InstallRuntimeSuccess(t *testing.T) {
	m := newTestManager(t)
	m.dotnet = fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{false, true}})
	ch, _ := m.Subscribe(16)

	if err := m.InstallRuntime(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("InstallRuntime: %v", err)
	}
	st := m.Status()
	if st.State != stateSuccess || st.RunKind != runKindRuntime {
		t.Errorf("state=%q runKind=%q want success/runtime", st.State, st.RunKind)
	}
	m.Unsubscribe(ch)
	foundResult := false
	for e := range ch {
		if e.Kind == "result" {
			foundResult = true
			if e.RunKind != runKindRuntime {
				t.Errorf("result.RunKind = %q want runtime", e.RunKind)
			}
		}
	}
	if !foundResult {
		t.Error("result イベントが配信されていない")
	}
}

func TestManager_InstallRuntimeFailure(t *testing.T) {
	m := newTestManager(t)
	m.dotnet = fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{false}, acquireErr: errors.New("boom")})

	err := m.InstallRuntime(context.Background(), t.TempDir())
	if !errors.Is(err, ErrDotnetInstallFailed) {
		t.Fatalf("ErrDotnetInstallFailed を返すべき: %v", err)
	}
	st := m.Status()
	if st.State != stateFailed || st.ErrorCode != "dotnet_install_failed" || st.RunKind != runKindRuntime {
		t.Errorf("state=%q errorCode=%q runKind=%q", st.State, st.ErrorCode, st.RunKind)
	}
}

func TestManager_InstallRuntimeExclusiveWithUpdate(t *testing.T) {
	m := newTestManager(t)
	p := testParams(t)
	if _, err := m.begin(context.Background(), p); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := m.InstallRuntime(context.Background(), t.TempDir()); !errors.Is(err, ErrUpdateInProgress) {
		t.Fatalf("更新中の InstallRuntime は ErrUpdateInProgress を返すべき: %v", err)
	}
	m.finish(nil)
	if st := m.Status(); st.RunKind != runKindUpdate {
		t.Errorf("begin（更新）の RunKind = %q want update", st.RunKind)
	}
}

// TestManager_UpdateRunsEnsureStep は run() の最終段で設置ステップが呼ばれることを確認する。
func TestManager_UpdateRunsEnsureStep(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "success")
	t.Setenv("GO_FAKE_DD_PASSWORD", "secret")

	m := newTestManager(t)
	calls := 0
	m.dotnet = fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{false, true}, acquireCalls: &calls})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.Update(ctx, testParams(t)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if calls != 1 {
		t.Errorf("設置ステップの acquire 呼び出し回数 = %d, want 1", calls)
	}
	if st := m.Status(); st.State != stateSuccess || st.RunKind != runKindUpdate {
		t.Errorf("state=%q runKind=%q want success/update", st.State, st.RunKind)
	}
}

// TestManager_UpdateEnsureStepFailure は設置失敗が更新失敗（dotnet_install_failed）になることを確認する。
func TestManager_UpdateEnsureStepFailure(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "success")
	t.Setenv("GO_FAKE_DD_PASSWORD", "secret")

	m := newTestManager(t)
	m.dotnet = fakeEnsurer(fakeEnsurerOpts{readOK: true, localResults: []bool{false}, acquireErr: errors.New("HTTP 404")})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := m.Update(ctx, testParams(t))
	if !errors.Is(err, ErrDotnetInstallFailed) {
		t.Fatalf("ErrDotnetInstallFailed を返すべき: %v", err)
	}
	if st := m.Status(); st.ErrorCode != "dotnet_install_failed" || st.ErrorDetail != "HTTP 404" {
		t.Errorf("errorCode=%q errorDetail=%q", st.ErrorCode, st.ErrorDetail)
	}
}
