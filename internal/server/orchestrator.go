package server

// restart-orchestrator（Phase 8・§3.16(1)(8)・P8-3b）。
// 手動「通常再起動」/ scheduled（P8-4 で結線）の統一安全再起動フローを実行する。
// 並行モデル = 案A: 小さな mutex で進行状態を保護し、flow は goroutine、cancel は context
// （既存 cfgMu/driver.mu と同じ流儀）。実コマンドの直列化は driver の execMu が担う。
//
// フロー（§3.16(1)・待機は2区間モデル R9）:
//   0人 → 即再起動 / 居たら ①即セッション変更 → ②静かに待機（合計人数監視）
//   → quiet 経過（＝締切 announce 前）で ③dynamicImpulse 告知1回 → ④強制停止→選択 config で起動
// cancel は ①②③のみ可（④以降は不可）。セッション変更は自動復元しない（§3.16(1)）。
//
// 手動「通常再起動」と通常停止（R7・TriggerStop）は config の待機制御を使わず固定で即応する
// ＝告知前0分＋告知後1分（immediateManualWait）。config.WaitControl は予定再起動のみが使う。
// 通常停止は同じ前段を共有し、終端だけ「停止のみ（起動しない）」に分岐する。

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
)

// restartDriver は orchestrator が使う driver の最小面（テストで差し替え可能にする）。
// 実体の *headless.Driver がこのインタフェースを満たす。
type restartDriver interface {
	Status() headless.Status
	Stop() error
	Start(headlessPath, configPath, configLabel string) error
}

// 進行フェーズ（restart-status の phase に出す）。
const (
	phaseIdle       = "idle"
	phasePreparing  = "preparing"  // ① セッション変更中
	phaseWaiting    = "waiting"    // ② 退出待ち
	phaseAnnouncing = "announcing" // ③ 告知中
	phaseUpdating   = "updating"   // ④内 予定再起動時の Resonite 更新中（P9-B・停止後→起動前）
	phaseRestarting = "restarting" // ④ 終端＝停止→起動 or 停止のみ（R7・cancel 不可）
)

var (
	errRestartInProgress     = errors.New("再起動が既に進行中です")
	errRestartNotRunning     = errors.New("ヘッドレスが稼働していません")
	errRestartNoConfig       = errors.New("再起動するコンフィグが特定できません")
	errNoRestartInProgress   = errors.New("進行中の再起動がありません")
	errRestartNotCancellable = errors.New("再起動の最終段階のため中止できません")
)

// restartProgress は進行中の再起動の状態スナップショット（mu で保護）。
type restartProgress struct {
	inProgress  bool
	phase       string
	triggerType string // "manual" | "scheduled" | "stop"（R7 通常停止）
	configName  string // 解決済みの対象 config（空ではない）
	startedAt   time.Time
	deadlineAt  time.Time // ② の締切（waiting/announcing 中のみ有効）
	cancelled   bool
}

type restartOrchestrator struct {
	driver      restartDriver
	worlds      headless.WorldsService
	resolve     func(name string) (headlessPath, launchPath string, err error)
	restartCfg  func() config.Restart
	lastUsed    func() string
	recordUsed  func(name string)
	recordStart func(trigger, at string)                      // 最終起動の記録（§3.16(9)）
	beforeStart func(ctx context.Context, triggerType string) // ④ 停止後・起動前のフック（予定再起動時の更新・P9-B・nil可）
	// resolveAnnounce は告知の templateId → URL/タグ解決（server 注入・item_templates.go）。
	// nil（テスト等）かつ templateId 指定の告知は安全側＝スキップする。
	resolveAnnounce func(ctx context.Context, a config.AnnounceAction) (config.AnnounceAction, bool)

	// タイミング（本番は定数・テストで小さく差し替え可能にする seam）。
	minute           time.Duration // quiet/announce 待機の「分」単位（本番 time.Minute）
	waitInterval     time.Duration // ② の人数ポーリング間隔
	spawnDelay       time.Duration // ③ spawn完了確認→impulse の初期化猶予（本番500ms）
	stopWaitTimeout  time.Duration // ④ stop 後 StateStopped を待つ最大
	stopPollInterval time.Duration // ④ stop 待ちのポーリング間隔

	mu        sync.Mutex
	p         restartProgress
	cancel    context.CancelFunc
	parentCtx context.Context // 進行中フローの親 ctx（Start で bg ctx に差し替え＝shutdown で ①②③ を cancel）
}

