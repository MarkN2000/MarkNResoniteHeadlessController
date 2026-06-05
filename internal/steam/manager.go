package steam

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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
	stallTimeout time.Duration                                                // 無進捗で打ち切る時間（既定5分）
	ensureDD     func(ctx context.Context, logf func(string)) (string, error) // DD本体を確保しパスを返す（テストで差し替え可）
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
	State      string     `json:"state"` // idle | running | success | failed
	Percent    float64    `json:"percent"`
	Phase      string     `json:"phase,omitempty"`
	File       string     `json:"file,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	LastError  string     `json:"lastError,omitempty"`
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
)

// UpdateParams は1回の入手/更新に必要なパラメータ（server が config から組む）。
type UpdateParams struct {
	InstallDir string
	Username   string
	Password   string
	BranchCode string
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
	m.ensureDD = func(ctx context.Context, logf func(string)) (string, error) {
		return acq.Ensure(ctx, m.toolsDir, logf)
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
	m.lastEvent = now
	return runCtx, nil
}

// run は acquire→DD実行→（非windowsは）chmod を行う。stall ウォッチドッグで無進捗を打ち切る。
func (m *Manager) run(runCtx context.Context, p UpdateParams) error {
	ddPath, err := m.ensureDD(runCtx, func(s string) { m.onEvent(Event{Kind: "log", Text: s}) })
	if err != nil {
		return fmt.Errorf("DepotDownloader の準備に失敗: %w", err)
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
	if wasStalled {
		return fmt.Errorf("更新が停滞したため中断しました（%s 無進捗）", m.stallTimeout)
	}
	if runErr != nil {
		// 親 ctx のキャンセル（ユーザー Cancel / shutdown）なら「中止」として明示する
		// （stall は watchCtx のみを cancel するため runCtx.Err() は nil＝上の分岐で先に処理済み）。
		if runCtx.Err() != nil {
			return errors.New("更新を中止しました")
		}
		return runErr
	}

	// DepotDownloader は実行権を付けないため、取得後に install 全体へ +x（Linux/ARM 必須）。
	if runtime.GOOS != "windows" {
		m.onEvent(Event{Kind: "log", Text: "実行権を付与中（chmod -R +x）"})
		if err := chmodTreeExec(p.InstallDir); err != nil {
			return fmt.Errorf("実行権の付与に失敗: %w", err)
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
	} else {
		m.st.State = stateSuccess
		m.st.Percent = 100
	}
	result := Event{Kind: "result"}
	if err != nil {
		result.Text = err.Error()
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
