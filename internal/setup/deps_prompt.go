// deps_prompt.go は不足依存（freetype2 / .NET 10）の導入をウィザード末尾で
// 対話提案する（R-C 経路①）。検出は platform、対話は setup の責務。
// 詳細仕様: docs/design/deps-onboarding.md §3.2
package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

// OfferDepInstall は不足依存の導入を対話で提案する。
// tty なら issue ごとに導入コマンドを提示し [Y/n]（空入力=Y）で同意実行、
// 実行後に recheck で確認結果を表示する。非 tty は提示のみ。
// 拒否・実行失敗・コマンド不明（distro 不明）でも必ず続行する（ブロックしない。
// 以後は毎起動のログ案内＝経路②が思い出させる）。
// run / recheck は seam: run が nil なら bash -c の実実装。recheck は nil なら確認を省く。
func OfferDepInstall(issues []platform.DepIssue, in *bufio.Reader, tty bool,
	run func(cmd string) error, recheck func(kind string) bool) {
	if run == nil {
		run = runDepCmd
	}
	for _, issue := range issues {
		fmt.Println()
		fmt.Println(depHeadline(issue))
		if len(issue.Commands) == 0 {
			fmt.Println("  " + issue.Fallback)
			continue
		}
		fmt.Println("  " + depCmdLabel(issue))
		for _, c := range issue.Commands {
			fmt.Println("    " + c)
		}
		if !tty {
			continue // 非 tty（パイプ実行）は提示のみ
		}
		if !promptYes(in, "  今すぐ実行しますか? [Y/n]: ") {
			fmt.Println("  スキップしました（上のコマンドは後で手動実行できます）。")
			continue
		}
		if err := runAll(run, issue.Commands); err != nil {
			fmt.Printf("  実行に失敗しました%s: %v\n", depFailNote(issue), err)
			fmt.Println("  上のコマンドを手動で実行してください。")
			continue
		}
		if recheck != nil {
			if recheck(issue.Kind) {
				fmt.Println("  ✓ 導入を確認しました。")
			} else {
				fmt.Println("  まだ確認できません（続行します）。")
			}
		}
	}
}

// depHeadline は issue の見出し行（kind 別の文言）。
func depHeadline(issue platform.DepIssue) string {
	if issue.Kind == "dotnet10" {
		return "⚠ ARM Linux では .NET 10 ランタイムが必要ですが、見つかりません。"
	}
	return fmt.Sprintf("⚠ Resonite の動作に必要な %s が見つかりません。", issue.Kind)
}

// depCmdLabel は導入コマンドの前置きラベル。
func depCmdLabel(issue platform.DepIssue) string {
	if issue.Kind == "dotnet10" {
		return "導入コマンド（sudo 不要・~/.dotnet に入ります）:"
	}
	return "導入コマンド:"
}

// depFailNote は実行失敗時の補足（dotnet-install.sh は curl と bash が前提）。
func depFailNote(issue platform.DepIssue) string {
	if issue.Kind == "dotnet10" {
		return "（curl と bash が必要です）"
	}
	return ""
}

func runAll(run func(string) error, cmds []string) error {
	for _, c := range cmds {
		if err := run(c); err != nil {
			return err
		}
	}
	return nil
}

// promptYes は [Y/n] を尋ねる（空入力=Y・EOF/読み取りエラーは N 扱い）。
func promptYes(in *bufio.Reader, prompt string) bool {
	fmt.Print(prompt)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true
	default:
		return false
	}
}

// runDepCmd は導入コマンドを bash 経由で実行する。stdio を端末に直結し、
// sudo のパスワード入力やパイプ（curl | bash）がそのまま機能するようにする。
// 注意: 呼び出し側 bufio.Reader の先読みバッファ内のバイトは子プロセスへ届かない
// （対話入力では行単位で消費されるため実害なし）。
func runDepCmd(cmd string) error {
	c := exec.Command("bash", "-c", cmd)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