func newRestartOrchestrator(s *Server) *restartOrchestrator {
	return &restartOrchestrator{
		driver:    s.driver,
		worlds:    s.worlds,
		resolve:   s.resolveLaunch,
		parentCtx: context.Background(),
		restartCfg: func() config.Restart {
			s.cfgMu.RLock()
			defer s.cfgMu.RUnlock()
			return s.cfg.RestartOrDefault()
		},
		lastUsed:         s.loadLastUsed,
		recordUsed:       s.recordLastUsed,
		recordStart:      s.recordLastStart,
		beforeStart:      s.beforeFlowStart, // 停止後・起動前の更新フック（予定＋手動「通常再起動」・P9-B）
		resolveAnnounce:  s.resolveAnnounce, // 告知テンプレの実行時解決
		minute:           time.Minute,
		waitInterval:     10 * time.Second,
		spawnDelay:       spawnReadyDelay,
		stopWaitTimeout:  190 * time.Second,
		stopPollInterval: 500 * time.Millisecond,
	}
}

// immediateManualWait は手動操作（手動「通常再起動」・通常停止 R7）の固定待機制御。
// 静かに待つ0分＝即告知、告知後1分で締切（強制実行）。予定再起動の config.WaitControl とは分離する。
func immediateManualWait() config.WaitControl {
	return config.WaitControl{QuietWaitMin: 0, AnnounceWaitMin: 1}
}

// Trigger は統一フローを非同期で開始する。即時に受付可否だけ返す。
// triggerType は "manual"（P8-3b）または "scheduled"（P8-4）。configName 空＝前回 config。
// 手動は即応（即告知＋最長1分）に上書きし、予定は config の待機制御に従う（§3.16(1)）。
func (o *restartOrchestrator) Trigger(triggerType, configName string) error {
	if o.driver.Status().State != headless.StateRunning {
		return errRestartNotRunning // 稼働していないものは「再起動」できない（§3.16(7) 手動は稼働中のみ）
	}
	name := configName
	if name == "" {
		name = o.lastUsed() // 空＝前回起動と同じ config（runtime-state）
	}
	if name == "" {
		return errRestartNoConfig
	}
	rc := o.restartCfg()
	if triggerType == "manual" {
		rc.WaitControl = immediateManualWait() // 手動は即応（rc はコピーなので保存設定は不変）。予定は config のまま
	}
	return o.startFlow(rc, name, triggerType, false)
}

// TriggerStop は「通常停止」を非同期で開始する（R7）。再起動フローの前段（0人判定／①セッション変更／
// ②待機／③告知）を共有し、終端だけ「停止のみ（起動しない）」に分岐する。停止は素早く行いたいので
// 再起動の待機制御（最大長時間）は使わず、即応＝告知前0分＋告知後1分の固定猶予とする（告知は即時）。
// 稼働中のみ・在席0人なら即停止・①③猶予中は Cancel 可。
func (o *restartOrchestrator) TriggerStop() error {
	if o.driver.Status().State != headless.StateRunning {
		return errRestartNotRunning // 稼働していないものは停止フローに載せない
	}
	rc := o.restartCfg()
	rc.WaitControl = immediateManualWait() // 即応＝固定1分（rc はコピーなので保存設定は不変）
	return o.startFlow(rc, "", "stop", true)
}

// startFlow は進行フラグの排他・ctx 生成・進行状態のセット・フロー goroutine 起動を行う共通処理。
// Trigger（再起動）と TriggerStop（停止）が受付処理を共有する＝呼び出し側で稼働確認・rc 準備・
// configName 解決を済ませてから渡す。停止は name="" を渡す（起動しないので config 不要）。
func (o *restartOrchestrator) startFlow(rc config.Restart, name, triggerType string, stopOnly bool) error {
	o.mu.Lock()
	if o.p.inProgress {
		o.mu.Unlock()
		return errRestartInProgress // 二重起動防止（再起動と停止で共通の進行フラグ・§3.16(1)）
	}
	parent := o.parentCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	o.cancel = cancel
	o.p = restartProgress{
		inProgress:  true,
		phase:       phasePreparing,
		triggerType: triggerType,
		configName:  name,
		startedAt:   time.Now(),
	}
	o.mu.Unlock()

	go o.run(ctx, rc, name, triggerType, stopOnly)
	return nil
}

