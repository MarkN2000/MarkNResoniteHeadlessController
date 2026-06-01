package server

// restart-orchestrator（Phase 8・P8-3b）のユニットテスト。
// driver/worlds をフェイク化し、タイミング seam（minute/waitInterval/spawnDelay）を ms に縮めて
// フロー（0人即時/①②③/cancel/排他）を高速・決定的に検証する。実ヘッドレスは不要。

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
)

// --- フェイク ---

type fakeDriver struct {
	mu      sync.Mutex
	state   headless.State
	stops   int
	starts  int
	label   string
}

func (d *fakeDriver) Status() headless.Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	return headless.Status{State: d.state}
}
func (d *fakeDriver) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stops++
	d.state = headless.StateStopped
	return nil
}
func (d *fakeDriver) Start(_, _, label string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.starts++
	d.label = label
	d.state = headless.StateRunning
	return nil
}
func (d *fakeDriver) snap() (state headless.State, stops, starts int, label string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state, d.stops, d.starts, d.label
}
func (d *fakeDriver) setState(s headless.State) {
	d.mu.Lock()
	d.state = s
	d.mu.Unlock()
}

type fakeWorlds struct {
	mu    sync.Mutex
	users int
	cmds  []string
}

func (fw *fakeWorlds) List(context.Context) ([]headless.World, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return []headless.World{{Index: 0, Name: "W", Users: fw.users}}, nil
}
func (fw *fakeWorlds) ForEach(_ context.Context, fn func(headless.World, headless.Scope) error) error {
	w := headless.World{Index: 0, Name: "W"}
	return fn(w, &fakeScope{fw: fw, w: w})
}
func (fw *fakeWorlds) setUsers(n int) {
	fw.mu.Lock()
	fw.users = n
	fw.mu.Unlock()
}
func (fw *fakeWorlds) commands() []string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return append([]string(nil), fw.cmds...)
}

type fakeScope struct {
	fw *fakeWorlds
	w  headless.World
}

func (s *fakeScope) Exec(cmd string, _ ...headless.ExecOption) ([]string, error) {
	s.fw.mu.Lock()
	s.fw.cmds = append(s.fw.cmds, cmd)
	s.fw.mu.Unlock()
	return nil, nil
}
func (s *fakeScope) World() headless.World { return s.w }

// --- ヘルパ ---

func newTestOrch(d restartDriver, fw headless.WorldsService, rc config.Restart, lastUsed string) *restartOrchestrator {
	return &restartOrchestrator{
		driver:           d,
		worlds:           fw,
		resolve:          func(name string) (string, string, error) { return "hp", "lp", nil },
		restartCfg:       func() config.Restart { return rc },
		lastUsed:         func() string { return lastUsed },
		recordUsed:       func(string) {},
		minute:           time.Millisecond, // 分→1ms に縮める
		waitInterval:     time.Millisecond,
		spawnDelay:       time.Millisecond,
		stopWaitTimeout:  2 * time.Second,
		stopPollInterval: time.Millisecond,
	}
}

func waitUntil(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("条件が時間内に満たされなかった: %s", msg)
}

