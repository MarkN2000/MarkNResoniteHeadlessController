package steam

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/pubsub"
)

// Manager は Resonite の入手/更新（acquire→DD実行→chmod）を束ねる単一入口。
// HTTP ハンドラ（非同期 Start）と restart-orchestrator（同期 Update）の両方から使い、
// single-flight で同時実行を1つに制限する。進捗/ログは pubsub で SSE 購読者へ配信する。
// 設計: docs/design/steam-depotdownloader.md §7
type Manager struct {
	toolsDir     string
	stallTimeout time.Duration                                                  // 無進捗で打ち切る時間（既定5分）
	ensureDD     func(ctx context.Context, onEvent func(Event)) (string, error) // DD本体を確保しパスを返す（テストで差し替え可）
	runner       *Runner

	hub *pubsub.Hub[Event]

	mu        sync.Mutex
	st        Status
	logs      []Event // log/milestone/result の履歴（SSE 再接続時にリプレイ）
	cancelFn  context.CancelFunc
	lastEvent time.Time       // 進捗停滞ウォッチドッグ用（最後にイベントを観測した時刻）
	parentCtx context.Context // 非同期 Start の親 ctx（shutdown で進行中を止めるため差し替え可）
}

// Status は更新の状態スナップショット（GET /steam/status・SSE 初期送出に使う）。
type Status struct {
	State       string     `json:"state"` // idle | running | success | failed
	Percent     float64    `json:"percent"`
	Phase       string     `json:"phase,omitempty"`
	File        string     `json:"file,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	LastError   string     `json:"lastError,omitempty"`   // エラー原文（ja・未知コードのフォールバック）
	ErrorCode   string     `json:"errorCode,omitempty"`   // エラー分類コード（表示層が locale 変換・errorCode 参照）
	ErrorDetail string     `json:"errorDetail,omitempty"` // 見出しを除いた診断詳細（errorDetail 参照）
}

const (
	stateIdle    = "idle"
	stateRunning = "running"
	stateSuccess = "success"
	stateFailed  = "failed"
)

const steamLogCapacity = 500

var (
	// ErrUpdateInProgress は更新が既に進行中であることを表す（single-flight）。
	ErrUpdateInProgress = errors.New("更新が既に進行中です")
	// ErrNoUpdateInProgress は中止対象の更新が無いことを表す。
	ErrNoUpdateInProgress = errors.New("進行中の更新がありません")
	// ErrSteamNotConfigured は Steam アカウント(A)・branch コード・install 先が未設定であることを表す。
	ErrSteamNotConfigured = errors.New("Steam アカウント設定が未設定です（ユーザー名・パスワード・branch コード・install 先）")
	// ErrCancelled は更新がキャンセル（ユーザー中止 / shutdown）で終わったことを表す。
	// 文言は従来の非 sentinel エラーと同一（Web UI 表示は不変）。ウィザード S5b が
	// 「失敗」と区別して専用文言を出すために sentinel 化した。
	ErrCancelled = errors.New("更新を中止しました")
	// ErrStalled は無進捗で打ち切られたことを表す（stallTimeout・文言はプレフィックス部のみ）。
	ErrStalled = errors.New("更新が停滞したため中断しました")
	// ErrVerifyMissing は DD exit 0 でも headless 実体が無いこと（＝ブランチコード誤りの
	// public フォールバック・H2）を表す。文言はプレフィックス部のみ。
	ErrVerifyMissing = errors.New("ダウンロードは完了しましたが headless 本体が見つかりません")
	// ErrAcquireFailed は DD 本体の取得（acquire）に失敗したことを表す。
	ErrAcquireFailed = errors.New("DepotDownloader の準備に失敗")
	// ErrChmodFailed は取得後の実行権付与に失敗したことを表す（Linux/ARM）。
	ErrChmodFailed = errors.New("実行権の付与に失敗")
)

// errorCode は err を表示層の locale 変換キーになる分類コードへ写す。文言（ja）でなくコードを
// 公開することで Web UI が言語を選べる（headless_not_installed と同じ流儀）。未知のエラーは
// "" ＝表示層は原文（LastError/Event.Text）へフォールバックする。中止系を最初に評価する。
func errorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrCancelled):
		return "cancelled"
	case errors.Is(err, ErrTwoFactorRequired):
		return "two_factor_required"
	case errors.Is(err, ErrAuthFailed):
		return "auth_failed"
	case errors.Is(err, ErrStalled):
		return "stalled"
	case errors.Is(err, ErrVerifyMissing):
		return "verify_missing"
	case errors.Is(err, ErrAcquireFailed):
		return "acquire_failed"
	case errors.Is(err, ErrDDStartFailed), errors.Is(err, ErrDDFailed):
		return "dd_failed"
	case errors.Is(err, ErrChmodFailed):
		return "chmod_failed"
	default:
		return ""
	}
}

// errorDetail は「<sentinel>: <内側>」型のエラーから内側の診断情報（HTTP 状態・exit code 等）を
// 取り出す。acquire/dd/chmod 系のみ＝見出しは locale 変換し、機械情報は原文で併記するため。
// 自己完結型（auth/2FA/stalled/verify/cancelled）は locale 文言だけで足りるので "" を返す。
func errorDetail(err error) string {
	for _, s := range []error{ErrAcquireFailed, ErrDDStartFailed, ErrDDFailed, ErrChmodFailed} {
		if errors.Is(err, s) {
			if detail, ok := strings.CutPrefix(err.Error(), s.Error()+": "); ok {
				return detail
			}
			return ""
		}
	}
	return ""
}

// UpdateParams は1回の入手/更新に必要なパラメータ（server が config から組む）。
type UpdateParams struct {
	InstallDir string
	Username   string
	Password   string
	BranchCode string
	// VerifyRelPath は DL 成功後に InstallDir 配下で存在を確認する相対パス（例 "Headless/Resonite.exe"）。
	// headless ブランチコードが誤りだと DD は public branch にフォールバックして exit 0 で終わるため、
	// このファイルの有無で「headless 実体が取れたか」を判定する（良性の "Password was invalid" 文字列
	// マッチは成功時にも出るため使わない・H2）。空なら検査しない（テスト等）。OS 名は server が DI する。
	VerifyRelPath string
}

// NewManager は既定（GitHub 取得・実 DepotDownloader・停滞5分）の Manager を返す。
// toolsDir は DD 本体の格納基点（例 {dataDir}/tools）。
func NewManager(toolsDir string) *Manager {
	acq := NewAcquirer()
	m := &Manager{
		toolsDir:     toolsDir,
		stallTimeout: 5 * time.Minute,
		runner:       NewRunner(),
		hub:          pubsub.NewHub[Event](),
		st:           Status{State: stateIdle},
		parentCtx:    context.Background(),
	}
	m.ensureDD = func(ctx context.Context, onEvent func(Event)) (string, error) {
		return acq.Ensure(ctx, m.toolsDir, onEvent)
	}
	return m
}

// SetParent は非同期 Start の親 ctx を差し替える（main の shutdown で進行中更新を止めるため）。
func (m *Manager) SetParent(ctx context.Context) {
	m.mu.Lock()
	m.parentCtx = ctx
	m.mu.Unlock()
}

// Update は同期的に入手/更新を行い、最終結果を返す（restart-orchestrator が停止→起動の間で使う）。
// ctx は呼び出し側の親（締切/キャンセル）。
func (m *Manager) Update(ctx context.Context, p UpdateParams) error {
	runCtx, err := m.begin(ctx, p)
	if err != nil {
		return err
	}
	err = m.run(runCtx, p)
	m.finish(err)
	return err
}

// Start は入手/更新を非同期で開始し、受付可否だけ即時に返す（HTTP ハンドラ用）。
// 進行は parentCtx の子として走る（リクエスト ctx には縛られない）。
func (m *Manager) Start(p UpdateParams) error {
	m.mu.Lock()
	parent := m.parentCtx
	m.mu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	runCtx, err := m.begin(parent, p)
	if err != nil {
		return err
	}
	go func() {
		m.finish(m.run(runCtx, p))
	}()
	return nil
}

// Cancel は進行中の更新を中止する。
func (m *Manager) Cancel() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.st.State != stateRunning {
		return ErrNoUpdateInProgress
	}
	if m.cancelFn != nil {
		m.cancelFn()
	}
	return nil
}

// Status は現在の状態スナップショットを返す。
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

// Subscribe は SSE 用のイベントチャンネルと、再接続時にリプレイする履歴を返す。
func (m *Manager) Subscribe(buf int) (chan Event, []Event) {
	ch := m.hub.Subscribe(buf)
	m.mu.Lock()
	hist := append([]Event(nil), m.logs...)
	m.mu.Unlock()
	return ch, hist
}

// Unsubscribe は購読を解除する。
func (m *Manager) Unsubscribe(ch chan Event) { m.hub.Unsubscribe(ch) }

// --- 内部 ---

// begin は single-flight スロットを確保し、状態を running にして実行用 ctx を返す。
func (m *Manager) begin(parent context.Context, p UpdateParams) (context.Context, error) {
	if p.InstallDir == "" || p.Username == "" || p.Password == "" || p.BranchCode == "" {
		return nil, ErrSteamNotConfigured
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.st.State == stateRunning {
		return nil, ErrUpdateInProgress
	}
	runCtx, cancel := context.WithCancel(parent)
	m.cancelFn = cancel
	now := time.Now()
	m.st = Status{State: stateRunning, StartedAt: &now}
	// 前回 run の履歴は持ち越さない＝SSE の再接続/途中参加が今回の run のログだけをリプレイする
	// （再接続時に古い run の行が混ざる・二重に見える問題の根治）。
	m.logs = nil
	m.lastEvent = now
	return runCtx, nil
}

// run は acquire→DD実行→（非windowsは）chmod を行う。stall ウォッチドッグで無進捗を打ち切る。
func (m *Manager) run(runCtx context.Context, p UpdateParams) error {
	ddPath, err := m.ensureDD(runCtx, m.onEvent)
	if err != nil {
		// acquire 段階の中断（ユーザー Cancel / shutdown）は ctx 起因の生エラーで返るため、
		// runner 後の分岐（下）と同様にここでも「中止」へ正規化する（acquire_failed への誤分類防止）。
		if runCtx.Err() != nil {
			return ErrCancelled
		}
		return fmt.Errorf("%w: %w", ErrAcquireFailed, err)
	}

	// 進捗停滞ウォッチドッグ: 一定時間イベントが無ければ watchCtx を cancel（DD を kill）。
	watchCtx, watchCancel := context.WithCancel(runCtx)
	defer watchCancel()
	var stalled bool
	var stallMu sync.Mutex
	go m.stallWatch(watchCtx, func() {
		stallMu.Lock()
		stalled = true
		stallMu.Unlock()
		watchCancel()
	})

	rp := RunParams{DDPath: ddPath, InstallDir: p.InstallDir, Username: p.Username, Password: p.Password, BranchCode: p.BranchCode}
	runErr := m.runner.Run(watchCtx, rp, m.onEvent)
	watchCancel()

	stallMu.Lock()
	wasStalled := stalled
	stallMu.Unlock()
	// runErr == nil は「ウォッチドッグ発火と同時に DD が正常終了していた」稀な競合＝実体は成功
	// なので stalled にしない（成功を停滞と誤報告しない）。
	if wasStalled && runErr != nil {
		return fmt.Errorf("%w（%s 無進捗）", ErrStalled, m.stallTimeout)
	}
	if runErr != nil {
		// 親 ctx のキャンセル（ユーザー Cancel / shutdown）なら「中止」として明示する
		// （stall は watchCtx のみを cancel するため runCtx.Err() は nil＝上の分岐で先に処理済み）。
		if runCtx.Err() != nil {
			return ErrCancelled
		}
		return runErr
	}

	// DD が exit 0 でも、ブランチコード誤りだと headless depot が public フォールバックして
	// headless 実体が落ちないことがある。期待ファイルの存在で「headless が取れたか」を確認する（H2）。
	if p.VerifyRelPath != "" {
		target := filepath.Join(p.InstallDir, p.VerifyRelPath)
		if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w（%s）。ブランチコードが正しいか確認してください", ErrVerifyMissing, p.VerifyRelPath)
		}
	}

	// DepotDownloader は実行権を付けないため、取得後に install 全体へ +x（Linux/ARM 必須）。
	if runtime.GOOS != "windows" {
		m.onEvent(Event{Kind: "log", Text: "実行権を付与中（chmod -R +x）", MsgKey: "chmodding"})
		if err := chmodTreeExec(p.InstallDir); err != nil {
			return fmt.Errorf("%w: %w", ErrChmodFailed, err)
		}
	}
	return nil
}

// stallWatch は最後のイベントから stallTimeout 経過したら onStall を呼ぶ。
func (m *Manager) stallWatch(ctx context.Context, onStall func()) {
	if m.stallTimeout <= 0 {
		return
	}
	interval := m.stallTimeout / 4
	if interval <= 0 {
		interval = m.stallTimeout
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			idle := time.Since(m.lastEvent)
			m.mu.Unlock()
			if idle >= m.stallTimeout {
				onStall()
				return
			}
		}
	}
}

// onEvent は runner/acquire からのイベントを状態へ反映し、購読者へ配信する。
// 並行（stdout/stderr の2 goroutine）から呼ばれ得るため mu で保護する。
func (m *Manager) onEvent(e Event) {
	m.mu.Lock()
	m.lastEvent = time.Now()
	switch e.Kind {
	case "progress":
		m.st.Percent = e.Percent
		m.st.File = e.File
	case "milestone":
		// 同一 phase の連続マイルストーン（Pre-allocating が数百ファイル分続く等）は
		// publish/log しない＝SSE/ログの洪水と、500件リングからの早期マイルストーン追い出しを防ぐ（M4）。
		// lastEvent は上で更新済みなので stall ウォッチドッグは正しく活動継続とみなす。
		if e.Text == m.st.Phase {
			m.mu.Unlock()
			return
		}
		m.st.Phase = e.Text
		m.appendLogLocked(e)
	case "log":
		m.appendLogLocked(e)
	}
	m.mu.Unlock()
	m.hub.Publish(e)
}

// appendLogLocked は履歴リングバッファへ追加する（mu 保持前提）。
func (m *Manager) appendLogLocked(e Event) {
	m.logs = append(m.logs, e)
	if len(m.logs) > steamLogCapacity {
		m.logs = m.logs[len(m.logs)-steamLogCapacity:]
	}
}

// finish は状態を確定（success/failed）し、結果イベントを配信する。
func (m *Manager) finish(err error) {
	m.mu.Lock()
	if m.cancelFn != nil {
		m.cancelFn()
		m.cancelFn = nil
	}
	now := time.Now()
	m.st.FinishedAt = &now
	if err != nil {
		m.st.State = stateFailed
		m.st.LastError = err.Error()
		m.st.ErrorCode = errorCode(err)
		m.st.ErrorDetail = errorDetail(err)
	} else {
		m.st.State = stateSuccess
		m.st.Percent = 100
	}
	result := Event{Kind: "result"}
	if err != nil {
		result.Text = err.Error()
		result.Code = m.st.ErrorCode
		result.Detail = m.st.ErrorDetail
	}
	m.appendLogLocked(result)
	m.mu.Unlock()
	m.hub.Publish(result)
}

// chmodTreeExec は root 配下の全エントリに実行権(+x)を付与する（chmod -R +x 相当）。
func chmodTreeExec(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chmod(path, info.Mode()|0o111)
	})
}