// Cancel は進行中の再起動を中止する（①②③のみ）。④以降は不可。
func (o *restartOrchestrator) Cancel() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.p.inProgress {
		return errNoRestartInProgress
	}
	if o.p.phase == phaseRestarting || o.p.phase == phaseUpdating {
		return errRestartNotCancellable // ④（更新中含む）は中止不可。更新の中断は /steam/cancel 側で行う
	}
	o.p.cancelled = true
	if o.cancel != nil {
		o.cancel()
	}
	return nil
}

func (o *restartOrchestrator) snapshot() restartProgress {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.p
}

// setParent は進行中フローの親 ctx を差し替える（Start で bg ctx に・shutdown で ①②③ を cancel）。
func (o *restartOrchestrator) setParent(ctx context.Context) {
	o.mu.Lock()
	o.parentCtx = ctx
	o.mu.Unlock()
}

// parent は現在の親 ctx を返す（beforeStart フックの更新 ctx に使う＝shutdown で更新も中断）。
func (o *restartOrchestrator) parent() context.Context {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.parentCtx == nil {
		return context.Background()
	}
	return o.parentCtx
}

func (o *restartOrchestrator) run(ctx context.Context, rc config.Restart, name, triggerType string, stopOnly bool) {
	defer o.finish()

	// 0人なら ①②③ を飛ばして即終端（再起動 or 停止）。
	if total, err := o.totalUsers(ctx); err == nil && total == 0 {
		if o.enterRestarting() {
			o.doTerminal(name, triggerType, stopOnly)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}

	// ① セッション変更（トリガー時に即発火・§3.16(1)）。
	o.applySessionChanges(ctx, rc.PreActions.SessionChanges)
	if ctx.Err() != nil {
		return
	}

	// ② 静かに待機（合計人数監視）。2区間モデル（R9）: 締切 = quiet+announce、
	// ③告知は quiet 経過時点（＝締切の announce 前）に1回。
	quiet := time.Duration(rc.WaitControl.QuietWaitMin) * o.minute
	announce := time.Duration(rc.WaitControl.AnnounceWaitMin) * o.minute
	deadline := time.Now().Add(quiet + announce)
	o.setWaiting(deadline)
	announced := false
	ticker := time.NewTicker(o.waitInterval)
	defer ticker.Stop()

loop:
	for {
		total, err := o.totalUsers(ctx)
		if ctx.Err() != nil {
			return // ②③中の cancel
		}
		if o.driver.Status().State == headless.StateStopped {
			break // フロー中にヘッドレスが落ちた → 締切を待たず即 ④ で復帰（Stop は空振り・Start で起動）
		}
		switch decideWait(total, err != nil, time.Until(deadline), announce, announced, rc.PreActions.Announce.Enabled) {
		case waitRestart:
			break loop
		case waitAnnounce:
			o.setPhase(phaseAnnouncing)
			o.announce(ctx, rc.PreActions.Announce)
			announced = true
			if ctx.Err() != nil {
				return
			}
			o.setPhase(phaseWaiting)
		case waitContinue:
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}

	// ④ 強制終端＝再起動 or 停止（cancel 不可）。
	if o.enterRestarting() {
		o.doTerminal(name, triggerType, stopOnly)
	}
}

// doTerminal は ④ の終端動作。stopOnly なら停止のみ（通常停止・R7）、そうでなければ停止→起動（再起動）。
func (o *restartOrchestrator) doTerminal(name, triggerType string, stopOnly bool) {
	if stopOnly {
		o.doStop()
		return
	}
	o.doRestart(name, triggerType)
}

// doStop は ④（通常停止）。停止して StateStopped を待つ。起動も最終起動記録もしない（cancel 不可）。
func (o *restartOrchestrator) doStop() {
	_ = o.driver.Stop() // 既に停止していれば ErrNotRunning（無視）
	o.waitStopped()
}

// waitAction は ② のティックでの判断（decideWait の戻り）。
type waitAction int

const (
	waitContinue waitAction = iota
	waitAnnounce
	waitRestart
)

// decideWait は ② のループ1ティックでの行動を決める純関数（タイミングと分離してテスト可能に）。
//   - 人数取得成功で0人 → 即再起動
//   - 締切到達 → 強制再起動
//   - 告知有効・未告知・残り <= actionTiming → 告知
//   - それ以外 → 継続
//
// 人数取得失敗（totalErr）は「0人」と誤認しない（待機継続。最終的に締切で強制）。
func decideWait(total int, totalErr bool, remaining, actionTiming time.Duration, announced, announceEnabled bool) waitAction {
	if !totalErr && total == 0 {
		return waitRestart
	}
	if remaining <= 0 {
		return waitRestart
	}
	if announceEnabled && !announced && remaining <= actionTiming {
		return waitAnnounce
	}
	return waitContinue
}

// enterRestarting は phase を restarting に進める（以降 cancel 不可）。
// 直前に cancel 済みなら false を返し、再起動を行わない（cancel 境界の原子化）。
func (o *restartOrchestrator) enterRestarting() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.p.cancelled {
		return false
	}
	o.p.phase = phaseRestarting
	o.p.deadlineAt = time.Time{}
	return true
}

func (o *restartOrchestrator) setWaiting(deadline time.Time) {
	o.mu.Lock()
	o.p.phase = phaseWaiting
	o.p.deadlineAt = deadline
	o.mu.Unlock()
}

func (o *restartOrchestrator) setPhase(phase string) {
	o.mu.Lock()
	o.p.phase = phase
	o.mu.Unlock()
}

func (o *restartOrchestrator) finish() {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel() // context を解放（親 bg ctx の children から外す＝再起動毎のリーク防止）
	}
	o.cancel = nil
	o.p = restartProgress{phase: phaseIdle}
	o.mu.Unlock()
}

