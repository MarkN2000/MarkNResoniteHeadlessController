package server

// crash-monitor（Phase 8・P8-4b）のユニットテスト。
// cfg/inProgress/lastUsed/start/now を注入し、now を手で進めてループ保護を決定的に検証する。

import (
	"context"
	"sync"
	"testing"
	"time"
)

func newTestCrashMon(cfg func() (bool, int, int), inProgress func() bool, start func(string) error, now func() time.Time) *crashMonitor {
	return &crashMonitor{
		cfg:        cfg,
		inProgress: inProgress,
		lastUsed:   func() string { return "night" },
		start:      start,
		now:        now,
		windowUnit: time.Minute,
		signals:    make(chan struct{}, 8),
		logf:       func(string, ...any) {},
	}
}

func TestCrashMonitor_RecoversAndLoopProtects(t *testing.T) {
	// now を手で進める seam。
	var mu sync.Mutex
	cur := time.Unix(1_700_000_000, 0)
	now := func() time.Time { mu.Lock(); defer mu.Unlock(); return cur }
	setNow := func(t time.Time) { mu.Lock(); cur = t; mu.Unlock() }

	started := make(chan string, 8)
	cm := newTestCrashMon(
		func() (bool, int, int) { return true, 3, 10 }, // enabled, max3, window10min
		func() bool { return false },
		func(name string) error { started <- name; return nil },
		now,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cm.run(ctx)

	base := time.Unix(1_700_000_000, 0)
	// crash1 @0 -> recover
	setNow(base)
	cm.onUnexpectedExit()
	expectStart(t, started, "night", "crash1 で復帰するはず")
	// crash2 @+1min -> recover
	setNow(base.Add(1 * time.Minute))
	cm.onUnexpectedExit()
	expectStart(t, started, "night", "crash2 で復帰するはず")
	// crash3 @+2min -> 窓内3回目 -> ループ保護で復帰しない
	setNow(base.Add(2 * time.Minute))
	cm.onUnexpectedExit()
	expectNoStart(t, started, "crash3 はループ保護で復帰しないはず")
	// crash4 @+15min -> 窓(10分)で古いものが落ち len=1 -> 再武装して復帰
	setNow(base.Add(15 * time.Minute))
	cm.onUnexpectedExit()
	expectStart(t, started, "night", "窓経過後の crash4 は再武装して復帰するはず")
}

func TestCrashMonitor_DisabledAndInProgress(t *testing.T) {
	started := make(chan string, 8)
	now := func() time.Time { return time.Unix(1_700_000_000, 0) }

	// 無効 -> 復帰しない
	off := newTestCrashMon(func() (bool, int, int) { return false, 3, 10 }, func() bool { return false }, func(string) error { started <- "x"; return nil }, now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go off.run(ctx)
	off.onUnexpectedExit()
	expectNoStart(t, started, "crashRecovery 無効では復帰しないはず")

	// 進行中 -> 復帰しない（orchestrator が所有）
	inprog := newTestCrashMon(func() (bool, int, int) { return true, 3, 10 }, func() bool { return true }, func(string) error { started <- "y"; return nil }, now)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go inprog.run(ctx2)
	inprog.onUnexpectedExit()
	expectNoStart(t, started, "再起動進行中は復帰しないはず")
}

func expectStart(t *testing.T, ch chan string, want, msg string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("%s: start config=%q want %q", msg, got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: start が呼ばれなかった", msg)
	}
}

func expectNoStart(t *testing.T, ch chan string, msg string) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("%s: 予期せず start(%q) が呼ばれた", msg, got)
	case <-time.After(200 * time.Millisecond):
	}
}
