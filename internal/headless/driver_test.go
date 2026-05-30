package headless

import "testing"

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
