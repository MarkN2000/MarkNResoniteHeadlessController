package server

// crash-monitor（Phase 8・§3.16(4)・§5.6・P8-4b）。
// driver の意図しない終了（SetOnUnexpectedExit）を受け、設定 ON なら直近 config で自動復帰する。
// ループ保護: windowMinutes 窓内のクラッシュ回数が maxCrashes 以上になったら復帰を止めて log（無限再起動防止）。
// run は Server.Start が起動し ctx 解除で終了する。状態（crashes 窓）は run goroutine だけが触る。

import (
	"context"
	"log"
	"time"
)

type crashMonitor struct {
	// cfg は現在の crashRecovery 設定（有効・許容回数・窓分）を返す（restartCfg 由来）。
	cfg func() (enabled bool, maxCrashes, windowMin int)
	// inProgress は再起動進行中か（true なら orchestrator がライフサイクルを所有＝復帰しない）。
	inProgress func() bool
	lastUsed   func() string
	start      func(name string) error // 直近 config で起動（resolveLaunch + driver.Start）
	now        func() time.Time
	windowUnit time.Duration // windowMin の単位（本番 time.Minute・テストで縮める seam）
	signals    chan struct{}
	logf       func(format string, args ...any)

	crashes []time.Time // ループ保護の窓（run goroutine 専有）
}

func newCrashMonitor(s *Server) *crashMonitor {
	return &crashMonitor{
		cfg: func() (bool, int, int) {
			s.cfgMu.RLock()
			rc := s.cfg.RestartOrDefault()
			s.cfgMu.RUnlock()
			cr := rc.CrashRecovery
			return cr.Enabled, cr.MaxCrashes, cr.WindowMinutes
		},
		inProgress: func() bool { return s.restart.snapshot().inProgress },
		lastUsed:   s.loadLastUsed,
		start: func(name string) error {
			headlessPath, launchPath, err := s.resolveLaunch(name)
			if err != nil {
				return err
			}
			return s.driver.Start(headlessPath, launchPath, name)
		},
		now:        time.Now,
		windowUnit: time.Minute,
		signals:    make(chan struct{}, 4),
		logf:       log.Printf,
	}
}

// onUnexpectedExit は driver のクラッシュ検知コールバック（非ブロッキング）。
func (cm *crashMonitor) onUnexpectedExit() {
	select {
	case cm.signals <- struct{}{}:
	default:
	}
}

// run はクラッシュシグナルを受けて自動復帰する。ctx 解除で終了。
func (cm *crashMonitor) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.signals:
			cm.handleCrash()
		}
	}
}

func (cm *crashMonitor) handleCrash() {
	enabled, maxCrashes, windowMin := cm.cfg()
	if !enabled {
		return // クラッシュ復帰 OFF
	}
	if cm.inProgress() {
		cm.logf("[crash] 再起動進行中のため自動復帰をスキップ（orchestrator が処理）")
		return
	}
	// ループ保護: 窓内のクラッシュ回数を数え、maxCrashes 以上なら復帰停止。
	now := cm.now()
	window := time.Duration(windowMin) * cm.windowUnit
	kept := cm.crashes[:0]
	for _, t := range cm.crashes {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	cm.crashes = append(kept, now)
	if len(cm.crashes) >= maxCrashes {
		cm.logf("[crash] ループ保護作動: %d分内に%d回クラッシュ。自動復帰を停止します", windowMin, len(cm.crashes))
		return
	}
	name := cm.lastUsed()
	if name == "" {
		cm.logf("[crash] 直近 config が不明のため復帰できません")
		return
	}
	if err := cm.start(name); err != nil {
		cm.logf("[crash] 自動復帰の起動に失敗（%s）: %v", name, err)
		return
	}
	cm.logf("[crash] 自動復帰しました（config=%s）", name)
}