func hasCmd(cmds []string, substr string) bool {
	for _, c := range cmds {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// --- decideWait（純関数）---

func TestDecideWait(t *testing.T) {
	min := time.Minute
	cases := []struct {
		name           string
		total          int
		totalErr       bool
		remaining      time.Duration
		actionTiming   time.Duration
		announced      bool
		announceOn     bool
		want           waitAction
	}{
		{"0人→即再起動", 0, false, 10 * min, 2 * min, false, true, waitRestart},
		{"取得失敗は0人扱いしない", 0, true, 10 * min, 2 * min, false, true, waitContinue},
		{"締切到達→再起動", 3, false, 0, 2 * min, true, true, waitRestart},
		{"締切前・告知圏内→告知", 3, false, 2 * min, 2 * min, false, true, waitAnnounce},
		{"告知済みは再告知しない", 3, false, 1 * min, 2 * min, true, true, waitContinue},
		{"告知無効なら告知しない", 3, false, 1 * min, 2 * min, false, false, waitContinue},
		{"まだ余裕→継続", 3, false, 9 * min, 2 * min, false, true, waitContinue},
	}
	for _, c := range cases {
		got := decideWait(c.total, c.totalErr, c.remaining, c.actionTiming, c.announced, c.announceOn)
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// --- フロー ---

func TestTrigger_NotRunning(t *testing.T) {
	d := &fakeDriver{state: headless.StateStopped}
	o := newTestOrch(d, &fakeWorlds{}, config.DefaultRestart(), "night")
	if err := o.Trigger("manual", ""); err != errRestartNotRunning {
		t.Fatalf("停止中の trigger は errRestartNotRunning のはず: %v", err)
	}
}

func TestTrigger_ZeroUsersImmediate(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{users: 0}
	o := newTestOrch(d, fw, config.DefaultRestart(), "night")

	if err := o.Trigger("manual", ""); err != nil { // 空→lastUsed("night")
		t.Fatalf("trigger 失敗: %v", err)
	}
	waitUntil(t, func() bool { _, _, starts, _ := d.snap(); return starts == 1 }, 2*time.Second, "起動完了")
	waitUntil(t, func() bool { return !o.snapshot().inProgress }, 2*time.Second, "進行終了")

	_, stops, starts, label := d.snap()
	if stops != 1 || starts != 1 || label != "night" {
		t.Fatalf("0人即時再起動が想定外: stops=%d starts=%d label=%q", stops, starts, label)
	}
	if len(fw.commands()) != 0 {
		t.Fatalf("0人時は事前アクションを行わないはず: %v", fw.commands())
	}
}

func TestTrigger_FullFlow_SessionThenAnnounceThenRestart(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{users: 2} // 常に居る→締切で強制
	rc := config.DefaultRestart()
	rc.WaitControl = config.WaitControl{ForceRestartTimeoutMin: 100, ActionTimingMin: 50} // 100ms/50ms（minute=1ms）
	rc.PreActions.SessionChanges = config.SessionChanges{SetPrivate: true, SetMaxUsersOne: true}
	rc.PreActions.Announce = config.AnnounceAction{Enabled: true, ItemURL: "resrec:///x", ImpulseTag: "MRHC.play", Message: "再起動します"}
	o := newTestOrch(d, fw, rc, "night")

	if err := o.Trigger("manual", "day"); err != nil {
		t.Fatalf("trigger 失敗: %v", err)
	}
	waitUntil(t, func() bool { _, _, starts, _ := d.snap(); return starts == 1 }, 5*time.Second, "強制再起動完了")
	waitUntil(t, func() bool { return !o.snapshot().inProgress }, 2*time.Second, "進行終了")

	cmds := fw.commands()
	if !hasCmd(cmds, "accesslevel Private") || !hasCmd(cmds, "maxusers 1") {
		t.Fatalf("① セッション変更が出ていない: %v", cmds)
	}
	if !hasCmd(cmds, "spawn resrec:///x true") {
		t.Fatalf("③ spawn が出ていない: %v", cmds)
	}
	if !hasCmd(cmds, `dynamicimpulsestring MRHC.play "再起動します"`) {
		t.Fatalf("③ dynamicimpulse が出ていない: %v", cmds)
	}
	if _, _, starts, label := d.snap(); starts != 1 || label != "day" {
		t.Fatalf("再起動の config が想定外: starts=%d label=%q（指定 day のはず）", starts, label)
	}
}

func TestTrigger_CancelDuringWaiting(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{users: 2} // 居続ける→待機にとどまる
	rc := config.DefaultRestart()
	rc.WaitControl = config.WaitControl{ForceRestartTimeoutMin: 100000, ActionTimingMin: 0} // 実質止まらない
	rc.PreActions.Announce.Enabled = false
	o := newTestOrch(d, fw, rc, "night")

	if err := o.Trigger("manual", ""); err != nil {
		t.Fatalf("trigger 失敗: %v", err)
	}
	waitUntil(t, func() bool { return o.snapshot().phase == phaseWaiting }, 2*time.Second, "待機到達")

	if err := o.Cancel(); err != nil {
		t.Fatalf("cancel 失敗: %v", err)
	}
	waitUntil(t, func() bool { return !o.snapshot().inProgress }, 2*time.Second, "中止後の進行終了")

	if _, stops, starts, _ := d.snap(); stops != 0 || starts != 0 {
		t.Fatalf("待機中の中止では再起動しないはず: stops=%d starts=%d", stops, starts)
	}
}

func TestTrigger_DoubleRejected(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{users: 2}
	rc := config.DefaultRestart()
	rc.WaitControl = config.WaitControl{ForceRestartTimeoutMin: 100000, ActionTimingMin: 0}
	rc.PreActions.Announce.Enabled = false
	o := newTestOrch(d, fw, rc, "night")

	if err := o.Trigger("manual", ""); err != nil {
		t.Fatalf("1回目の trigger 失敗: %v", err)
	}
	if err := o.Trigger("manual", ""); err != errRestartInProgress {
		t.Fatalf("2回目は errRestartInProgress のはず: %v", err)
	}
	_ = o.Cancel()
	waitUntil(t, func() bool { return !o.snapshot().inProgress }, 2*time.Second, "片付け")
}

// フロー中（②待機中）にヘッドレスが落ちたら、締切を待たず即 ④ で復帰する（レビュー #2）。
func TestTrigger_HeadlessStopsDuringWait(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{users: 2} // 居続ける→本来は締切まで待機
	rc := config.DefaultRestart()
	rc.WaitControl = config.WaitControl{ForceRestartTimeoutMin: 100000, ActionTimingMin: 0} // 実質止まらない締切
	rc.PreActions.Announce.Enabled = false
	o := newTestOrch(d, fw, rc, "night")

	if err := o.Trigger("manual", ""); err != nil {
		t.Fatalf("trigger 失敗: %v", err)
	}
	waitUntil(t, func() bool { return o.snapshot().phase == phaseWaiting }, 2*time.Second, "待機到達")

	// フロー中にヘッドレスがクラッシュ（停止）したと模擬。
	d.setState(headless.StateStopped)

	// 締切（実質無限）を待たず即 ④ で復帰するはず。
	waitUntil(t, func() bool { _, _, starts, _ := d.snap(); return starts == 1 }, 2*time.Second, "即時復帰")
	waitUntil(t, func() bool { return !o.snapshot().inProgress }, 2*time.Second, "進行終了")
	if _, _, starts, label := d.snap(); starts != 1 || label != "night" {
		t.Fatalf("フロー中クラッシュ後の復帰が想定外: starts=%d label=%q", starts, label)
	}
}

func TestCancel_WhenIdle(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	o := newTestOrch(d, &fakeWorlds{}, config.DefaultRestart(), "night")
	if err := o.Cancel(); err != errNoRestartInProgress {
		t.Fatalf("idle の cancel は errNoRestartInProgress のはず: %v", err)
	}
}
