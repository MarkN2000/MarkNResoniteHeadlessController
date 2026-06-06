// deps_prompt.go は不足依存（freetype2 / .NET 10）の導入をウィザード末尾で
// 対話提案する（R-C 経路①）。検出は platform、対話は setup の責務。
// 詳細仕様: docs/design/deps-onboarding.md §3.2・docs/design/cli-onboarding.md S4
package setup

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

// OfferDepInstall は不足依存の導入を対話で提案する。
// tty なら issue ごとに導入コマンドを提示し [Y/n]（空入力=Y・不正は再入力）で同意実行、
// 実行後に recheck で確認結果を表示する。非 tty は提示のみ。
// 拒否・実行失敗・コマンド不明（distro 不明）でも必ず続行する（ブロックしない。
// 以後は毎起動のログ案内＝経路②が思い出させる）。EOF は以降の対話が不可能なので
// 残りの issue ごと打ち切る（直後のプロンプト＝S5/S6 が中断を扱う）。
// run / recheck は seam: run が nil なら bash -c の実実装。recheck は nil なら確認を省く。
func OfferDepInstall(issues []platform.DepIssue, in *bufio.Reader, out io.Writer, lang i18n.Lang, tty bool,
	run func(cmd string) error, recheck func(kind string) bool) {
	if run == nil {
		run = runDepCmd
	}
	for _, issue := range issues {
		fmt.Fprintln(out)
		fmt.Fprintln(out, i18n.T(lang, "deps.headline."+issue.Kind))
		if len(issue.Commands) == 0 {
			fmt.Fprintln(out, "  "+issue.GuideText(lang)) // distro 不明＝手動導入の案内のみ
			continue
		}
		fmt.Fprintln(out, depCmdLabel(lang, issue))
		for _, c := range issue.Commands {
			fmt.Fprintln(out, "    "+c)
		}
		if !tty {
			continue // 非 tty（パイプ実行）は提示のみ
		}
		yes, err := promptYN(in, out, lang, i18n.T(lang, "deps.runNow"))
		if err != nil {
			return // EOF: 入力が尽きた（残りはスキップ・後続プロンプトが中断を扱う）
		}
		if !yes {
			fmt.Fprintln(out, i18n.T(lang, "deps.skipped"))
			continue
		}
		if err := runAll(run, issue.Commands); err != nil {
			fmt.Fprintln(out, depFailText(lang, issue, err))
			fmt.Fprintln(out, i18n.T(lang, "deps.runManually"))
			continue
		}
		if recheck != nil {
			if recheck(issue.Kind) {
				fmt.Fprintln(out, i18n.T(lang, "deps.verified"))
			} else {
				fmt.Fprintln(out, i18n.T(lang, "deps.notVerified"))
			}
		}
	}
}

// depCmdLabel は導入コマンドの前置きラベル（dotnet10 は sudo 不要の補足つき）。
func depCmdLabel(lang i18n.Lang, issue platform.DepIssue) string {
	if issue.Kind == "dotnet10" {
		return i18n.T(lang, "deps.cmdLabel.dotnet10")
	}
	return i18n.T(lang, "deps.cmdLabel")
}

// depFailText は実行失敗の文言（dotnet-install.sh は curl と bash が前提）。
func depFailText(lang i18n.Lang, issue platform.DepIssue, err error) string {
	if issue.Kind == "dotnet10" {
		return i18n.T(lang, "deps.runFailed.dotnet10", err)
	}
	return i18n.T(lang, "deps.runFailed", err)
}

func runAll(run func(string) error, cmds []string) error {
	for _, c := range cmds {
		if err := run(c); err != nil {
			return err
		}
	}
	return nil
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
