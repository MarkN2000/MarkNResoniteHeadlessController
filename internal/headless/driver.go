// Package headless はResoniteヘッドレスのプロセスを管理する Console Driver。
// 起動/停止、コマンド送信（stdin）、ログ収集（stdout/stderr）、状態管理、
// ログ/状態のSSE向けブロードキャストを担う。
// 文字コードは platform.ConsoleEncoding で抽象化（Win=コードページ/Linux=UTF-8、
// nil=UTF-8パススルー）。stdoutは行単位でデコード、stdinはエンコードして送る。
package headless

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
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

// maxLineBytes は改行が来ない病的に長い1行に対する安全網。
// 旧 readPipe は bufio.NewReaderSize(r, 64*1024) で 64KB 上限があり、本実装は
// チャンク読みのため明示的に上限を設ける。実 Resonite では到達しないが、
// 不正な出力でメモリ無限増を起こさないため。超過時は強制的に1行として切り出す。
const maxLineBytes = 1 << 20 // 1 MiB

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

	// 構造化コマンド実行（案C'）の同期プリミティブ。詳細: docs/design/structured-driver.md
	//   - execMu: Exec の直列キュー（コマンド1個ずつ排他）／ExecGroup は同 mu を保持
	//   - activeCollector: 実行中の応答収集バッファ（読み手は readPipe、待ち手は waitComplete）
	execMu          sync.Mutex
	activeCollector atomic.Pointer[respCollector]
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
	if _, err := os.Stat(headlessPath); err != nil {
		d.mu.Unlock()
		return fmt.Errorf("ヘッドレスパスが見つかりません: %s", headlessPath)
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

// readPipe は子プロセスの stdout/stderr をチャンク読みしながら行に分解する。
// 設計（案C'）: 構造化実行中は activeCollector に対し、
//   - '\n' 確定毎に appendLine（decode 済テキスト）
//   - チャンク処理後の lineBuf（未確定 raw バイト＝プロンプト候補）を updateTail
// で通知する。これにより waitComplete が「プロンプト末尾 + 安定窓」で完了検出できる。
// stdout のみコレクタに流す（stderr の '>' はプロンプトと混同しないため）。
func (d *Driver) readPipe(r io.Reader, kind string) {
	buf := make([]byte, 4096)
	var lineBuf []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				switch b {
				case '\n':
					text := d.decodeLine(lineBuf)
					d.publishLog(kind, text)
					if kind == "out" {
						d.maybeReady(text)
						if c := d.activeCollector.Load(); c != nil {
							c.appendLine(text)
						}
					}
					lineBuf = lineBuf[:0]
				case '\r':
					// 行終端の前段。decodeLine 側でも吸収されるが、ここで落とすほうがシンプル。
				default:
					if len(lineBuf) >= maxLineBytes {
						// 改行が来ない病的長行: 強制的に1行として記録（メモリ無限増を防ぐ安全網）
						d.publishLog("sys", fmt.Sprintf("[truncate] %s: 単一行が %d bytes を超えたため強制改行", kind, maxLineBytes))
						text := d.decodeLine(lineBuf)
						d.publishLog(kind, text)
						if kind == "out" {
							if c := d.activeCollector.Load(); c != nil {
								c.appendLine(text)
							}
						}
						lineBuf = lineBuf[:0]
					}
					lineBuf = append(lineBuf, b)
				}
			}
			// チャンク処理後の lineBuf は「次の '\n' まで未確定の bytes」＝プロンプト候補
			if kind == "out" {
				if c := d.activeCollector.Load(); c != nil {
					c.updateTail(lineBuf)
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				d.publishLog("sys", fmt.Sprintf("ログ読取終了: %v", err))
			}
			return
		}
	}
}

