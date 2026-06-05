package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestHeadlessBinaryName は OS 別のヘッドレス実行ファイル名を検証する（R-A）。
func TestHeadlessBinaryName(t *testing.T) {
	got := HeadlessBinaryName()
	want := "Resonite.dll"
	if runtime.GOOS == "windows" {
		want = "Resonite.exe"
	}
	if got != want {
		t.Errorf("HeadlessBinaryName()=%q want %q (GOOS=%s)", got, want, runtime.GOOS)
	}
}

// TestExpandHome は先頭 "~" の展開と、対象外パスの素通しを検証する（R-A）。
func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("home 取得不可のためスキップ: %v", err)
	}

	// "~/..." は home 配下へ展開される。
	got := ExpandHome("~/Resonite")
	want := filepath.Join(home, "Resonite")
	if got != want {
		t.Errorf("ExpandHome(~/Resonite)=%q want %q", got, want)
	}

	// "~" 単独は home そのもの。
	if got := ExpandHome("~"); got != home {
		t.Errorf("ExpandHome(~)=%q want %q", got, home)
	}

	// 先頭が "~" でないものは素通し（"~" を含む途中文字列も展開しない）。
	for _, p := range []string{"/abs/path", "relative/path", "C:/x", "~user/x", "a~b"} {
		if got := ExpandHome(p); got != p {
			t.Errorf("ExpandHome(%q)=%q want 素通し", p, got)
		}
	}

	// 展開結果が home 始まりであること（セパレータ非依存の緩い確認）。
	if !strings.HasPrefix(ExpandHome("~/a/b"), home) {
		t.Errorf("ExpandHome(~/a/b) が home 始まりでない")
	}
}
