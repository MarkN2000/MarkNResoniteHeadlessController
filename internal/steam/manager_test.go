package steam

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestManager は ensureDD を「偽 DepotDownloader（テストバイナリ自己再実行）」に差し替えた
// Manager を返す。GO_FAKE_DD 系の env は呼び出し側テストが t.Setenv で設定する。
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(t.TempDir())
	m.ensureDD = func(ctx context.Context, onEvent func(Event)) (string, error) {
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

// TestManager_VerifyHeadlessMissing は DL 成功(exit 0)でも VerifyRelPath が無ければ失敗にする（H2）。
func TestManager_VerifyHeadlessMissing(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "success")
	t.Setenv("GO_FAKE_DD_PASSWORD", "secret")

	m := newTestManager(t)
	p := testParams(t)
	p.VerifyRelPath = filepath.Join("Headless", "Resonite.exe") // 偽DDは作らない→不在

	err := m.Update(context.Background(), p)
	if err == nil {
		t.Fatal("headless 実体が無ければ Update は失敗すべき（ブランチコード誤り検出・H2）")
	}
	if !errors.Is(err, ErrVerifyMissing) {
		t.Errorf("ErrVerifyMissing を返すべき: %v", err)
	}
	if st := m.Status(); st.State != stateFailed || st.ErrorCode != "verify_missing" {
		t.Errorf("state=%q errorCode=%q want failed/verify_missing", st.State, st.ErrorCode)
	}
}