// decodeLine は1行ぶんの生バイトを文字コードに応じてUTF-8文字列へ変換する。
// 行単位で独立にデコードするため、不正バイトがあってもその行に閉じ込められ、
// stdoutのdrain自体は止まらない（=プロセスのstdout詰まり/ハングを防ぐ）。
func (d *Driver) decodeLine(raw []byte) string {
	raw = bytes.TrimRight(raw, "\r\n")
	if d.enc == nil {
		return string(raw) // UTF-8パススルー
	}
	decoded, _, err := transform.Bytes(d.enc.NewDecoder(), raw)
	if err != nil {
		return string(raw) // デコード失敗時は生バイトにフォールバック（行は失わない）
	}
	return string(decoded)
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
	// 実行中の Exec があればプロセス死亡を通知（waitComplete が ErrProcessGone を返す）
	// 元エラー（cmd.Wait の戻り値）はそのまま渡す → waitComplete が %v で wrap する
	if c := d.activeCollector.Load(); c != nil {
		c.markGone(err)
	}
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

// Stop は shutdown を送り、猶予（180秒）後に強制終了する。
// ワールド保存に数分かかる場合があるため即 kill せず長めの猶予を取る。
// （v1 は 60s SIGTERM / 70s SIGKILL の段階式。Windows は SIGTERM 相当が無いため
//  単段の force-kill とし、その分猶予を長くした。設計: phase-7-spec レビュー 2026-05-29）
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
		timer := time.NewTimer(180 * time.Second)
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

// SendCommand はヘッドレスのstdinへコマンドを送る（fire-and-forget）。
//
// 並行性: execMu を取得することで、構造化 Exec/ExecGroup と stdin への書き込みを
// 直列化する。これがないと、構造化 Exec の応答収集ウィンドウ内に SendCommand の
// 出力が混入し、構造化レスポンスが壊れる。
// 待機時間はせいぜい走行中の Exec が終わるまで（既定 5s）。
func (d *Driver) SendCommand(command string) error {
	d.mu.Lock()
	stdin := d.stdin
	state := d.state
	d.mu.Unlock()
	if stdin == nil || (state != StateRunning && state != StateStarting) {
		return ErrNotRunning
	}

	// 構造化 Exec との混入を防ぐため execMu を取得
	d.execMu.Lock()
	defer d.execMu.Unlock()

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

// --- 構造化コマンド実行（案C'） ---
// 詳細: docs/design/structured-driver.md
//   - Exec: 1コマンドを送って応答行を構造化用に返す（直列キュー）
//   - ExecGroup: 同 mu を保持したまま複数 Exec を連続実行（focus→status 等の原子的グループ）
//
// ⚠️ 終了系コマンド（shutdown/restart/close）は Exec の対象外：
//    プロンプトが返らず必ず timeout になる。shutdown は Driver.Stop() を使うこと。

// Tx は ExecGroup の fn 内で使う Exec ハンドル。execMu は ExecGroup 側で保持済。
type Tx interface {
	Exec(cmd string, opts ...ExecOption) ([]string, error)
}

type txImpl struct {
	d   *Driver
	ctx context.Context
}

func (t *txImpl) Exec(cmd string, opts ...ExecOption) ([]string, error) {
	return t.d.execLocked(t.ctx, cmd, opts...)
}

// Exec は 1 コマンドを送って応答を構造化用に返す。execMu で直列化される。
// 戻り値は「プロンプト接頭辞除去済」の行リスト。timeout 等のエラー時も部分結果を含む。
func (d *Driver) Exec(ctx context.Context, cmd string, opts ...ExecOption) ([]string, error) {
	if err := d.checkReady(); err != nil {
		return nil, err
	}
	d.execMu.Lock()
	defer d.execMu.Unlock()
	return d.execLocked(ctx, cmd, opts...)
}

// ExecGroup は fn 内で行われる Exec を「他の Exec から割込まれない」原子的なグループとして実行する。
// 例: focus N → status を確実に同じワールドで取る。
func (d *Driver) ExecGroup(ctx context.Context, fn func(tx Tx) error) error {
	if err := d.checkReady(); err != nil {
		return err
	}
	d.execMu.Lock()
	defer d.execMu.Unlock()
	return fn(&txImpl{d: d, ctx: ctx})
}

// checkReady は Driver が構造化コマンドを受け付けられるかを確認する。
func (d *Driver) checkReady() error {
	d.mu.Lock()
	stdin := d.stdin
	ready := d.ready
	state := d.state
	d.mu.Unlock()
	if stdin == nil || !ready || state != StateRunning {
		return ErrNotReady
	}
	return nil
}

// execLocked は execMu を保持している前提で 1 コマンドを実行する。
// Exec / ExecGroup 経由でのみ呼ばれる（execMu 取得は呼び出し側責務）。
func (d *Driver) execLocked(ctx context.Context, cmd string, opts ...ExecOption) ([]string, error) {
	cfg := defaultExecConfig()
	for _, o := range opts {
		o(&cfg)
	}
	d.mu.Lock()
	stdin := d.stdin
	d.mu.Unlock()
	if stdin == nil {
		return nil, ErrNotReady
	}

	c := newRespCollector()
	d.activeCollector.Store(c)
	defer d.activeCollector.Store(nil)

	d.publishLog("cmd", "> "+cmd)
	payload := cmd + "\n"
	if d.enc != nil {
		if out, _, e := transform.String(d.enc.NewEncoder(), payload); e == nil {
			payload = out
		}
	}
	if _, err := io.WriteString(stdin, payload); err != nil {
		return nil, fmt.Errorf("send failed: %w", err)
	}

	// 注: prompt-prefix 剥がしは parser 側 (stripLineLeadingPrompts) で per-line に行う。
	// Driver は raw lines を返す（ambient と応答が混在し得る性質に合わせて parser 責任に統一）。
	return c.waitComplete(ctx, cfg)
}