// presentUserCount は全ワールドの在席ユーザー合計。
// ホスト（ヘッドレス自身）は実機採取(2026-05-28: ホストのみ=Users:1/Present:0)で
// Present:False のため自然に除外される（"-1" ハックも「ホスト=各ワールド1人」前提も不要）。
func presentUserCount(worlds []headless.World) int {
	sum := 0
	for _, w := range worlds {
		sum += w.Present
	}
	return sum
}

// totalUsers は全ワールドの在席ユーザー合計（ホスト除外）。取得失敗は err を返す
// （呼び出し側で0人と区別する）。run() の即時再起動判定と decideWait の両方が共有する。
func (o *restartOrchestrator) totalUsers(ctx context.Context) (int, error) {
	worlds, err := o.worlds.List(ctx)
	if err != nil {
		return -1, err
	}
	return presentUserCount(worlds), nil
}

// applySessionChanges は ① 各ワールドに accesslevel/maxusers/name を適用（best-effort）。
// 各項目は独立トグル。全 OFF なら何もしない。個々の Exec 失敗は無視して次ワールドへ。
func (o *restartOrchestrator) applySessionChanges(ctx context.Context, sc config.SessionChanges) {
	if !sc.SetPrivate && !sc.SetMaxUsersOne && !(sc.RenameEnabled && sc.RenameTo != "") {
		return
	}
	_ = o.worlds.ForEach(ctx, func(_ headless.World, scope headless.Scope) error {
		if sc.SetPrivate {
			_, _ = scope.Exec("accesslevel Private", headless.WithTimeout(2*time.Second))
		}
		if sc.SetMaxUsersOne {
			_, _ = scope.Exec("maxusers 1", headless.WithTimeout(2*time.Second))
		}
		if sc.RenameEnabled && sc.RenameTo != "" {
			_, _ = scope.Exec(fmt.Sprintf("name %q", sc.RenameTo), headless.WithTimeout(2*time.Second))
		}
		return nil
	})
}

