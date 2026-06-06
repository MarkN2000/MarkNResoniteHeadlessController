package steam

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// resoniteAppID は Resonite の Steam App ID。
const resoniteAppID = "2519830"

// ddReadChunk は stdout/stderr のチャンク読みサイズ。
const ddReadChunk = 4096

// ErrTwoFactorRequired は DD が 2FA を要求した（Steam Guard オン）ことを表す。
// v1 は 2FA 入力 UI を持たないため、Guard オフの予備アカウント運用を案内する（設計 §6）。
var ErrTwoFactorRequired = errors.New("Steam の2要素認証(2FA)が要求されました。MRHC v1 は2FA未対応です（Steam Guard をオフにした予備アカウントを使用してください）")

// ErrAuthFailed は投入したパスワードが拒否され DD が再度パスワードを要求した（＝誤資格）ことを表す。
// 再要求にこれ以上応じても無限に失敗するため即終了し、5分の stall を待たずに原因を明示する（M1）。
var ErrAuthFailed = errors.New("Steam のユーザー名またはパスワードが正しくない可能性があります")

// ErrDDStartFailed は DepotDownloader プロセスの起動自体に失敗したことを表す（実行ファイル欠落等）。
// ErrDDFailed は起動後の異常終了（exit 非0）を表す。どちらも errorCode では "dd_failed" に写す。
// 文言はプレフィックス部のみ（詳細は発生箇所で "%w: %w" ラップして付加する・エラーコード規約は
// 設計 docs/design/steam-depotdownloader.md 参照）。
var (
	ErrDDStartFailed = errors.New("DepotDownloader の起動に失敗")
	ErrDDFailed      = errors.New("DepotDownloader が失敗しました")
)

// RunParams は DepotDownloader 1回の実行に必要なパラメータ。
type RunParams struct {
	DDPath     string // DepotDownloader 実行ファイル（acquire 済み）
	InstallDir string // Resonite の入手/更新先
	Username   string // Steam ユーザー名(A)
	Password   string // Steam パスワード（stdin 投入・ASCII）
	BranchCode string // headless branch password
}

// Event は DD 実行中に発生するイベント（manager が SSE へ流す）。
// 表示文言の多言語化のため、エラーは Code（result）、MRHC 生成ログ行は MsgKey/MsgArgs を併せて
// 持ち、表示層（Web UI）が locale 変換する。Text は従来どおりの原文（ja）＝未知 Code/MsgKey の
// フォールバックとして常に残す。
type Event struct {
	Kind    string            `json:"kind"`              // "log" | "progress" | "milestone" | "result"
	Text    string            `json:"text,omitempty"`    // log 行 / milestone 名 / result のエラー原文
	Percent float64           `json:"percent,omitempty"` // progress: 0..100
	File    string            `json:"file,omitempty"`    // progress: 対象パス
	Code    string            `json:"code,omitempty"`    // result: エラー分類コード（errorCode 参照）
	Detail  string            `json:"detail,omitempty"`  // result: 見出し（Code）を除いた診断詳細（HTTP 状態・exit 等）
	RunKind string            `json:"runKind,omitempty"` // result: run の種別（update / runtime。表示の出し分け用）
	MsgKey  string            `json:"msgKey,omitempty"`  // log: MRHC 生成行の文言キー
	MsgArgs map[string]string `json:"msgArgs,omitempty"` // log: MsgKey の補間引数（名前付き）
}

// Runner は DepotDownloader を実行する。状態を持たないため使い回し可。
type Runner struct{}

// NewRunner は Runner を返す。
func NewRunner() *Runner { return &Runner{} }

