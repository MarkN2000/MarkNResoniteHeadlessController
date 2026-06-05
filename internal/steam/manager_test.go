package steam

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// newTestManager は ensureDD を「偽 DepotDownloader（テストバイナリ自己再実行）」に差し替えた
// Manager を返す。GO_FAKE_DD 系の env は呼び出し側テストが t.Setenv で設定する。
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(t.TempDir())
	m.ensureDD = func(ctx context.Context, logf func(string)) (string, error) {
		return os.Args[0], nil
	}
	return m
}

func testParams(t *testing.T) UpdateParams {
	return UpdateParams{
		InstallDir: t.TempDir(),
		Username:   "user",
		Password:   "secret",
		BranchCode: "betacode",
	}
}

func TestManager_UpdateSuccess(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "success")
	t.Setenv("GO_FAKE_DD_PASSWORD", "secret")

	m := newTestManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Update(ctx, testParams(t)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	st := m.Status()
	if st.State != stateSuccess {
		t.Errorf("state=%q want success（lastError=%q）", st.State, st.LastError)
	}
	if st.Percent != 100 {
		t.Errorf("percent=%v want 100", st.Percent)
	}
}

func TestManager_NotConfigured(t *testing.T) {
	m := newTestManager(t)
	err := m.Update(context.Background(), UpdateParams{InstallDir: "x"}) // 資格欠落
	if !errors.Is(err, ErrSteamNotConfigured) {
		t.Fatalf("未設定は ErrSteamNotConfigured を返すべき: %v", err)
	}
}

func TestManager_SingleFlight(t *testing.T) {
	m := newTestManager(t)
	p := testParams(t)
	if _, err := m.begin(context.Background(), p); err != nil {
		t.Fatalf("1回目 begin: %v", err)
	}
	if _, err := m.begin(context.Background(), p); !errors.Is(err, ErrUpdateInProgress) {
		t.Fatalf("2回目 begin は ErrUpdateInProgress を返すべき: %v", err)
	}
	m.finish(nil)
	if _, err := m.begin(context.Background(), p); err != nil {
		t.Fatalf("finish 後の begin は通るべき: %v", err)
	}
	m.finish(nil)
}

func TestManager_CancelInIdle(t *testing.T) {
	m := newTestManager(t)
	if err := m.Cancel(); !errors.Is(err, ErrNoUpdateInProgress) {
		t.Fatalf("idle の Cancel は ErrNoUpdateInProgress: %v", err)
	}
}

func TestManager_CancelInProgress(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "hang")

	m := newTestManager(t)
	m.stallTimeout = time.Hour // stall は発火させない（cancel を検証）

	if err := m.Start(testParams(t)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, m, stateRunning, 5*time.Second)
	if err := m.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waitState(t, m, stateFailed, 10*time.Second)
}

func TestManager_StallTimeout(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "hang")

	m := newTestManager(t)
	m.stallTimeout = 300 * time.Millisecond // すぐ停滞判定

	if err := m.Start(testParams(t)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, m, stateFailed, 10*time.Second)
	if st := m.Status(); st.LastError == "" {
		t.Error("停滞中断時は lastError が入るべき")
	}
}

// waitState は m の状態が want になるまで最大 timeout 待つ。
func waitState(t *testing.T, m *Manager, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.Status().State == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("状態が %q にならなかった（現在 %q）", want, m.Status().State)
}