// announce は ③ dynamicImpulse 告知。itemUrl 非空なら全ワールドでspawn完了確認後、
// execMuを解放してspawnDelay待機し、成功ワールドだけへimpulseする。
// itemUrl 空＝常設受け機構前提で spawn を省略し impulse のみ（§3.16(2)）。
// テンプレ参照（templateId 非空）は実行直前に URL/タグへ解決する＝リモートリストの更新が
// 保存済み config にも即反映される（docs/design/item-templates.md）。
func (o *restartOrchestrator) announce(ctx context.Context, a config.AnnounceAction) {
	if a.TemplateID != "" {
		if o.resolveAnnounce == nil {
			log.Printf("[restart] 告知テンプレートの解決手段が無いため告知をスキップ: %s", a.TemplateID)
			return
		}
		resolved, ok := o.resolveAnnounce(ctx, a)
		if !ok {
			log.Printf("[restart] 告知テンプレートを解決できないため告知をスキップ: %s", a.TemplateID)
			return
		}
		a = resolved
	}
	impulseCmd := headless.DynamicImpulseStringCmd(a.ImpulseTag, a.Message)
	if a.ItemURL != "" {
		spawnedWorlds := make(map[int]string)
		err := o.worlds.ForEach(ctx, func(world headless.World, scope headless.Scope) error {
			if err := execTemporarySpawn(scope, a.ItemURL); err != nil {
				log.Printf("[restart] world #%d の告知アイテムspawn失敗（impulseをスキップ）: %v", world.Index, err)
				return nil // spawn失敗だけを飛ばし、次ワールドへ進む
			}
			spawnedWorlds[world.Index] = world.Name
			return nil
		})
		if err != nil && ctx.Err() == nil {
			log.Printf("[restart] 告知アイテムspawnのワールド巡回失敗: %v", err)
		}
		if len(spawnedWorlds) == 0 {
			return
		}
		select { // ForEachの外で待ち、execMuを500ms保持し続けない
		case <-ctx.Done():
			return
		case <-time.After(o.spawnDelay):
		}
		err = o.worlds.ForEach(ctx, func(world headless.World, scope headless.Scope) error {
			if name, ok := spawnedWorlds[world.Index]; !ok || name != world.Name {
				return nil
			}
			if _, err := scope.Exec(impulseCmd, headless.WithTimeout(2*time.Second)); err != nil {
				log.Printf("[restart] world #%d の告知impulse送信失敗: %v", world.Index, err)
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			log.Printf("[restart] 告知impulseのワールド巡回失敗: %v", err)
		}
		return
	}
	_ = o.worlds.ForEach(ctx, func(_ headless.World, scope headless.Scope) error {
		_, _ = scope.Exec(impulseCmd, headless.WithTimeout(2*time.Second))
		return nil
	})
}

// doRestart は ④ 停止→（StateStopped 待ち）→（予定再起動なら更新）→選択 config で起動。cancel 不可。
func (o *restartOrchestrator) doRestart(name, triggerType string) {
	_ = o.driver.Stop() // 既に停止していれば ErrNotRunning（無視）
	o.waitStopped()
	// 予定再起動なら停止完了後・起動前に Resonite を更新する（scheduled 限定・失敗でも起動継続・P9-B）。
	if o.beforeStart != nil {
		o.beforeStart(o.parent(), triggerType)
	}
	headlessPath, launchPath, err := o.resolve(name)
	if err != nil {
		log.Printf("[restart] config 解決に失敗（再起動を中断）: %v", err)
		return
	}
	if err := o.driver.Start(headlessPath, launchPath, name); err != nil {
		log.Printf("[restart] 起動に失敗（再起動を中断）: %v", err)
		return
	}
	o.recordUsed(name)
	if o.recordStart != nil {
		o.recordStart(triggerType, time.Now().Format(time.RFC3339)) // 最終起動を記録（§3.16(9)）
	}
}

func (o *restartOrchestrator) waitStopped() {
	deadline := time.Now().Add(o.stopWaitTimeout)
	for time.Now().Before(deadline) {
		if o.driver.Status().State == headless.StateStopped {
			return
		}
		time.Sleep(o.stopPollInterval)
	}
}