// Run は DepotDownloader を1回実行して Resonite を入手/更新する。
//   - パスワードは引数に出さず stdin で投入する（-password はプロセス一覧に露出するため不可）。
//   - 常に -remember-password を付ける（指定漏れの「stored credentials」再要求を避ける）。
//   - 2FA を要求されたら子プロセスを止めて ErrTwoFactorRequired を返す。
//
// ctx のキャンセル/締切で子プロセスは終了する（manager の stall タイムアウト・cancel で使う）。
// 進捗・ログは onEvent（nil 可）へ通知する。
func (r *Runner) Run(ctx context.Context, p RunParams, onEvent func(Event)) error {
	args := []string{
		"-app", resoniteAppID,
		"-branch", "headless", "-branchpassword", p.BranchCode,
		"-username", p.Username, "-remember-password",
		"-dir", p.InstallDir,
	}
	cmd := exec.CommandContext(ctx, p.DDPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %w", ErrDDStartFailed, err)
	}

	var twoFactor atomic.Bool
	var authFailed atomic.Bool
	var pwSent atomic.Bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.readStdout(stdout, stdin, p.Password, &pwSent, &twoFactor, &authFailed, cmd, onEvent)
	}()
	go func() { defer wg.Done(); r.readStderr(stderr, onEvent) }()
	wg.Wait() // パイプを drain し切ってから Wait（末尾出力の取りこぼし防止）

	waitErr := cmd.Wait()
	if twoFactor.Load() {
		return ErrTwoFactorRequired
	}
	if authFailed.Load() {
		return ErrAuthFailed
	}
	if waitErr != nil {
		return fmt.Errorf("%w: %w", ErrDDFailed, waitErr)
	}
	return nil
}

// readStdout は子プロセスの stdout をチャンク読みし、確定行は進捗/マイルストーン/ログとして
// onEvent へ流す。改行されない末尾断片（tail）はプロンプト候補として検査し、パスワード要求なら
// stdin へ投入、2FA 要求なら子プロセスを止める。
func (r *Runner) readStdout(stdout io.Reader, stdin io.WriteCloser, password string, pwSent, twoFactor, authFailed *atomic.Bool, cmd *exec.Cmd, onEvent func(Event)) {
	buf := make([]byte, ddReadChunk)
	var lineBuf []byte
	for {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				switch b {
				case '\n':
					emitLine(string(lineBuf), onEvent)
					lineBuf = lineBuf[:0]
				case '\r':
					// 行終端の前段。確定行から落とす（CRLF 対策）。
				default:
					lineBuf = append(lineBuf, b)
				}
			}
			// チャンク処理後の lineBuf は「次の改行まで未確定」＝プロンプト候補。
			switch detectPrompt(string(lineBuf)) {
			case promptPassword:
				if pwSent.CompareAndSwap(false, true) {
					_, _ = io.WriteString(stdin, password+"\n")
					// 処理済みプロンプト断片を破棄（直後の進捗行とグルーになるのを防ぐ）。
					lineBuf = lineBuf[:0]
				} else if authFailed.CompareAndSwap(false, true) && cmd.Process != nil {
					// パスワード送信済みなのに再度要求＝投入したパスワードが拒否された（誤資格）。
					// これ以上応じても失敗し続けるので即 kill し、stall を待たず ErrAuthFailed にする（M1）。
					_ = cmd.Process.Kill()
				}
			case promptTwoFactor:
				if twoFactor.CompareAndSwap(false, true) && cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			case promptNone:
			}
		}
		if readErr != nil {
			return
		}
	}
}

// readStderr は stderr を行単位でログイベントへ流す。
func (r *Runner) readStderr(stderr io.Reader, onEvent func(Event)) {
	buf := make([]byte, ddReadChunk)
	var lineBuf []byte
	for {
		n, readErr := stderr.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				switch b {
				case '\n':
					emit(onEvent, Event{Kind: "log", Text: string(lineBuf)})
					lineBuf = lineBuf[:0]
				case '\r':
				default:
					lineBuf = append(lineBuf, b)
				}
			}
		}
		if readErr != nil {
			if len(lineBuf) > 0 {
				emit(onEvent, Event{Kind: "log", Text: string(lineBuf)})
			}
			return
		}
	}
}

// emitLine は確定した stdout 1行を分類して onEvent へ流す。
//   - 進捗行（`X% path`）はログに流さない（行数が多くログを埋めるため）。
//   - マイルストーン行（`Pre-allocating <path>` 等）はファイル毎に大量に出るため、ログには
//     重複させずマイルストーンのみ通知する（dedup は manager 側で phase 変化時だけに行う・M4）。
//   - それ以外の行（接続/depot key/ログイン等）はログとして流す。
func emitLine(line string, onEvent func(Event)) {
	if pct, file, ok := parseProgress(line); ok {
		emit(onEvent, Event{Kind: "progress", Percent: pct, File: file})
		return
	}
	if ms, ok := detectMilestone(line); ok {
		emit(onEvent, Event{Kind: "milestone", Text: ms})
		return
	}
	emit(onEvent, Event{Kind: "log", Text: line})
}

func emit(onEvent func(Event), e Event) {
	if onEvent != nil {
		onEvent(e)
	}
}
