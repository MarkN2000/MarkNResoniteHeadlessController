package setup

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

// quietStdout はテスト中の fmt.Print 出力を捨てる（対話文言はここでは検証対象外）。
func quietStdout(t *testing.T) {
	t.Helper()
	old := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("devnull open 失敗: %v", err)
	}
	os.Stdout = devnull
	t.Cleanup(func() {
		os.Stdout = old
		devnull.Close()
	})
}

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
		Title:    "freetype2（Resonite のネイティブ依存）",
		Commands: []string{"sudo pacman -S freetype2"},
		Sudo:     true,
	}
}

func TestOfferDepInstall(t *testing.T) {
	quietStdout(t)

	t.Run("空入力（既定=Y）で実行し recheck する", func(t *testing.T) {
		rec := &recorder{resolved: true}
		in := bufio.NewReader(strings.NewReader("\n"))
		OfferDepInstall([]platform.DepIssue{freetypeIssue()}, in, true, rec.run, rec.recheck)
		if len(rec.ran) != 1 || rec.ran[0] != "sudo pacman -S freetype2" {
			t.Errorf("run 呼び出し = %v, want [sudo pacman -S freetype2]", rec.ran)
		}
		if len(rec.rechecked) != 1 || rec.rechecked[0] != "freetype2" {
			t.Errorf("recheck 呼び出し = %v, want [freetype2]", rec.rechecked)
		}
	})
	t.Run("y 入力で実行する", func(t *testing.T) {
		rec := &recorder{}
		in := bufio.NewReader(strings.NewReader("y\n"))
		OfferDepInstall([]platform.DepIssue{freetypeIssue()}, in, true, rec.run, rec.recheck)
		if len(rec.ran) != 1 {
			t.Errorf("run が呼ばれるべき: %v", rec.ran)
		}
	})
	t.Run("n 入力でスキップ（run も recheck も呼ばれない）", func(t *testing.T) {
		rec := &recorder{}
		in := bufio.NewReader(strings.NewReader("n\n"))
		OfferDepInstall([]platform.DepIssue{freetypeIssue()}, in, true, rec.run, rec.recheck)
		if len(rec.ran) != 0 || len(rec.rechecked) != 0 {
			t.Errorf("スキップなのに呼ばれた: run=%v recheck=%v", rec.ran, rec.rechecked)
		}
	})
	t.Run("EOF（入力が尽きた）は N 扱い", func(t *testing.T) {
		rec := &recorder{}
		in := bufio.NewReader(strings.NewReader(""))
		OfferDepInstall([]platform.DepIssue{freetypeIssue()}, in, true, rec.run, rec.recheck)
		if len(rec.ran) != 0 {
			t.Errorf("EOF で実行された: %v", rec.ran)
		}
	})
	t.Run("非 tty は提示のみ（対話も実行もしない・入力を消費しない）", func(t *testing.T) {
		rec := &recorder{}
		in := bufio.NewReader(strings.NewReader("y\n"))
		OfferDepInstall([]platform.DepIssue{freetypeIssue()}, in, false, rec.run, rec.recheck)
		if len(rec.ran) != 0 {
			t.Errorf("非 tty で実行された: %v", rec.ran)
		}
		if _, err := in.ReadString('\n'); err != nil {
			t.Errorf("非 tty は入力を消費すべきでない: %v", err)
		}
	})
	t.Run("実行失敗なら recheck しないで続行する", func(t *testing.T) {
		rec := &recorder{runErr: errors.New("boom")}
		in := bufio.NewReader(strings.NewReader("y\ny\n"))
		two := []platform.DepIssue{freetypeIssue(), {
			Kind:     "dotnet10",
			Title:    ".NET 10 ランタイム（ARM Linux で必要）",
			Commands: []string{"curl ... | bash"},
		}}
		OfferDepInstall(two, in, true, rec.run, rec.recheck)
		if len(rec.ran) != 2 {
			t.Errorf("失敗しても次の issue へ進むべき: ran=%v", rec.ran)
		}
		if len(rec.rechecked) != 0 {
			t.Errorf("失敗時に recheck された: %v", rec.rechecked)
		}
	})
	t.Run("Commands 空（distro 不明）は案内だけで対話しない", func(t *testing.T) {
		rec := &recorder{}
		issue := platform.DepIssue{Kind: "freetype2", Title: "freetype2",
			Fallback: "パッケージマネージャで freetype2 を導入してください。"}
		in := bufio.NewReader(strings.NewReader("y\n"))
		OfferDepInstall([]platform.DepIssue{issue}, in, true, rec.run, rec.recheck)
		if len(rec.ran) != 0 {
			t.Errorf("コマンド不明なのに実行された: %v", rec.ran)
		}
		if _, err := in.ReadString('\n'); err != nil {
			t.Errorf("対話せず入力を消費すべきでない: %v", err)
		}
	})
	t.Run("issue ゼロは何もしない", func(t *testing.T) {
		rec := &recorder{}
		OfferDepInstall(nil, bufio.NewReader(strings.NewReader("")), true, rec.run, rec.recheck)
		if len(rec.ran) != 0 || len(rec.rechecked) != 0 {
			t.Errorf("issue ゼロで何か呼ばれた")
		}
	})
}
