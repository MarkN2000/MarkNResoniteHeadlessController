package headless

import "testing"

// TestScanFault_DuplicateInstance は起動ログからの「同一 data path 競合」検出を検証する。
// 文言は実機（2026-06-05 採取）の DuplicateInstanceException 由来。検出時 Status.Fault が立つ。
func TestScanFault_DuplicateInstance(t *testing.T) {
	if got := NewDriver(nil).Status().Fault; got != "" {
		t.Fatalf("初期 Fault は空のはず: %q", got)
	}
	// data path 競合メッセージ。
	d1 := NewDriver(nil)
	d1.scanFault(`Another instance of is already running with same data path (C:\...\Headless\Data), shutting down...`)
	if got := d1.Status().Fault; got != FaultDuplicateInstance {
		t.Errorf("data path 競合で Fault=%q を期待, got %q", FaultDuplicateInstance, got)
	}
	// 例外名でも検出。
	d2 := NewDriver(nil)
	d2.scanFault("FrooxEngine will shutdown because: ... DuplicateInstanceException")
	if got := d2.Status().Fault; got != FaultDuplicateInstance {
		t.Errorf("DuplicateInstanceException で Fault=%q を期待, got %q", FaultDuplicateInstance, got)
	}
	// 無関係な行では立たない。
	d3 := NewDriver(nil)
	d3.scanFault("World running")
	if got := d3.Status().Fault; got != "" {
		t.Errorf("無関係行で Fault は空のはず, got %q", got)
	}
}

// TestScanLogin_States は起動ログからの Resonite ログイン状態検出を検証する。
// 文言は実機 fixtures（scripts/empirical-capture/fixtures/2026-05-28-lan-login/）由来。
func TestScanLogin_States(t *testing.T) {
	// 初期＝anonymous。
	d := NewDriver(nil)
	if got := d.Status().LoginState; got != LoginAnonymous {
		t.Fatalf("初期は anonymous のはず: %s", got)
	}
	// 匿名フロー（試行なし・Initial Startup のみ）→ anonymous のまま。
	d.scanLogin("Initializing SignalR: Initial Startup")
	d.scanLogin("Connecting to SignalR (Initial Startup)...")
	if got := d.Status().LoginState; got != LoginAnonymous {
		t.Fatalf("試行なしは anonymous のはず: %s", got)
	}

	// 成功フロー（実機の順）。
	ok := NewDriver(nil)
	ok.scanLogin("Logging in as MarkN_headless")
	if got := ok.Status().LoginState; got != LoginFailed {
		t.Fatalf("試行直後・成功前は failed のはず: %s", got)
	}
	ok.scanLogin("Initializing SignalR: UserLogin: U-1NzqeqewOpM")
	ok.scanLogin("Logged in successfully")
	st := ok.Status()
	if st.LoginState != LoginLoggedIn {
		t.Fatalf("成功は loggedIn のはず: %s", st.LoginState)
	}
	if st.LoginUserID != "U-1NzqeqewOpM" {
		t.Fatalf("UserID は U- 付きで取得のはず: %q", st.LoginUserID)
	}
	// 再ログイン試行で成功フラグがリセットされる。
	ok.scanLogin("Logging in as other")
	if got := ok.Status(); got.LoginState != LoginFailed || got.LoginUserID != "" {
		t.Fatalf("再試行で failed＋UserID クリアのはず: %+v", got)
	}

	// 失敗フロー（試行あり・成功確認なし）→ failed。
	ng := NewDriver(nil)
	ng.scanLogin("Logging in as someone")
	ng.scanLogin("Initializing SignalR: Initial Startup")
	if got := ng.Status().LoginState; got != LoginFailed {
		t.Fatalf("試行ありで成功なしは failed のはず: %s", got)
	}
}

// TestMaybeReady_IgnoresEngineReady は "Engine Ready" が readiness トリガに
// ならない（warmup を起動しない・ready を立てない）ことを検証する。
// 実機で "Engine Ready" は REPL 稼働の約3.6秒前に出るため、これを信号にすると
// 起動直後の最初の1コマンドが無視/Unknown 化する。その回帰を防ぐ。
func TestMaybeReady_IgnoresEngineReady(t *testing.T) {
	d := NewDriver(nil)
	d.mu.Lock()
	d.state = StateStarting
	d.mu.Unlock()

	d.maybeReady("Engine Ready!")

	d.mu.Lock()
	started, ready := d.warmupStarted, d.ready
	d.mu.Unlock()
	if started {
		t.Fatal("Engine Ready は warmup を起動してはいけない")
	}
	if ready {
		t.Fatal("Engine Ready は ready を立ててはいけない")
	}
}
