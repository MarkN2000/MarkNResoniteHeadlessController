// Package headless はResoniteヘッドレスのプロセスを管理する Console Driver。
// 起動/停止、コマンド送信（stdin）、ログ収集（stdout/stderr）、状態管理、
// ログ/状態のSSE向けブロードキャストを担う。
//
// 注: 入出力は当面 UTF-8（Linuxヘッドレスは UTF-8）。Windowsのロケール
// コードページ対応は後続でプラットフォーム抽象に追加する。
package headless

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
)

// LogLine は1行のログ。Seqで重複排除/順序付けができる。
type LogLine struct {
	Seq  uint64    `json:"seq"`
	Time time.Time `json:"time"`
	Kind string    `json:"kind"` // out | err | sys | cmd
	Text string    `json:"text"`
}

// Status はヘッドレスの現在状態。
type Status struct {
	State     State      `json:"state"`
	PID       int        `json:"pid,omitempty"`
	Config    string     `json:"config,omitempty"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
	Ready     bool       `json:"ready"`
}

var (
	ErrAlreadyRunning = errors.New("ヘッドレスは既に起動しています")
	ErrNotRunning     = errors.New("ヘッドレスは起動していません")
	ErrNoPath         = errors.New("Resoniteヘッドレスのパスが未設定です")
)

const logCapacity = 2000

// Driver は単一のヘッドレスプロセスを管理する。
type Driver struct {
	mu sync.Mutex

	enc       encoding.Encoding // nil = UTF-8パススルー
	logHub    *hub[LogLine]
	statusHub *hub[Status]
	history   []LogLine
	seq       uint64

	cmd      *exec.Cmd
	stdin    io.WriteCloser
	state    State
	cfgPath  string
	pid      int
	started  time.Time
	ready    bool
	stopping bool
}

// NewDriver は文字コード enc（nil=UTF-8パススルー）でドライバを生成する。
func NewDriver(enc encoding.Encoding) *Driver {
	return &Driver{
		enc:       enc,
		logHub:    newHub[LogLine](),
		statusHub: newHub[Status](),
		state:     StateStopped,
	}
}

// --- 状態 ---

func (d *Driver) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statusLocked()
}

func (d *Driver) statusLocked() Status {
	var startedAt *time.Time
	if d.state != StateStopped && !d.started.IsZero() {
		t := d.started
		startedAt = &t
	}
	return Status{State: d.state, PID: d.pid, Config: d.cfgPath, StartedAt: startedAt, Ready: d.ready}
}

// setStateLocked は d.mu 保持中に呼ぶ。状態を更新し購読者へ通知する。
func (d *Driver) setStateLocked(s State) {
	d.state = s
	d.statusHub.publish(d.statusLocked())
}

func (d *Driver) publishLog(kind, text string) {
	d.mu.Lock()
	d.seq++
	line := LogLine{Seq: d.seq, Time: time.Now(), Kind: kind, Text: strings.ReplaceAll(text, "\r", "")}
	d.history = append(d.history, line)
	if len(d.history) > logCapacity {
		d.history = d.history[len(d.history)-logCapacity:]
	}
	d.mu.Unlock()
	d.logHub.publish(line)
}

// --- ライフサイクル ---

// Start はヘッドレスを起動する。headlessPath が空ならエラー。
func (d *Driver) Start(headlessPath, configPath string) error {
	d.mu.Lock()
	if d.state != StateStopped {
		d.mu.Unlock()
		return ErrAlreadyRunning
	}
	if headlessPath == "" {
		d.mu.Unlock()
		return ErrNoPath
	}

	cmd := platform.BuildHeadlessCommand(headlessPath, configPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		d.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		d.mu.Unlock()
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		d.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		d.mu.Unlock()
		return fmt.Errorf("起動失敗: %w", err)
	}

	d.cmd = cmd
	d.stdin = stdin
	d.cfgPath = configPath
	d.pid = cmd.Process.Pid
	d.started = time.Now()
	d.ready = false
	d.stopping = false
	d.setStateLocked(StateStarting)
	d.mu.Unlock()

	d.publishLog("sys", fmt.Sprintf("起動: pid=%d config=%q", cmd.Process.Pid, configPath))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); d.readPipe(stdout, "out") }()
	go func() { defer wg.Done(); d.readPipe(stderr, "err") }()
	go d.waitExit(cmd, &wg)
	return nil
}

func (d *Driver) readPipe(r io.Reader, kind string) {
	if d.enc != nil { // 非UTF-8（Windowsコードページ等）はUTF-8へデコード
		r = transform.NewReader(r, d.enc.NewDecoder())
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		text := sc.Text()
		d.publishLog(kind, text)
		if kind == "out" {
			d.maybeReady(text)
		}
	}
}

// maybeReady は readiness 合図（"World running" / "Engine Ready"）を検出して
// 状態を Running に上げる。実機観測: 起動直後の最初のコマンドが Unknown command
// になり得るため、UI側は Ready を待ってコマンドを送るとよい。
func (d *Driver) maybeReady(text string) {
	if !strings.Contains(text, "World running") && !strings.Contains(text, "Engine Ready") {
		return
	}
	d.mu.Lock()
	if !d.ready && d.state == StateStarting {
		d.ready = true
		d.setStateLocked(StateRunning)
		d.mu.Unlock()
		d.publishLog("sys", "ヘッドレス準備完了（コマンド受付可）")
		return
	}
	d.mu.Unlock()
}

func (d *Driver) waitExit(cmd *exec.Cmd, wg *sync.WaitGroup) {
	wg.Wait() // 全パイプをdrainしてから reap（Waitがパイプを閉じる前に読み切る＝末尾行の取りこぼし防止）
	err := cmd.Wait()
	d.mu.Lock()
	wasStopping := d.stopping
	d.cmd = nil
	d.stdin = nil
	d.pid = 0
	d.ready = false
	d.setStateLocked(StateStopped)
	d.mu.Unlock()

	if wasStopping {
		d.publishLog("sys", "ヘッドレスを停止しました")
	} else {
		d.publishLog("sys", fmt.Sprintf("ヘッドレスが終了しました（意図しない終了の可能性: %v）", err))
		// TODO(v1.x §5.6): クラッシュ自動復帰（設定ON時・クラッシュループ保護付き）
	}
}

// Stop は shutdown を送り、猶予後に強制終了する。
func (d *Driver) Stop() error {
	d.mu.Lock()
	if d.state == StateStopped || d.cmd == nil {
		d.mu.Unlock()
		return ErrNotRunning
	}
	d.stopping = true
	stdin := d.stdin
	cmd := d.cmd
	d.setStateLocked(StateStopping)
	d.mu.Unlock()

	d.publishLog("sys", "停止要求: shutdown を送信")
	if stdin != nil {
		_, _ = io.WriteString(stdin, "shutdown\n")
	}

	go func() {
		timer := time.NewTimer(60 * time.Second)
		defer timer.Stop()
		<-timer.C
		d.mu.Lock()
		stuck := d.cmd == cmd && d.state == StateStopping
		d.mu.Unlock()
		if stuck && cmd.Process != nil {
			d.publishLog("sys", "猶予超過: プロセスを強制終了します")
			_ = cmd.Process.Kill()
		}
	}()
	return nil
}

// SendCommand はヘッドレスのstdinへコマンドを送る。
func (d *Driver) SendCommand(command string) error {
	d.mu.Lock()
	stdin := d.stdin
	state := d.state
	d.mu.Unlock()
	if stdin == nil || (state != StateRunning && state != StateStarting) {
		return ErrNotRunning
	}
	d.publishLog("cmd", "> "+command)
	payload := command + "\n"
	if d.enc != nil { // 非UTF-8（Windowsコードページ等）へエンコードして送信
		if out, _, e := transform.String(d.enc.NewEncoder(), payload); e == nil {
			payload = out
		}
	}
	_, err := io.WriteString(stdin, payload)
	return err
}

// --- 購読（SSE用） ---

func (d *Driver) SubscribeLog(buf int) (chan LogLine, []LogLine) {
	ch := d.logHub.subscribe(buf)
	d.mu.Lock()
	hist := append([]LogLine(nil), d.history...)
	d.mu.Unlock()
	return ch, hist
}

func (d *Driver) UnsubscribeLog(ch chan LogLine) { d.logHub.unsubscribe(ch) }

func (d *Driver) SubscribeStatus(buf int) (chan Status, Status) {
	ch := d.statusHub.subscribe(buf)
	return ch, d.Status()
}

func (d *Driver) UnsubscribeStatus(ch chan Status) { d.statusHub.unsubscribe(ch) }
