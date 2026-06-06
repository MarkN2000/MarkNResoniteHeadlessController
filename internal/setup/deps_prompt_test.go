package setup

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

// recorder は run/recheck の呼び出しを記録する偽実装。
type recorder struct {
	ran       []string
	rechecked []string
	runErr    error
	resolved  bool
}

func (r *recorder) run(cmd string) error {
	r.ran = append(r.ran, cmd)
	return r.runErr
}

func (r *recorder) recheck(kind string) bool {
	r.rechecked = append(r.rechecked, kind)
	return r.resolved
}

func freetypeIssue() platform.DepIssue {
	return platform.DepIssue{
		Kind:     "freetype2",
		Commands: []string{"sudo pacman -S freetype2"},
	}
}

// offer は OfferDepInstall を出力キャプチャ付きで呼ぶ薄いラッパ。
func offer(issues []platform.DepIssue, input string, tty bool, rec *recorder) *bytes.Buffer {
	out := &bytes.Buffer{}
	in := bufio.NewReader(strings.NewReader(input))
	OfferDepInstall(issues, in, out, i18n.Ja, tty, rec.run, rec.recheck)
	return out
}

func TestOfferDepInstall(t *testing.T) {
	t.Run("空入力（既定=Y）で実行し recheck する", func(t *testing.T) {
		rec := &recorder{resolved: true}
		out := offer([]platform.DepIssue{freetypeIssue()}, "\n", true, rec)
		if len(rec.ran) != 1 || rec.ran[0] != "sudo pacman -S freetype2" {
			t.Errorf("run 呼び出し = %v, want [sudo pacman -S freetype2]", rec.ran)
		}
		if len(rec.rechecked) != 1 || rec.rechecked[0] != "freetype2" {
			t.Errorf("recheck 呼び出し = %v, want [freetype2]", rec.rechecked)
		}
		if !strings.Contains(out.String(), "✓ 導入を確認しました。") {
			t.Errorf("確認の案内が出ていない:\n%s", out.String())
		}
	})
	t.Run("y 入力で実行する", func(t *testing.T) {
		rec := &recorder{}
		offer([]platform.DepIssue{freetypeIssue()}, "y\n", true, rec)
		if len(rec.ran) != 1 {
			t.Errorf("run が呼ばれるべき: %v", rec.ran)
		}
	})
	t.Run("n 入力でスキップ（run も recheck も呼ばれない）", func(t *testing.T) {
		rec := &recorder{}
		out := offer([]platform.DepIssue{freetypeIssue()}, "n\n", true, rec)
		if len(rec.ran) != 0 || len(rec.rechecked) != 0 {
			t.Errorf("スキップなのに呼ばれた: run=%v recheck=%v", rec.ran, rec.rechecked)
		}
		if !strings.Contains(out.String(), "スキップしました") {
			t.Errorf("スキップの案内が出ていない:\n%s", out.String())
		}
	})
	t.Run("不正入力は再入力（黙って n に倒さない）", func(t *testing.T) {
		rec := &recorder{}
		out := offer([]platform.DepIssue{freetypeIssue()}, "x\ny\n", true, rec)
		if len(rec.ran) != 1 {
			t.Errorf("再入力後の y で実行されるべき: %v", rec.ran)
		}
		if !strings.Contains(out.String(), "y か n で答えてください。") {
			t.Errorf("再入力の案内が出ていない:\n%s", out.String())
		}
	})
	t.Run("EOF（入力が尽きた）は実行せず打ち切る", func(t *testing.T) {
		rec := &recorder{}
		offer([]platform.DepIssue{freetypeIssue()}, "", true, rec)
		if len(rec.ran) != 0 {
			t.Errorf("EOF で実行された: %v", rec.ran)
		}
	})
	t.Run("EOF は残りの issue も打ち切る（無限ループしない）", func(t *testing.T) {
		rec := &recorder{}
		two := []platform.DepIssue{freetypeIssue(), {Kind: "dotnet10", Commands: []string{"curl ... | bash"}}}
		out := offer(two, "", true, rec)
		if len(rec.ran) != 0 {
			t.Errorf("EOF で実行された: %v", rec.ran)
		}
		// 1 件目の提示はされるが 2 件目のヘッドラインは出ない（打ち切り）
		if strings.Contains(out.String(), ".NET 10") {
			t.Errorf("EOF 後に次の issue が提示された:\n%s", out.String())
		}
	})
	t.Run("非 tty は提示のみ（対話も実行もしない・入力を消費しない）", func(t *testing.T) {
		rec := &recorder{}
		in := bufio.NewReader(strings.NewReader("y\n"))
		out := &bytes.Buffer{}
		OfferDepInstall([]platform.DepIssue{freetypeIssue()}, in, out, i18n.Ja, false, rec.run, rec.recheck)
		if len(rec.ran) != 0 {
			t.Errorf("非 tty で実行された: %v", rec.ran)
		}
		if _, err := in.ReadString('\n'); err != nil {
			t.Errorf("非 tty は入力を消費すべきでない: %v", err)
		}
	})
	t.Run("実行失敗なら recheck しないで続行する", func(t *testing.T) {
		rec := &recorder{runErr: errors.New("boom")}
		two := []platform.DepIssue{freetypeIssue(), {
			Kind:     "dotnet10",
			Commands: []string{"curl ... | bash"},
		}}
		out := offer(two, "y\ny\n", true, rec)
		if len(rec.ran) != 2 {
			t.Errorf("失敗しても次の issue へ進むべき: ran=%v", rec.ran)
		}
		if len(rec.rechecked) != 0 {
			t.Errorf("失敗時に recheck された: %v", rec.rechecked)
		}
		// dotnet10 の失敗は curl/bash 前提の補足つき
		if !strings.Contains(out.String(), "curl と bash が必要です") {
			t.Errorf("dotnet10 失敗の補足が出ていない:\n%s", out.String())
		}
	})
	t.Run("Commands 空（distro 不明）は案内だけで対話しない", func(t *testing.T) {
		rec := &recorder{}
		issue := platform.DepIssue{Kind: "freetype2"} // Commands 空＝fallback
		in := bufio.NewReader(strings.NewReader("y\n"))
		out := &bytes.Buffer{}
		OfferDepInstall([]platform.DepIssue{issue}, in, out, i18n.Ja, true, rec.run, rec.recheck)
		if len(rec.ran) != 0 {
			t.Errorf("コマンド不明なのに実行された: %v", rec.ran)
		}
		if _, err := in.ReadString('\n'); err != nil {
			t.Errorf("対話せず入力を消費すべきでない: %v", err)
		}
		if !strings.Contains(out.String(), "パッケージマネージャ") {
			t.Errorf("手動導入の案内が出ていない:\n%s", out.String())
		}
	})
	t.Run("英語カタログでも提示できる", func(t *testing.T) {
		rec := &recorder{}
		out := &bytes.Buffer{}
		in := bufio.NewReader(strings.NewReader("n\n"))
		OfferDepInstall([]platform.DepIssue{freetypeIssue()}, in, out, i18n.En, true, rec.run, rec.recheck)
		if !strings.Contains(out.String(), "was not found") || !strings.Contains(out.String(), "Skipped") {
			t.Errorf("英語文言が出ていない:\n%s", out.String())
		}
	})
	t.Run("issue ゼロは何もしない", func(t *testing.T) {
		rec := &recorder{}
		out := offer(nil, "", true, rec)
		if len(rec.ran) != 0 || len(rec.rechecked) != 0 || out.Len() != 0 {
			t.Errorf("issue ゼロで何か出力・実行された: %q", out.String())
		}
	})
}
