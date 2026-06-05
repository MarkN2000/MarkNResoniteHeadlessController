package steam

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMain は GO_FAKE_DD=1 のとき本テストバイナリを「偽 DepotDownloader」として振る舞わせる
// （os/exec の自己再実行パターン）。runner_test は os.Args[0] を DDPath に渡して実行する。
func TestMain(m *testing.M) {
	if os.Getenv("GO_FAKE_DD") == "1" {
		fakeDDMain()
		return
	}
	os.Exit(m.Run())
}

// fakeDDMain は偽 DepotDownloader。GO_FAKE_DD_MODE で挙動を切り替える。
//   - success: app branch → パスワードプロンプト(改行なし) → stdin 読取 → 進捗2行 → Total → exit 0
//   - 2fa:     パスワード受領後に 2FA プロンプト(改行なし) を出して入力待ちで block（runner が kill）
//   - fail:    エラー行を出して exit 1
//
// プロンプトは実 DD と同じく改行なし Write で出す（runner の tail 検出を実地で検証するため）。
func fakeDDMain() {
	switch os.Getenv("GO_FAKE_DD_MODE") {
	case "fail":
		fmt.Fprintln(os.Stdout, "[error] App 2519830 not available")
		os.Exit(1)
	case "2fa":
		fmt.Fprint(os.Stdout, `Enter account password for "user": `)
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Fprint(os.Stdout, "Please enter your 2 factor auth code from your authenticator app: ")
		select {} // 入力待ちで block。runner が 2FA を検出して kill する。
	case "hang":
		// パスワード受領後に1行だけ進捗を出して以降は沈黙＝無進捗で固まる。
		// manager の cancel / stall ウォッチドッグ検証用（kill されるまで block）。
		fmt.Fprint(os.Stdout, `Enter account password for "user": `)
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Fprintln(os.Stdout, " 10.00% /Resonite/a")
		select {}
	default: // success
		fmt.Fprintln(os.Stdout, "Using app branch: 'headless'.")
		fmt.Fprint(os.Stdout, `Enter account password for "user": `)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		got := strings.TrimRight(line, "\r\n")
		if got != os.Getenv("GO_FAKE_DD_PASSWORD") {
			fmt.Fprintln(os.Stdout, "[error] wrong password")
			os.Exit(3)
		}
		fmt.Fprintln(os.Stdout, " 50.00% /Resonite/a")
		fmt.Fprintln(os.Stdout, " 100.00% /Resonite/b")
		fmt.Fprintln(os.Stdout, "Total downloaded: 100 bytes (100 bytes uncompressed) from 1 depots")
		os.Exit(0)
	}
}

func fakeRunParams() RunParams {
	return RunParams{
		DDPath:     os.Args[0],
		InstallDir: os.TempDir(),
		Username:   "user",
		Password:   "secret",
		BranchCode: "betacode",
	}
}

func TestRunner_Success(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "success")
	t.Setenv("GO_FAKE_DD_PASSWORD", "secret")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var events []Event
	err := NewRunner().Run(ctx, fakeRunParams(), func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var maxPct float64
	var sawTotal bool
	for _, e := range events {
		if e.Kind == "progress" && e.Percent > maxPct {
			maxPct = e.Percent
		}
		if e.Kind == "milestone" && e.Text == "Total downloaded" {
			sawTotal = true
		}
	}
	if maxPct != 100 {
		t.Errorf("最大 percent=%v want 100（進捗イベント未検出の可能性）", maxPct)
	}
	if !sawTotal {
		t.Error("Total downloaded マイルストーン未検出")
	}
}

func TestRunner_WrongPasswordFails(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "success")
	t.Setenv("GO_FAKE_DD_PASSWORD", "secret")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p := fakeRunParams()
	p.Password = "wrong" // 偽DDは不一致なら exit 3
	err := NewRunner().Run(ctx, p, nil)
	if err == nil {
		t.Fatal("誤パスワードは失敗するべき（＝パスワードが stdin で渡っている証左）")
	}
	if errors.Is(err, ErrTwoFactorRequired) {
		t.Fatal("2FA エラーではないはず")
	}
}

func TestRunner_TwoFactorRequired(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "2fa")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := NewRunner().Run(ctx, fakeRunParams(), nil)
	if !errors.Is(err, ErrTwoFactorRequired) {
		t.Fatalf("2FA 要求は ErrTwoFactorRequired を返すべき: %v", err)
	}
}

func TestRunner_Failure(t *testing.T) {
	t.Setenv("GO_FAKE_DD", "1")
	t.Setenv("GO_FAKE_DD_MODE", "fail")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := NewRunner().Run(ctx, fakeRunParams(), nil)
	if err == nil {
		t.Fatal("exit 1 は失敗を返すべき")
	}
	if errors.Is(err, ErrTwoFactorRequired) {
		t.Fatal("2FA エラーではないはず")
	}
}
