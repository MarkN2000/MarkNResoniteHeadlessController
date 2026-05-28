package headless

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// テストの方針:
//   - respCollector を「fake な readPipe」スレッドから appendLine/updateTail で
//     更新し、waitComplete が期待通りに完了/timeout/ErrProcessGone/ErrCanceled を返すか検証。
//   - 実プロセス/実 stdout は不要（純粋なロジックテスト）。

// 短いタイムアウト/安定窓でテストを高速化
func fastConfig() execConfig {
	return execConfig{
		MaxTimeout:       300 * time.Millisecond,
		SettleConfirm:    20 * time.Millisecond,
		ReadChunkTimeout: 5 * time.Millisecond,
	}
}

func TestWaitComplete_BasicPromptDetection(t *testing.T) {
	c := newRespCollector()
	cfg := fastConfig()

	// fake readPipe goroutine: 行を3つ流して、最後にプロンプトを tail に出す
	go func() {
		time.Sleep(10 * time.Millisecond)
		c.appendLine("[0] World A           Users: 1\tPresent: 0\tAccessLevel: LAN\tMaxUsers: 16")
		time.Sleep(10 * time.Millisecond)
		c.appendLine("[1] World B           Users: 0\tPresent: 0\tAccessLevel: Anyone\tMaxUsers: 8")
		time.Sleep(10 * time.Millisecond)
		c.updateTail([]byte("World B>"))
	}()

	got, err := c.waitComplete(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected nil err, got %v (lines=%v)", err, got)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
}

func TestWaitComplete_SilentCommand(t *testing.T) {
	// silent コマンド: 行は来ず、プロンプトだけが pending に乗る
	c := newRespCollector()
	cfg := fastConfig()
	go func() {
		time.Sleep(5 * time.Millisecond)
		c.updateTail([]byte("Renamed>"))
	}()
	got, err := c.waitComplete(context.Background(), cfg)
	if err != nil {
		t.Fatalf("silent cmd should succeed, got err=%v", err)
	}
	if len(got) != 0 {
		t.Fatalf("silent cmd should return 0 lines, got %v", got)
	}
}

func TestWaitComplete_Timeout(t *testing.T) {
	// プロンプトが来ないまま MaxTimeout 超過
	c := newRespCollector()
	cfg := fastConfig()
	go func() {
		c.appendLine("partial response")
		// プロンプトを出さない → タイムアウトするはず
	}()
	got, err := c.waitComplete(context.Background(), cfg)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	// 部分結果は返ること
	if !reflect.DeepEqual(got, []string{"partial response"}) {
		t.Fatalf("expected partial result, got %v", got)
	}
}

func TestWaitComplete_ProcessGone(t *testing.T) {
	c := newRespCollector()
	cfg := fastConfig()
	go func() {
		c.appendLine("some output")
		time.Sleep(10 * time.Millisecond)
		c.markGone(errors.New("process died"))
	}()
	got, err := c.waitComplete(context.Background(), cfg)
	if !errors.Is(err, ErrProcessGone) {
		t.Fatalf("expected ErrProcessGone, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected partial result, got %v", got)
	}
}

func TestWaitComplete_ProcessGoneWrapsUnderlying(t *testing.T) {
	// markGone(err) で渡したエラーが返値のメッセージに含まれること。
	// errors.Is(err, ErrProcessGone) も引き続き true。
	c := newRespCollector()
	cfg := fastConfig()
	underlying := errors.New("exit status 137")
	go func() {
		time.Sleep(5 * time.Millisecond)
		c.markGone(underlying)
	}()
	_, err := c.waitComplete(context.Background(), cfg)
	if !errors.Is(err, ErrProcessGone) {
		t.Fatalf("errors.Is(ErrProcessGone) failed: %v", err)
	}
	if !strings.Contains(err.Error(), "exit status 137") {
		t.Fatalf("元エラーが err.Error() に含まれるべき: got=%q", err.Error())
	}
}

func TestWaitComplete_ProcessGoneCleanExit(t *testing.T) {
	// markGone(nil) でも「プロセスが消えた」状態は同じ → ErrProcessGone を返す
	c := newRespCollector()
	cfg := fastConfig()
	go func() {
		time.Sleep(5 * time.Millisecond)
		c.markGone(nil)
	}()
	_, err := c.waitComplete(context.Background(), cfg)
	if !errors.Is(err, ErrProcessGone) {
		t.Fatalf("clean exit でも ErrProcessGone 判定されるべき: %v", err)
	}
}

func TestWaitComplete_ContextCancel(t *testing.T) {
	c := newRespCollector()
	cfg := fastConfig()
	cfg.MaxTimeout = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(15 * time.Millisecond)
		cancel()
	}()
	_, err := c.waitComplete(ctx, cfg)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("expected ErrCanceled, got %v", err)
	}
}

func TestWaitComplete_AmbientWithCloseAngle(t *testing.T) {
	// ambient 行末に '>' を含んでも、改行で終わっているので tail には残らず誤検出しない。
	// その後の本物のプロンプトでのみ完了する。
	c := newRespCollector()
	cfg := fastConfig()
	go func() {
		time.Sleep(5 * time.Millisecond)
		c.appendLine("[ambient] InitState: Initializing>") // 末尾 > だが行として確定（改行で区切られた想定）
		// この時点で pending tail は空 → 完了検出しない
		time.Sleep(20 * time.Millisecond)
		c.updateTail([]byte("World>")) // 本物のプロンプト
	}()
	got, err := c.waitComplete(context.Background(), cfg)
	if err != nil {
		t.Fatalf("should succeed eventually, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 line, got %d", len(got))
	}
}

func TestWaitComplete_StableWindow(t *testing.T) {
	// pending tail が '>' になっても、その後すぐ変化したら確定しない（再度安定を待つ）。
	c := newRespCollector()
	cfg := execConfig{
		MaxTimeout:       400 * time.Millisecond,
		SettleConfirm:    40 * time.Millisecond,
		ReadChunkTimeout: 5 * time.Millisecond,
	}
	go func() {
		time.Sleep(5 * time.Millisecond)
		c.updateTail([]byte("World>"))
		time.Sleep(15 * time.Millisecond)            // < SettleConfirm
		c.appendLine("late ambient line that arrives during settle")
		c.updateTail([]byte("World>")) // 再度プロンプト → 安定窓やり直し
	}()
	start := time.Now()
	got, err := c.waitComplete(context.Background(), cfg)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("should succeed, got %v", err)
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("確定が早すぎる (安定窓を待っていない可能性): %v", elapsed)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 line, got %d", len(got))
	}
}
