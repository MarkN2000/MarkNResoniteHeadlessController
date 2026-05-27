// Package platform はOS差を隔離する。ここではResoniteヘッドレスの
// 起動コマンド構築を担う。
package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildHeadlessCommand はResoniteヘッドレスの起動コマンドを構築する。
//   - Windows: headlessPath は .../Headless/Resonite.exe（直接実行）
//   - Linux:   headlessPath は .../Headless/Resonite.dll。Resonite同梱の
//     dotnet（<install>/dotnet-runtime/dotnet、無ければPATHの dotnet）で実行
//
// 作業ディレクトリは常に Headless フォルダに設定する。
func BuildHeadlessCommand(headlessPath, configPath string) *exec.Cmd {
	headlessDir := filepath.Dir(headlessPath)

	var cmd *exec.Cmd
	if strings.EqualFold(filepath.Ext(headlessPath), ".dll") {
		dotnet := bundledDotnet(headlessDir)
		args := []string{filepath.Base(headlessPath)}
		if configPath != "" {
			args = append(args, "-HeadlessConfig", configPath)
		}
		cmd = exec.Command(dotnet, args...)
	} else {
		var args []string
		if configPath != "" {
			args = append(args, "-HeadlessConfig", configPath)
		}
		cmd = exec.Command(headlessPath, args...)
	}
	cmd.Dir = headlessDir
	return cmd
}

// bundledDotnet はResonite同梱の dotnet パスを返す。無ければ "dotnet"
// （PATH上のdotnet）にフォールバックする。
func bundledDotnet(headlessDir string) string {
	installRoot := filepath.Dir(headlessDir) // .../Resonite
	for _, name := range []string{"dotnet", "dotnet.exe"} {
		candidate := filepath.Join(installRoot, "dotnet-runtime", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "dotnet"
}
