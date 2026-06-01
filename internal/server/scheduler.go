package server

// scheduler 発火 goroutine（Phase 8・§3.16(8)・P8-4a）。
// 有効予定の次回発火時刻（P8-2 の NextScheduled）まで待機し、到達したら orchestrator に
// scheduled トリガーを送る。config 変更（restart-config PUT）は Reload シグナルで即再計算する
// （ポーリングしない）。cron 不使用。run は Server.Start が起動し ctx 解除で終了する。

import (
	"context"
	"log"
	"time"
)

type restartScheduler struct {
	// nextFire は now 時点の次回発火（時刻・config名・有無）を返す。本番は restart 設定の NextScheduled。
	nextFire func(now time.Time) (at time.Time, configName string, ok bool)
	// trigger は発火時に呼ぶ（本番は orchestrator.Trigger）。非稼働/進行中は err を返すが scheduler は log して継続。
	trigger  func(triggerType, configName string) error
	now      func() time.Time
	reloadCh chan struct{}
	logf     func(format string, args ...any)
}

func newRestartScheduler(s *Server) *restartScheduler {
	return &restartScheduler{
		nextFire: func(now time.Time) (time.Time, string, bool) {
			s.cfgMu.RLock()
			rc := s.cfg.RestartOrDefault()
			s.cfgMu.RUnlock()
			at, sched, ok := rc.NextScheduled(now)
			return at, sched.ConfigName, ok
		},
		trigger:  s.restart.Trigger,
		now:      time.Now,
		reloadCh: make(chan struct{}, 1),
		logf:     log.Printf,
	}
}

// Reload は次回発火の再計算を促す（restart-config PUT 後に呼ぶ）。
// バッファ1の非ブロッキング送信＝多重呼び出しは合体する。
func (sc *restartScheduler) Reload() {
	select {
	case sc.reloadCh <- struct{}{}:
	default:
	}
}

// run は次回発火まで待機し、発火で trigger を呼ぶループ。ctx 解除で終了。
func (sc *restartScheduler) run(ctx context.Context) {
	for {
		at, configName, ok := sc.nextFire(sc.now())
		var timerC <-chan time.Time
		var timer *time.Timer
		if ok {
			d := at.Sub(sc.now())
			if d < 0 {
				d = 0
			}
			timer = time.NewTimer(d)
			timerC = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-sc.reloadCh:
			if timer != nil {
				timer.Stop()
			}
			// config 変更 → 次ループで再計算。
		case <-timerC:
			// 発火。strictly-future（P8-2）なので、次ループの再計算で必ず未来へ進む（同分の二重発火なし・once は past で消える）。
			if err := sc.trigger("scheduled", configName); err != nil {
				sc.logf("[scheduler] 予定再起動を見送り（%s）: %v", configName, err)
			}
		}
	}
}
