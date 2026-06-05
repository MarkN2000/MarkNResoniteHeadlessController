package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HeadlessBinaryName はこの OS の Resonite ヘッドレス実行ファイル名を返す。
//   - Windows: Resonite.exe（直接実行）
//   - それ以外: Resonite.dll（同梱/システムの dotnet 経由で実行）
//
// config.HeadlessPathOrDefault へ注入し、既定パスを OS 別に導出する（R-A）。
func HeadlessBinaryName() string {
	if runtime.GOOS == "windows" {
		return "Resonite.exe"
	}
	return "Resonite.dll"
}

// ExpandHome は先頭の "~" をユーザーのホームディレクトリへ展開する。
// 対象は "~" 単独・"~/..."・"~\..."（Windows）のみ（"~user" 形式は非対応）。
// filepath.Abs は "~" を展開しない（"./~/..." 扱いになる）ため、利用時に本ヘルパで
// 展開してから OS へ渡す（R-A・利用時展開方針）。home 取得失敗時は入力をそのまま返す。
func ExpandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}
