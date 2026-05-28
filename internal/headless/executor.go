package headless

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// 構造化コマンド実行の中核。設計詳細: docs/design/structured-driver.md §4 完了検出（案C'）。
//
// 役割分担:
//   - respCollector: stdout から流れてくる行と pending tail（最後の改行以降のバイト列）を保持。
//     waitComplete で「pending tail が '>' で終わる + 安定窓を満たす」を検出。
//   - Driver: stdin への書き込み + readPipe での collector への push + Exec の窓口（直列キュー）。
//
// 設計の心臓部:
//   - 完了シグナル = プロンプト末尾の '>'（負荷/応答時間に非依存）
//   - 直列化 = Driver の execMu で 1 コマンドずつ
//   - 原子的グループ = 同 mu を保持したまま複数 Exec（ExecGroup）

// ExecOption は Exec の挙動を 1 個ずつ上書きする関数型。
type ExecOption func(*execConfig)

type execConfig struct {
	MaxTimeout       time.Duration // コマンド全体のタイムアウト
	SettleConfirm    time.Duration // pending tail が '>' になってから「変化なし」を確認する窓
	ReadChunkTimeout time.Duration // pending を再評価するポーリング間隔
}

func defaultExecConfig() execConfig {
	return execConfig{
		MaxTimeout:       5 * time.Second,
		SettleConfirm:    50 * time.Millisecond,
		ReadChunkTimeout: 20 * time.Millisecond,
	}
}

// WithTimeout は cmd 毎に最大待ち時間を上書きする。
// 既定 5s。startworldurl 等の重いコマンドは大きく取る（例: 60s）。
func WithTimeout(d time.Duration) ExecOption {
	return func(c *execConfig) { c.MaxTimeout = d }
}

// WithSettleConfirm は「pending tail が '>' で安定」と判定するための窓を上書きする。
// 既定 50ms。短くしすぎると応答行の途中の `>` を誤検出する（行末改行で守られているので
// 通常は問題にならない）。
func WithSettleConfirm(d time.Duration) ExecOption {
	return func(c *execConfig) { c.SettleConfirm = d }
}

// 標準エラー（センチネル）。呼び出し側は errors.Is で判別する。
var (
	ErrNotReady    = errors.New("headless: not ready")
	ErrTimeout     = errors.New("headless: command timeout")
	ErrProcessGone = errors.New("headless: process gone during exec")
	ErrCanceled    = errors.New("headless: canceled")
)

// markGone(nil) で使う sentinel。allocate 回避のため package var で保持。
var errCleanExit = errors.New("clean exit")

// respCollector は実行中の応答収集バッファ。
// readPipe（生産者）と Exec の waitComplete（消費者）が共有する。
type respCollector struct {
	mu          sync.Mutex
	lines       []string // 改行で確定した行（decode 済）
	pendingTail []byte   // 最後の '\n' 以降の raw バイト（プロンプト検出用）
	lastChange  time.Time
	gone        error // プロセス死亡時に設定される
}

func newRespCollector() *respCollector {
	return &respCollector{lastChange: time.Now()}
}

// appendLine: readPipe が行を確定したときに呼ぶ。decode 済テキストを追加。
// pendingTail は新しい行が始まるのでリセット。
func (c *respCollector) appendLine(line string) {
	c.mu.Lock()
	c.lines = append(c.lines, line)
	c.pendingTail = c.pendingTail[:0]
	c.lastChange = time.Now()
	c.mu.Unlock()
}

// updateTail: readPipe が「未確定（改行未到来）のバイト列」を更新したときに呼ぶ。
// プロンプト検出はこの tail の末尾バイトを見る。
func (c *respCollector) updateTail(tail []byte) {
	c.mu.Lock()
	if len(tail) == 0 {
		c.pendingTail = c.pendingTail[:0]
	} else {
		c.pendingTail = append(c.pendingTail[:0], tail...)
	}
	c.lastChange = time.Now()
	c.mu.Unlock()
}

// markGone: 監視中プロセスが終了したことを伝える。以降 waitComplete は
// fmt.Errorf("%w: %v", ErrProcessGone, err) を返す（errors.Is で ErrProcessGone 判定可
// + err.Error() で元エラー情報も得られる）。
// 正常終了（err==nil）でも「プロセスが消えた」状態は同じなので clean exit のセンチネルを残す。
func (c *respCollector) markGone(err error) {
	c.mu.Lock()
	if c.gone == nil {
		if err == nil {
			err = errCleanExit
		}
		c.gone = err
	}
	c.mu.Unlock()
}

// snapshot: 現在までに収集した行のコピーを返す（呼び出し元での自由な変更可）。
func (c *respCollector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.lines))
	copy(out, c.lines)
	return out
}

// waitComplete はプロンプト末尾検出（案C'）でコマンド完了を待つ。
// 戻り値の lines は常に部分結果を含む（timeout/プロセス死亡時でも収集済の行は返す）。
//
// 完了条件:
//   - pending tail の末尾バイトが '>' で、 SettleConfirm 以上変化が無い
//
// 中断条件:
//   - ctx.Err()        → ErrCanceled
//   - c.gone != nil    → ErrProcessGone
//   - MaxTimeout 超過  → ErrTimeout
func (c *respCollector) waitComplete(ctx context.Context, cfg execConfig) ([]string, error) {
	deadline := time.Now().Add(cfg.MaxTimeout)
	for {
		// 1. ctx 中断
		if err := ctx.Err(); err != nil {
			return c.snapshot(), ErrCanceled
		}
		// 2. プロセス死亡 + pending tail スナップショット (ロック内で値コピー)
		c.mu.Lock()
		gone := c.gone
		// pendingTail の slice header をコピーするとロック外で更新されうるため、
		// 必要な値 (末尾バイトと長さ) だけをロック内で抽出する。
		pendingLen := len(c.pendingTail)
		var pendingLast byte
		if pendingLen > 0 {
			pendingLast = c.pendingTail[pendingLen-1]
		}
		lastChange := c.lastChange
		c.mu.Unlock()
		if gone != nil {
			return c.snapshot(), fmt.Errorf("%w: %v", ErrProcessGone, gone)
		}
		// 3. 全体タイムアウト
		if time.Now().After(deadline) {
			return c.snapshot(), ErrTimeout
		}
		// 4. プロンプト末尾検出 + 安定窓
		if pendingLen > 0 && pendingLast == '>' {
			stable := time.Since(lastChange)
			if stable >= cfg.SettleConfirm {
				return c.snapshot(), nil
			}
			// 安定窓まで待つ（変化が無ければ次ループで確定）
			sleepDur := cfg.SettleConfirm - stable
			if sleepDur > cfg.ReadChunkTimeout {
				sleepDur = cfg.ReadChunkTimeout
			}
			time.Sleep(sleepDur)
			continue
		}
		// 5. それ以外はポーリング間隔だけ待って再評価
		time.Sleep(cfg.ReadChunkTimeout)
	}
}