// TestManager_VerifyHeadlessPresent は VerifyRelPath が存在すれば成功する（H2 の偽陽性が無いこと）。
func TestManager_VerifyHeadlessPresent(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "success")
	t.Setenv("GO_FAKE_DD_PASSWORD", "secret")

	m := newTestManager(t)
	p := testParams(t)
	rel := filepath.Join("Headless", "Resonite.exe")
	p.VerifyRelPath = rel
	// 期待ファイルを先に作っておく（DD が落としたとみなす）。
	target := filepath.Join(p.InstallDir, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := m.Update(context.Background(), p); err != nil {
		t.Fatalf("実体ありなら成功すべき: %v", err)
	}
	if st := m.Status(); st.State != stateSuccess {
		t.Errorf("state=%q want success", st.State)
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

// TestManager_MilestoneDedup は同一 phase の連続マイルストーンが1回だけ publish される（M4）ことを確認する。
func TestManager_MilestoneDedup(t *testing.T) {
	m := newTestManager(t)
	ch, _ := m.Subscribe(16)
	for _, txt := range []string{"Pre-allocating", "Pre-allocating", "Pre-allocating", "Total downloaded", "Total downloaded"} {
		m.onEvent(Event{Kind: "milestone", Text: txt})
	}
	m.Unsubscribe(ch) // チャンネルを閉じる→range で drain

	var got []string
	for e := range ch {
		if e.Kind == "milestone" {
			got = append(got, e.Text)
		}
	}
	want := []string{"Pre-allocating", "Total downloaded"}
	if len(got) != len(want) {
		t.Fatalf("publish されたマイルストーン=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("milestone[%d]=%q want %q", i, got[i], want[i])
		}
	}
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
	if st := m.Status(); st.ErrorCode != "cancelled" {
		t.Errorf("errorCode=%q want cancelled", st.ErrorCode)
	}
}

// TestManager_CancelDuringAcquire は acquire（DD本体DL）段階の Cancel も「中止」に正規化される
// ことを確認する（acquire_failed への誤分類防止）。ensureDD は ctx が止まるまで block する。
func TestManager_CancelDuringAcquire(t *testing.T) {
	m := newTestManager(t)
	m.ensureDD = func(ctx context.Context, onEvent func(Event)) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	if err := m.Start(testParams(t)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, m, stateRunning, 5*time.Second)
	if err := m.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waitState(t, m, stateFailed, 10*time.Second)
	st := m.Status()
	if st.ErrorCode != "cancelled" {
		t.Errorf("acquire 段階の中止は errorCode=cancelled になるべき: %q（lastError=%q）", st.ErrorCode, st.LastError)
	}
	// 終端 result イベント（履歴）にも code が乗る（SSE 再接続経路の担保）。
	_, hist := m.Subscribe(1)
	var sawResult bool
	for _, e := range hist {
		if e.Kind == "result" {
			sawResult = true
			if e.Code != "cancelled" {
				t.Errorf("result イベントの code=%q want cancelled", e.Code)
			}
		}
	}
	if !sawResult {
		t.Error("履歴に result イベントが無い")
	}
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
	if st := m.Status(); st.LastError == "" || st.ErrorCode != "stalled" {
		t.Errorf("停滞中断時は lastError と errorCode=stalled が入るべき（lastError=%q errorCode=%q）", st.LastError, st.ErrorCode)
	}
}

// TestManager_BeginClearsLogs は run 開始時に前回 run の履歴を持ち越さない（SSE リプレイの混入防止）
// ことを確認する。
func TestManager_BeginClearsLogs(t *testing.T) {
	m := newTestManager(t)
	m.onEvent(Event{Kind: "log", Text: "old run line"})
	if _, hist := m.Subscribe(1); len(hist) != 1 {
		t.Fatalf("前提: 履歴1件のはず（%d件）", len(hist))
	}
	if _, err := m.begin(context.Background(), testParams(t)); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, hist := m.Subscribe(1); len(hist) != 0 {
		t.Errorf("begin 後の履歴は空のはず（%d件）", len(hist))
	}
	m.finish(nil)
}

// TestErrorCode は sentinel→コードの対応（ラップ込み）を検証する。
func TestErrorCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"cancelled", ErrCancelled, "cancelled"},
		{"two_factor", ErrTwoFactorRequired, "two_factor_required"},
		{"auth", ErrAuthFailed, "auth_failed"},
		{"stalled wrapped", fmt.Errorf("%w（5m0s 無進捗）", ErrStalled), "stalled"},
		{"verify wrapped", fmt.Errorf("%w（Headless/Resonite.exe）。ブランチコードが正しいか確認してください", ErrVerifyMissing), "verify_missing"},
		{"acquire wrapped", fmt.Errorf("%w: %w", ErrAcquireFailed, errors.New("HTTP 404")), "acquire_failed"},
		{"dd wrapped", fmt.Errorf("%w: %w", ErrDDFailed, errors.New("exit status 1")), "dd_failed"},
		{"dd start wrapped", fmt.Errorf("%w: %w", ErrDDStartFailed, errors.New("exec format error")), "dd_failed"},
		{"chmod wrapped", fmt.Errorf("%w: %w", ErrChmodFailed, errors.New("permission denied")), "chmod_failed"},
		{"unknown", errors.New("something else"), ""},
	}
	for _, c := range cases {
		if got := errorCode(c.err); got != c.want {
			t.Errorf("%s: errorCode=%q want %q", c.name, got, c.want)
		}
	}
}

// TestErrorDetail は「<sentinel>: <内側>」型からの診断詳細の抽出を検証する。
func TestErrorDetail(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"acquire", fmt.Errorf("%w: %w", ErrAcquireFailed, errors.New("ダウンロードに失敗: HTTP 404")), "ダウンロードに失敗: HTTP 404"},
		{"dd", fmt.Errorf("%w: %w", ErrDDFailed, errors.New("exit status 1")), "exit status 1"},
		{"自己完結型は空", ErrAuthFailed, ""},
		{"stalled は空", fmt.Errorf("%w（5m0s 無進捗）", ErrStalled), ""},
		{"nil", nil, ""},
	}
	for _, c := range cases {
		if got := errorDetail(c.err); got != c.want {
			t.Errorf("%s: errorDetail=%q want %q", c.name, got, c.want)
		}
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
