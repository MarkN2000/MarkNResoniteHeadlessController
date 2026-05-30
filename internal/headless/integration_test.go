package headless

// 統合テスト: poc/fakehl を実プロセスとして起動し、Driver / WorldsService を
// end-to-end で検証する。
//   - TestMain で go build により fakehl 一時バイナリを用意
//   - 各テストは独自の Driver を作って fakehl を spawn → 構造化APIを叩く
//   - cleanup で shutdown 送信 → プロセス終了を待つ

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var fakehlPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "mrhc-fakehl-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir tmp: %v\n", err)
		os.Exit(1)
	}
	fakehlPath = filepath.Join(tmp, "fakehl"+exeSuffix())
	build := exec.Command("go", "build", "-o", fakehlPath, "../../poc/fakehl")
	if out, err := build.CombinedOutput(); err != nil {
		os.RemoveAll(tmp)
		fmt.Fprintf(os.Stderr, "build fakehl: %v\n%s\n", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// newFakeDriver は fakehl を spawn して ready 状態の Driver を返す。
// cleanup は t.Cleanup で登録（shutdown送信 → 待機 → fallback force-kill）。
func newFakeDriver(t *testing.T) *Driver {
	t.Helper()
	d := NewDriver(nil) // UTF-8 passthrough（fakehl は ASCII / UTF-8 出力）
	if err := d.Start(fakehlPath, "", ""); err != nil {
		t.Fatalf("start fakehl: %v", err)
	}
	t.Cleanup(func() {
		if d.Status().State == StateStopped {
			return
		}
		_ = d.SendCommand("shutdown")
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if d.Status().State == StateStopped {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		// fallback: 直接 kill（hang 中で shutdown が届かないケース等）
		killFakehl(d)
	})

	// readiness 待ち（fakehl は "World running..." を即時に出す）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if d.Status().Ready {
			return d
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("fakehl never became ready (state=%s)", d.Status().State)
	return nil
}

// killFakehl は Driver の child process を直接強制終了する（テスト専用）。
func killFakehl(d *Driver) {
	d.mu.Lock()
	var p *os.Process
	if d.cmd != nil {
		p = d.cmd.Process
	}
	d.mu.Unlock()
	if p != nil {
		_ = p.Kill()
	}
}

// --- 基本 Exec ---

func TestIntegration_Exec_Worlds(t *testing.T) {
	d := newFakeDriver(t)
	lines, err := d.Exec(context.Background(), "worlds")
	if err != nil {
		t.Fatalf("Exec worlds: %v", err)
	}
	worlds := ParseWorlds(lines)
	if len(worlds) != 2 {
		t.Fatalf("expected 2 worlds, got %d: lines=%v", len(worlds), lines)
	}
	if worlds[0].Name != "Fake World 0" || worlds[1].Name != "Fake World 1" {
		t.Fatalf("unexpected names: %+v", worlds)
	}
	if worlds[0].AccessLevel != "Private" || worlds[0].MaxUsers != 4 {
		t.Fatalf("unexpected world 0: %+v", worlds[0])
	}
}

// TestIntegration_WarmupRunsBeforeReady は ready になる前に warmup の捨てコマンド
// (worlds) が「最初のコンソール入力」として送られていることを確認する。これにより
// ユーザーの最初の実コマンドは常に2番目の入力となり、起動直後の最初の1入力が
// 無視/Unknown 化する実機癖を身代わりに吸収できる。
func TestIntegration_WarmupRunsBeforeReady(t *testing.T) {
	d := newFakeDriver(t) // Ready==true まで待つ（＝warmup 確認後）

	d.mu.Lock()
	hist := append([]LogLine(nil), d.history...)
	d.mu.Unlock()

	want := "> " + warmupCommand
	found := false
	for _, ln := range hist {
		if ln.Kind == "cmd" && ln.Text == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("warmup の %q が起動ログに見当たらない: %+v", want, hist)
	}
}

func TestIntegration_Exec_Status(t *testing.T) {
	d := newFakeDriver(t)
	// 原子的グループで focus → status を実行（実際は分けても通る想定だが atomic がより安全）
	var got WorldStatus
	err := d.ExecGroup(context.Background(), func(tx Tx) error {
		if _, err := tx.Exec("focus 1"); err != nil {
			return err
		}
		lines, err := tx.Exec("status")
		if err != nil {
			return err
		}
		got = ParseStatus(lines)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecGroup: %v", err)
	}
	if got.Name != "Fake World 1" || got.MaxUsers != 4 || got.ResoniteLink != "off" {
		t.Fatalf("unexpected status: %+v", got)
	}
	if got.AccessLevel != "Private" {
		t.Fatalf("AccessLevel: %q", got.AccessLevel)
	}
}

func TestIntegration_Exec_SilentCommand(t *testing.T) {
	d := newFakeDriver(t)
	lines, err := d.Exec(context.Background(), `name "Renamed"`)
	if err != nil {
		t.Fatalf("Exec name: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("silent cmd should return 0 lines, got %v", lines)
	}
	// 続けて worlds: 改名が反映されているか
	lines2, err := d.Exec(context.Background(), "worlds")
	if err != nil {
		t.Fatalf("Exec worlds: %v", err)
	}
	worlds := ParseWorlds(lines2)
	if worlds[0].Name != "Renamed" {
		t.Fatalf("rename not applied: %+v", worlds)
	}
}

func TestIntegration_Exec_UnknownCommand(t *testing.T) {
	d := newFakeDriver(t)
	lines, err := d.Exec(context.Background(), "this_is_not_a_command")
	if err != nil {
		t.Fatalf("Exec unknown: %v", err)
	}
	// Driver が完了時の実プロンプトを剥がすので lines は綺麗（prompt prefix 無し）。
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "Unknown command" {
		t.Fatalf("expected 'Unknown command', got %q", lines[0])
	}
}

// --- 直列化・原子的グループ ---

func TestIntegration_ExecMu_Serializes(t *testing.T) {
	// 2 並行 Exec が直列に処理されることを確認。
	// fakehl は単一 goroutine で逐次処理 → どちらが先かは送信順だが、
	// 両方とも成功して結果が「混ざらない」ことを期待。
	d := newFakeDriver(t)
	var wg sync.WaitGroup
	var result1, result2 []World
	var err1, err2 error
	wg.Add(2)
	go func() {
		defer wg.Done()
		lines, e := d.Exec(context.Background(), "worlds")
		err1 = e
		result1 = ParseWorlds(lines)
	}()
	go func() {
		defer wg.Done()
		lines, e := d.Exec(context.Background(), "worlds")
		err2 = e
		result2 = ParseWorlds(lines)
	}()
	wg.Wait()
	if err1 != nil || err2 != nil {
		t.Fatalf("err1=%v err2=%v", err1, err2)
	}
	if len(result1) != 2 || len(result2) != 2 {
		t.Fatalf("expected each result to have 2 worlds, got %d/%d", len(result1), len(result2))
	}
	// 名前が正しく取れていれば「混ざってない」証拠
	if result1[0].Name != "Fake World 0" || result2[0].Name != "Fake World 0" {
		t.Fatalf("crosstalk suspected: %+v / %+v", result1, result2)
	}
}

func TestIntegration_ExecGroup_BlocksOthers(t *testing.T) {
	// ExecGroup 実行中に別 goroutine からの Exec が「待たされる」ことを確認。
	d := newFakeDriver(t)
	groupDone := make(chan struct{})
	var execStartTime atomic.Int64
	var groupEndTime atomic.Int64

	go func() {
		_ = d.ExecGroup(context.Background(), func(tx Tx) error {
			// グループ内で 200ms 滞在
			if _, err := tx.Exec("worlds"); err != nil {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			if _, err := tx.Exec("worlds"); err != nil {
				return err
			}
			groupEndTime.Store(time.Now().UnixNano())
			return nil
		})
		close(groupDone)
	}()

	// 並行に Exec を投げる。これは ExecGroup が手放すまで待つはず。
	time.Sleep(50 * time.Millisecond) // ExecGroup が先に execMu を取る
	execStartTime.Store(time.Now().UnixNano())
	_, err := d.Exec(context.Background(), "worlds")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	execEnd := time.Now().UnixNano()
	<-groupDone

	gEnd := groupEndTime.Load()
	if gEnd == 0 {
		t.Fatalf("group did not finish")
	}
	// Exec の終了は ExecGroup の終了 (gEnd) より後（または同等）であるはず
	if execEnd < gEnd {
		t.Fatalf("Exec が ExecGroup 終了より前に完了。直列化されていない可能性: execEnd=%d gEnd=%d", execEnd, gEnd)
	}
}

// --- WorldsService ---

func TestIntegration_WorldsService_List(t *testing.T) {
	d := newFakeDriver(t)
	svc := NewWorldsService(d)
	worlds, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(worlds) != 2 {
		t.Fatalf("expected 2 worlds, got %d", len(worlds))
	}
}

func TestIntegration_WorldsService_ForEach(t *testing.T) {
	d := newFakeDriver(t)
	svc := NewWorldsService(d)

	visited := make([]string, 0, 2)
	err := svc.ForEach(context.Background(), func(w World, s Scope) error {
		// scope.Exec で status を取得 → 名前一致を確認
		lines, err := s.Exec("status")
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		st := ParseStatus(lines)
		if st.Name != w.Name {
			return fmt.Errorf("focus 不整合: world=%s status.Name=%s", w.Name, st.Name)
		}
		visited = append(visited, w.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("ForEach: %v", err)
	}
	if len(visited) != 2 || visited[0] != "Fake World 0" || visited[1] != "Fake World 1" {
		t.Fatalf("unexpected visited: %v", visited)
	}
}

// --- タイムアウト・プロセス死亡 ---

func TestIntegration_Exec_Timeout(t *testing.T) {
	d := newFakeDriver(t)
	_, err := d.Exec(context.Background(), "hang", WithTimeout(150*time.Millisecond))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestIntegration_Exec_ProcessGone(t *testing.T) {
	d := newFakeDriver(t)
	// hang を投げて Exec をブロック → 並行で fakehl を直接 kill
	resultCh := make(chan error, 1)
	go func() {
		_, err := d.Exec(context.Background(), "hang", WithTimeout(5*time.Second))
		resultCh <- err
	}()
	time.Sleep(150 * time.Millisecond) // Exec が hang 待ちに入るのを待つ
	killFakehl(d)

	select {
	case err := <-resultCh:
		if !errors.Is(err, ErrProcessGone) {
			t.Fatalf("expected ErrProcessGone, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("Exec did not return after kill")
	}
}

func TestIntegration_Exec_NotReadyAfterStop(t *testing.T) {
	// 既に停止した Driver に Exec を投げたら ErrNotReady
	d := newFakeDriver(t)
	_ = d.SendCommand("shutdown")
	// 終了を待つ
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if d.Status().State == StateStopped {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if d.Status().State != StateStopped {
		t.Fatalf("fakehl did not stop in time")
	}
	_, err := d.Exec(context.Background(), "worlds")
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ErrNotReady, got %v", err)
	}
}
