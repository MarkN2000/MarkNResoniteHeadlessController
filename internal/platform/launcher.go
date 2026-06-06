// Package platform はOS差を隔離する。ここではResoniteヘッドレスの
// 起動コマンド構築を担う。
package platform

import (
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BuildHeadlessCommand はResoniteヘッドレスの起動コマンドを構築する。
//   - Windows: headlessPath は .../Headless/Resonite.exe（直接実行）
//   - Linux:   headlessPath は .../Headless/Resonite.dll。dotnet 経由で実行する
//     （dotnet の解決は resolveDotnet が担う: 同梱が実行 arch と一致すれば同梱、
//     ARM 等で不一致なら ~/.dotnet/dotnet→PATH のシステム dotnet）。
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
		dotnet := resolveDotnet(headlessDir, runtime.GOARCH)
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
		// MRHC が設置したローカルランタイム（<install>/dotnet-runtime）が要求を満たすときだけ
		// DOTNET_ROOT で指す（Windows）。apphost は DOTNET_ROOT 設定時そこだけを見てシステムへ
		// フォールバックしないため、「存在する」でなく「充足する」を条件にする。
		// 満たさなければ env を触らない＝システム .NET（従来挙動・開発機や Steam クライアント
		// 併用環境は不変）。
		if req, ok := ReadRuntimeRequirement(headlessDir); ok {
			installRoot := filepath.Dir(headlessDir)
			if LocalRuntimeSatisfies(installRoot, req, runtime.GOARCH) {
				cmd.Env = append(os.Environ(), "DOTNET_ROOT="+filepath.Join(installRoot, "dotnet-runtime"))
			}
		}
	}
	cmd.Dir = headlessDir
	return cmd
}

// resolveDotnet は Resonite.dll を実行する dotnet のパスを解決する。
// ローカル dotnet（<install>/dotnet-runtime/dotnet。MRHC が設置 or 公式ブートストラップが生成）が
// 実行 arch(goarch) と一致し、かつ要求版を満たす時だけ採用し、それ以外はシステムの dotnet に
// フォールバックする（arch 不一致の残骸を掴むと "Exec format error"・版不足は起動失敗になるため）。
func resolveDotnet(headlessDir, goarch string) string {
	if bundled, ok := bundledDotnetPath(headlessDir); ok && dotnetUsable(bundled, goarch) && bundledSatisfies(headlessDir, goarch) {
		return bundled
	}
	return systemDotnet()
}

// bundledSatisfies は要求（runtimeconfig.json）が読める場合に、ローカルランタイムが要求版を
// 満たすかを判定する。読めない場合は従来どおり楽観（true）＝runtimeconfig の無い環境
// （fakehl 等）で挙動を変えない。設置失敗で stale なローカルが残ったケースをここで弾き、
// システム .NET へ正しく落とす。
func bundledSatisfies(headlessDir, goarch string) bool {
	req, ok := ReadRuntimeRequirement(headlessDir)
	if !ok {
		return true
	}
	return LocalRuntimeSatisfies(filepath.Dir(headlessDir), req, goarch)
}

// bundledDotnetPath はResonite同梱の dotnet 実行ファイルを探す。
// 見つかればそのパスと true、無ければ ("", false)。
func bundledDotnetPath(headlessDir string) (string, bool) {
	installRoot := filepath.Dir(headlessDir) // .../Resonite
	for _, name := range []string{"dotnet", "dotnet.exe"} {
		candidate := filepath.Join(installRoot, "dotnet-runtime", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// systemDotnet はシステムにインストールされた dotnet のパスを返す。
// ~/.dotnet/dotnet があればそれを、無ければ PATH 上の "dotnet" を返す。
// 同梱 dotnet が使えない ARM 環境で、別途導入した .NET ランタイムへ橋渡しする。
// この経路は Resonite.dll 実行（非 Windows）でのみ通るため、拡張子無しの "dotnet" のみ探す。
// 返す dotnet の arch・版(.NET 10) の妥当性はここでは検証しない（依存チェックは後続 R-C の責務）。
// 本関数はパス解決に徹する。
func systemDotnet() string {
	candidate := ExpandHome(filepath.Join("~", ".dotnet", "dotnet"))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return "dotnet"
}

// dotnetUsable は path の実行ファイルが実行 arch(goarch) で使えるかを判定する。
// ELF として arch が判明し、かつ goarch と食い違う時だけ false（＝同梱を拒否）。
// ELF でない・読めない・未知の machine（判定不能）は true を返し、従来どおり楽観採用する
// （x64 や Windows/mac の挙動を変えないため）。
func dotnetUsable(path, goarch string) bool {
	arch := elfArch(path)
	return arch == "" || arch == goarch
}

// elfArch は ELF 実行ファイルの命令セット（e_machine）を Go の GOARCH 表記で返す。
// ELF でない・読めない・未対応 machine は "" を返す。
// arch 判定に要るのは先頭 20 バイト（e_ident+e_type+e_machine）だけなので、debug/elf で
// ファイル全体を解釈せず自前で必要最小限だけ読む（import を増やさず・読み込み量も最小）。
func elfArch(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var hdr [20]byte // e_ident(16) + e_type(2) + e_machine(2)
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return ""
	}
	if hdr[0] != 0x7f || hdr[1] != 'E' || hdr[2] != 'L' || hdr[3] != 'F' {
		return ""
	}

	var bo binary.ByteOrder
	switch hdr[5] { // EI_DATA
	case 1:
		bo = binary.LittleEndian
	case 2:
		bo = binary.BigEndian
	default:
		return ""
	}

	switch bo.Uint16(hdr[18:20]) { // e_machine
	case 62: // EM_X86_64
		return "amd64"
	case 183: // EM_AARCH64
		return "arm64"
	case 3: // EM_386（32bit x86。Fedora 系の /usr/lib は 32bit のため判別必須・R-C）
		return "386"
	case 40: // EM_ARM（32bit ARM）
		return "arm"
	default:
		return ""
	}
}
