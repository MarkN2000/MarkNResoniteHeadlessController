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
	// cmd.Dir を Headless フォルダに変える（下記）。そのため相対パスのままだと headless 側が
	// 「自分の cwd(=Headless フォルダ)」基準で解決し、-HeadlessConfig は見つからず即終了
	// （"Config file not found!"・自前ログも出ない）、exe/dll も取り違える。MRHC の現在の cwd
	// 基準で絶対化してから組み立てる。Windows/Linux 共通: filepath.Abs は OS のセパレータと
	// 現在の cwd で解決し、既に絶対なら正規化のみ（意味は不変）。
	if abs, err := filepath.Abs(headlessPath); err == nil {
		headlessPath = abs
	}
	if configPath != "" {
		if abs, err := filepath.Abs(configPath); err == nil {
			configPath = abs
		}
	}

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
