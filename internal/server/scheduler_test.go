package server

// scheduler 発火 goroutine（Phase 8・P8-4a）のユニットテスト。
// nextFire/trigger/now を注入し、実時間の短い遅延で「発火→trigger」「Reload で再計算」
// 「ctx 解除で終了」を決定的に確認する（実 config 非依存）。

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func newTestScheduler(nextFire func(time.Time) (time.Time, string, bool), trigger func(string, string) error) *restartScheduler {
	return &restartScheduler{
		nextFire: nextFire,
		trigger:  trigger,
		now:      time.Now,
		reloadCh: make(chan struct{}, 1),
		logf:     func(string, ...any) {},
	}
}

func TestScheduler_FiresAndTriggers(t *testing.T) {
	var calls int32
	nextFire := func(now time.Time) (time.Time, string, bool) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return now.Add(30 * time.Millisecond), "night", true // 初回のみ発火予定
		}
		return time.Time{}, "", false // 発火後は予定なし（再発火しない）
	}
	fired := make(chan [2]string, 4)
	sc := newTestScheduler(nextFire, func(tt, cn string) error {
		fired <- [2]string{tt, cn}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.run(ctx)

	select {
	case got := <-fired:
		if got[0] != "scheduled" || got[1] != "night" {
			t.Fatalf("trigger 引数が想定外: %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("予定時刻に trigger が呼ばれなかった")
	}
}

func TestScheduler_ReloadRecomputes(t *testing.T) {
	var armed int32      // 1 になったら予定が現れる
	var fireCalls int32  // 予定は1回だけ発火させる
	nextFire := func(now time.Time) (time.Time, string, bool) {
		if atomic.LoadInt32(&armed) == 1 && atomic.AddInt32(&fireCalls, 1) == 1 {
			return now.Add(20 * time.Millisecond), "day", true
		}
		return time.Time{}, "", false
	}
	fired := make(chan [2]string, 4)
	sc := newTestScheduler(nextFire, func(tt, cn string) error {
		fired <- [2]string{tt, cn}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.run(ctx)

	// 当初は予定なし → 発火しない。
	select {
	case got := <-fired:
		t.Fatalf("予定なしのはずが発火した: %v", got)
	case <-time.After(80 * time.Millisecond):
	}

	// 予定を出して Reload → 再計算で発火するはず。
	atomic.StoreInt32(&armed, 1)
	sc.Reload()
	select {
	case got := <-fired:
		if got[1] != "day" {
			t.Fatalf("Reload 後の trigger config が想定外: %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Reload 後に trigger が呼ばれなかった")
	}
}

func TestScheduler_CtxCancelStops(t *testing.T) {
	nextFire := func(time.Time) (time.Time, string, bool) { return time.Time{}, "", false } // 予定なし＝待機
	sc := newTestScheduler(nextFire, func(string, string) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sc.run(ctx); close(done) }()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ctx 解除で run が終了しなかった")
	}
}
