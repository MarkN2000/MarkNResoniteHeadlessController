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
	"regexp"
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

// LoginState は Resonite アカウントのログイン状態（起動ログから検出）。
type LoginState string

const (
	LoginAnonymous LoginState = "anonymous" // ログイン試行なし＝アカウント未設定の意図的匿名
	LoginLoggedIn  LoginState = "loggedIn"  // ログイン成功（LoginUserID 付き）
	LoginFailed    LoginState = "failed"    // ログイン試行はあったが成功確認なし
)

// loginUserIDRe は "Initializing SignalR: UserLogin: U-xxx" から UserID を抽出する
// （実機ログ・fixtures 2026-05-28-lan-login で確認・v1 踏襲）。
var loginUserIDRe = regexp.MustCompile(`Initializing SignalR: UserLogin:\s*(U-\S+)`)

// Status はヘッドレスの現在状態。
type Status struct {
	State       State      `json:"state"`
	PID         int        `json:"pid,omitempty"`
	Config      string     `json:"config,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	Ready       bool       `json:"ready"`
	LoginState  LoginState `json:"loginState"`            // anonymous|loggedIn|failed
	LoginUserID string     `json:"loginUserId,omitempty"` // 例 "U-xxxx"（loggedIn 時のみ）
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

// warmup は "World running" 検出後に送る捨てコマンド。起動直後の最初の1入力が
// 無視/Unknown 化する実機癖を身代わりに吸収し、ユーザーの実コマンドを常に2番目の
// 入力にするのが狙い。worlds は読み取り専用・必ず有効で、プロンプト復帰の確証にもなる。
const (
	warmupCommand = "worlds"
	warmupTimeout = 5 * time.Second
)

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

	// Resonite アカウントのログイン状態（起動ログから検出・d.mu 保護）。
	// 成功行 "Logged in successfully" は "World running" より前に出る（実機確認）ため ready 時点で確定。
	loginAttempted bool   // "Logging in as ..." を観測
	loginConfirmed bool   // "Logged in successfully" を観測
	loginUserID    string // "UserLogin: U-xxx" の U- 付き UserID

	// warmupStarted は "World running" 検出で warmup を一度だけ起動するためのガード（d.mu 保護）。
	warmupStarted bool

	// 構造化コマンド実行（案C'）の同期プリミティブ。詳細: docs/design/structured-driver.md
	//   - execMu: Exec の直列キュー（コマンド1個ずつ排他）／ExecGroup は同 mu を保持
	//   - activeCollector: 実行中の応答収集バッファ（読み手は readPipe、待ち手は waitComplete）
	execMu          sync.Mutex
	activeCollector atomic.Pointer[respCollector]
	// lastPrompt は直前コマンド完了時の検出プロンプト（＝次コマンド応答の「先頭グルー」）。
	// focus はプロンプトを変えるため、応答先頭のグルーは「直前コマンドのプロンプト」になる。
	// execMu 保持中（execLocked 内）でのみアクセスする。
	lastPrompt string

	// onUnexpectedExit は意図しない終了（クラッシュ）検知時に1回呼ばれる（設定時のみ）。
	// waitExit が wasStopping==false のとき mu 外で呼ぶ。中身は非ブロッキングに保つこと（crash-monitor §5.6）。
	onUnexpectedExit func()
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

// SetOnUnexpectedExit は意図しない終了（クラッシュ）の通知コールバックを登録する（§5.6・P8-4b）。
// 起動前に1回設定する想定。コールバックは waitExit の goroutine で走るため非ブロッキングに保つこと。
func (d *Driver) SetOnUnexpectedExit(fn func()) {
	d.mu.Lock()
	d.onUnexpectedExit = fn
	d.mu.Unlock()
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
	return Status{
		State: d.state, PID: d.pid, Config: d.cfgPath, StartedAt: startedAt, Ready: d.ready,
		LoginState: d.loginStateLocked(), LoginUserID: d.loginUserID,
	}
}

// loginStateLocked は観測フラグから現在のログイン状態を導出する（d.mu 保持前提）。
func (d *Driver) loginStateLocked() LoginState {
	switch {
	case d.loginConfirmed:
		return LoginLoggedIn
	case d.loginAttempted:
		return LoginFailed
	default:
		return LoginAnonymous
	}
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
//   - configPath: Resonite に渡す -HeadlessConfig の実ファイルパス（起動時生成の一時ファイル等）
//   - configLabel: Status().Config に表示する論理名（UI 表示用。一時パスを見せない）
func (d *Driver) Start(headlessPath, configPath, configLabel string) error {
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
	d.cfgPath = configLabel
	d.pid = cmd.Process.Pid
	d.started = time.Now()
	d.ready = false
	d.stopping = false
	d.warmupStarted = false
	d.loginAttempted = false
	d.loginConfirmed = false
	d.loginUserID = ""
	d.setStateLocked(StateStarting)
	d.mu.Unlock()

	d.publishLog("sys", fmt.Sprintf("起動: pid=%d config=%q", cmd.Process.Pid, configLabel))

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
//
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
						d.scanLogin(text)
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

// scanLogin は起動ログから Resonite アカウントのログイン状態を検出する。
// 実機ログ（fixtures 2026-05-28-lan-login）: 成功=「Logging in as X」→「…UserLogin: U-xxx」→
// 「Logged in successfully」（成功行は "World running" より前）／匿名（未設定）=これらが出ず
// 「Initial Startup」のみ／失敗=試行はあるが成功確認なし。手動 login コマンドにも追従する。
func (d *Driver) scanLogin(text string) {
	switch {
	case strings.HasPrefix(text, "Logging in as "):
		d.mu.Lock()
		// 新規ログイン試行＝成功/UserID をリセットしてから試行フラグを立てる（再ログイン追従）。
		d.loginAttempted = true
		d.loginConfirmed = false
		d.loginUserID = ""
		d.mu.Unlock()
	case strings.Contains(text, "Logged in successfully"):
		d.mu.Lock()
		d.loginConfirmed = true
		d.mu.Unlock()
	case strings.Contains(text, "UserLogin:"):
		if m := loginUserIDRe.FindStringSubmatch(text); m != nil {
			d.mu.Lock()
			d.loginUserID = m[1]
			d.mu.Unlock()
		}
	}
}

// maybeReady は readiness 合図 "World running..." を検出したら一度だけ warmup を起動する。
// "Engine Ready" は実機では World 起動・コンソール REPL 稼働の約3.6秒前に出るため、readiness
// 信号には採用しない（起動直後の最初の1コマンドが無視/Unknown 化する原因だった）。
// ready=true は warmup がプロンプト往復を確認（または縮退）した時点で初めて立てる。
func (d *Driver) maybeReady(text string) {
	if !strings.Contains(text, "World running") {
		return
	}
	d.mu.Lock()
	if d.warmupStarted || d.state != StateStarting {
		d.mu.Unlock()
		return
	}
	d.warmupStarted = true
	d.mu.Unlock()
	go d.warmup()
}

// warmup は "World running" 後に捨てコマンド(worlds)を1回送り、プロンプト復帰を待ってから
// ready=true にする。狙いは「ユーザーの最初の実コマンドを必ず2番目の入力にする」こと
// ＝起動直後の最初の1入力が無視/Unknown 化する実機癖を、この捨てコマンドが身代わりに吸収する。
// 送信した時点で目的は達成されるため、プロンプト確認は ready を早く立てるための確証にすぎない。
//   - 成功（プロンプト復帰）          → ready
//   - timeout（REPL 未応答だが送信済み）→ 縮退して ready（前進優先。最悪でも信号精緻化のみと同等）
//   - ErrProcessGone（起動中に死亡）   → ready にしない（waitExit が Stopped にする）
//
// 別 goroutine で走らせるのは必須: 応答を流す readPipe(out) を塞がないため。
// 副次効果: ここで d.lastPrompt が埋まり、最初の実コマンドのプロンプト剥がしも正しく効く。
func (d *Driver) warmup() {
	d.execMu.Lock()
	_, err := d.execLocked(context.Background(), warmupCommand, WithTimeout(warmupTimeout))
	d.execMu.Unlock()
	if errors.Is(err, ErrProcessGone) {
		return
	}
	d.mu.Lock()
	if d.state != StateStarting {
		d.mu.Unlock()
		return
	}
	d.ready = true
	d.setStateLocked(StateRunning)
	d.mu.Unlock()
	if err == nil {
		d.publishLog("sys", "ヘッドレス準備完了（コマンド受付可）")
	} else {
		d.publishLog("sys", "ヘッドレス準備完了（ウォームアップ未応答のため縮退）")
	}
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
	onExit := d.onUnexpectedExit
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
		if onExit != nil {
			onExit() // §5.6 クラッシュ自動復帰: crash-monitor へ非ブロッキング通知（mu 外で呼ぶ）
		}
	}
}

// Stop は shutdown を送り、猶予（180秒）後に強制終了する。
// ワールド保存に数分かかる場合があるため即 kill せず長めの猶予を取る。
// （v1 は 60s SIGTERM / 70s SIGKILL の段階式。Windows は SIGTERM 相当が無いため
//
//	単段の force-kill とし、その分猶予を長くした。設計: phase-7-spec レビュー 2026-05-29）
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
// ⚠️ shutdown は Exec の対象外：プロンプトが返らず必ず timeout になる → Driver.Stop() を使う。
//    restart/save/close は実機検証(2026-05-30)で **プロンプトを返す**ことを確認済み
//    （空ワールドで restart≈1s／save・close≈0.2s）。よって ExecGroup で実行可。
//    ただし restart は重いワールドだと時間がかかり得るため呼び出し側で長めの timeout を指定する。

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

	// prompt-prefix 剥がしは Driver 側で行う（検出した「実プロンプト」をリテラルに剥がす）。
	// 応答先頭のグルーは「直前コマンド完了時のプロンプト」(d.lastPrompt)＝focus 変更時は
	// <旧><新> の連結になるため、それを剥がす。さらに念のため今回のプロンプトも剥がす。
	// 値の '>'（リッチテキスト/<br>）やセッション名の ':' には影響しない（貪欲ヒューリスティック廃止）。
	// ambient 行は parser regex が自然に無視。
	lines, err := c.waitComplete(ctx, cfg)
	cur := d.decodeLine(c.prompt())
	lines = stripExactPrompt(lines, d.lastPrompt)
	lines = stripExactPrompt(lines, cur)
	if cur != "" {
		d.lastPrompt = cur
	}
	return lines, err
}
