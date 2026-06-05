// deps.go はヘッドレス動作に必要な外部依存（Linux の freetype2 / ARM の .NET 10）を
// 検出し、不足時の導入手段を組み立てる（R-C）。検出結果は 3 値（present/absent/unknown）で、
// 案内を出すのは absent（検出手段は機能したが無い）と確認できたときだけ。unknown
// （検出手段自体が機能しない環境）は黙る＝誤警告ゼロを優先する（R-B の「判定不能は楽観」と同思想）。
// 詳細仕様: docs/design/deps-onboarding.md
package platform

import (
	"context"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// DepIssue は不足している外部依存 1 件と導入手段。
type DepIssue struct {
	Kind     string   // "freetype2" | "dotnet10"
	Title    string   // 表示名
	Commands []string // 導入コマンド（[Y/n] 実行・ログ提示の両方で使う。distro 不明時は空）
	Fallback string   // Commands が空（distro 不明）のときの手動導入案内
	Sudo     bool     // sudo を伴うか（文言出し分け用）
}

// dotnetInstallCmd は .NET 10 ランタイムの導入コマンド。
// 公式 dotnet-install.sh・sudo 不要・~/.dotnet 配下にランタイムのみ導入。
// 導入後は launcher の systemDotnet() が ~/.dotnet/dotnet を最優先で拾う（R-B）。
const dotnetInstallCmd = "curl -fsSL https://dot.net/v1/dotnet-install.sh | bash -s -- --channel 10.0 --runtime dotnet"

// CheckHeadlessDeps はヘッドレス動作に必要な外部依存の不足を検出する。
// absent と確認できたものだけ返す（present/unknown は返さない）。
// goos != "linux" は常に nil（Windows/mac は完全 no-op）。
// installDir は内部で "~" を展開する（呼び出し側の展開漏れを防ぐ）。
func CheckHeadlessDeps(goos, goarch, installDir string) []DepIssue {
	return checkHeadlessDeps(realDepProbe(), goos, goarch, installDir)
}

// depStatus は依存検出の 3 値。
type depStatus int

const (
	depUnknown depStatus = iota // 検出手段自体が機能しなかった（黙る）
	depPresent                  // 在ると確認できた
	depAbsent                   // 検出手段は機能したが見つからない（案内を出す）
)

// depProbe は検出の I/O を束ねる seam。本番は realDepProbe()、テストは偽実装を注入する
// （実ホストの /usr/lib 等に依存せず決定的にマトリクスを検証するため）。
type depProbe struct {
	lookPath func(string) (string, error)
	stat     func(string) (os.FileInfo, error)
	readDir  func(string) ([]os.DirEntry, error)
	readFile func(string) ([]byte, error) // /etc/os-release
	runCmd   func(timeout time.Duration, name string, args ...string) (string, error)
	elfArch  func(string) string
	home     string // "~" 展開用
}

func realDepProbe() depProbe {
	home, _ := os.UserHomeDir() // 失敗時は空＝ "~" は展開されずそのまま（ExpandHome と同じ縮退）
	return depProbe{
		lookPath: exec.LookPath,
		stat:     os.Stat,
		readDir:  os.ReadDir,
		readFile: os.ReadFile,
		runCmd: func(timeout time.Duration, name string, args ...string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			out, err := exec.CommandContext(ctx, name, args...).Output()
			return string(out), err
		},
		elfArch: elfArch,
		home:    home,
	}
}

func checkHeadlessDeps(p depProbe, goos, goarch, installDir string) []DepIssue {
	if goos != "linux" {
		return nil
	}
	installDir = expandHomeLinux(installDir, p.home)

	var issues []DepIssue
	if freetypeStatus(p, goarch) == depAbsent {
		issue := DepIssue{
			Kind:  "freetype2",
			Title: "freetype2（Resonite のネイティブ依存）",
			Sudo:  true,
		}
		if cmd := freetypeInstallCmd(osReleasePkgManager(p)); cmd != "" {
			issue.Commands = []string{cmd}
		} else {
			issue.Fallback = "お使いのディストリビューションのパッケージマネージャで freetype2（Debian系では libfreetype6）を導入してください。"
		}
		issues = append(issues, issue)
	}
	// .NET 10 は linux/arm64 のみ（x64 は Resonite 同梱 dotnet で完結・実機実証済み。
	// arm(32bit) は Resonite 自体が非対応のため対象外）。
	if goarch == "arm64" && dotnet10Status(p, goarch, installDir) == depAbsent {
		issues = append(issues, DepIssue{
			Kind:     "dotnet10",
			Title:    ".NET 10 ランタイム（ARM Linux で必要）",
			Commands: []string{dotnetInstallCmd},
		})
	}
	return issues
}

// --- freetype2（全 Linux） ---

// freetypeStatus は libfreetype.so.6 の有無を 3 値で判定する。
// 候補を ldconfig -p と既知 lib ディレクトリ走査の 2 系統で集め、
// 「使用可能な候補（stat 成功かつ ELF arch が実行 arch と一致 or 判定不能）」が
// 1 つでもあれば present。検出手段が機能したのに使用可能候補ゼロなら absent
// （候補が全て他 arch＝multiarch で ARM 機に amd64 の so だけがあるケースも absent）。
func freetypeStatus(p depProbe, goarch string) depStatus {
	var candidates []string
	probeWorked := false

	if out, ok := runLdconfig(p); ok {
		probeWorked = true
		candidates = append(candidates, ldconfigCandidates(out)...)
	}
	for _, dir := range freetypeLibDirs(goarch) {
		entries, err := p.readDir(dir)
		if err != nil {
			continue
		}
		probeWorked = true
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "libfreetype.so.6") {
				candidates = append(candidates, path.Join(dir, e.Name()))
			}
		}
	}
	if !probeWorked {
		return depUnknown
	}
	for _, c := range candidates {
		if _, err := p.stat(c); err != nil {
			continue // dangling symlink・消えた実体を除外（stat は symlink を追う）
		}
		if a := p.elfArch(c); a == "" || a == goarch {
			return depPresent
		}
	}
	return depAbsent
}

// runLdconfig は ldconfig -p を実行して出力を返す。ldconfig は Debian 系で
// /sbin に置かれ一般ユーザーの PATH に無いことがあるため、PATH → /sbin → /usr/sbin
// の順で実体を探す。見つからない・実行失敗は (_, false)。
func runLdconfig(p depProbe) (string, bool) {
	name := ""
	if path, err := p.lookPath("ldconfig"); err == nil {
		name = path
	} else {
		for _, c := range []string{"/sbin/ldconfig", "/usr/sbin/ldconfig"} {
			if _, err := p.stat(c); err == nil {
				name = c
				break
			}
		}
	}
	if name == "" {
		return "", false
	}
	out, err := p.runCmd(5*time.Second, name, "-p")
	if err != nil {
		return "", false
	}
	return out, true
}

// ldconfigCandidates は `ldconfig -p` の出力から libfreetype.so.6 の実体パスを抽出する。
// 行形式: "\tlibfreetype.so.6 (libc6,x86-64) => /usr/lib/libfreetype.so.6"
// （先頭のヘッダ行「NNN libs found…」は libfreetype を含まないため自然に無視される。
// musl/Alpine の ldconfig は -p 非対応で出力空＝候補ゼロだが、その場合も
// ディレクトリ走査側が機能するため判定は壊れない）。
func ldconfigCandidates(out string) []string {
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "libfreetype.so.6") {
			continue
		}
		if i := strings.LastIndex(line, "=> "); i >= 0 {
			if path := strings.TrimSpace(line[i+3:]); path != "" {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

// freetypeLibDirs は libfreetype.so.6 を探す既知 lib ディレクトリ。
// multiarch の triplet ディレクトリは実行 arch のものだけ見る
// （他 arch のディレクトリで見つけても使えないため）。
func freetypeLibDirs(goarch string) []string {
	dirs := []string{"/usr/lib", "/usr/lib64", "/lib", "/lib64"}
	switch goarch {
	case "amd64":
		dirs = append(dirs, "/usr/lib/x86_64-linux-gnu", "/lib/x86_64-linux-gnu")
	case "arm64":
		dirs = append(dirs, "/usr/lib/aarch64-linux-gnu", "/lib/aarch64-linux-gnu")
	}
	return dirs
}

// --- .NET 10（linux/arm64 のみ） ---

// dotnet10Status は .NET 10 ベースランタイムの有無を 3 値で判定する。
//  1. Resonite 同梱 dotnet が実行 arch で使えるなら present
//     （将来 Resonite が ARM 同梱 dotnet を配布しても誤案内しない）
//  2. システム候補（~/.dotnet/dotnet → PATH の dotnet）がどこにも無ければ absent
//     （systemDotnet() と同順序。同関数はパス解決専用で「無い」を表現できないため流用しない）
//  3. 候補の ELF arch が判明して実行 arch と不一致なら absent
//     （x64 dotnet を ARM に誤導入したケース。実行しても ENOEXEC で確実に失敗する）
//  4. `dotnet --list-runtimes` に Microsoft.NETCore.App 10.x があれば present、
//     無ければ absent（導入コマンドはそのまま有効）。実行失敗/タイムアウトは unknown
//     （ARM SBC は .NET プロセス起動自体が遅い実績があるため 10s の防御値）。
func dotnet10Status(p depProbe, goarch, installDir string) depStatus {
	if installDir != "" {
		bundled := path.Join(installDir, "dotnet-runtime", "dotnet")
		if _, err := p.stat(bundled); err == nil {
			if a := p.elfArch(bundled); a == "" || a == goarch { // dotnetUsable と同規則
				return depPresent
			}
		}
	}

	candidate := path.Join(p.home, ".dotnet", "dotnet")
	if _, err := p.stat(candidate); p.home == "" || err != nil {
		found, err := p.lookPath("dotnet")
		if err != nil {
			return depAbsent
		}
		candidate = found
	}
	if a := p.elfArch(candidate); a != "" && a != goarch {
		return depAbsent
	}

	out, err := p.runCmd(10*time.Second, candidate, "--list-runtimes")
	if err != nil {
		return depUnknown
	}
	if hasDotnet10(out) {
		return depPresent
	}
	return depAbsent
}

// hasDotnet10 は `dotnet --list-runtimes` の出力に .NET 10 のベースランタイムが
// 含まれるかを判定する。行形式: "Microsoft.NETCore.App 10.0.x [/path/...]"。
// ASP.NET 用の Microsoft.AspNetCore.App は見ない（ヘッドレスに必要なのはベースのみ）。
func hasDotnet10(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Microsoft.NETCore.App 10.") {
			return true
		}
	}
	return false
}

// --- distro 判定（freetype2 の導入コマンド出し分け） ---

// pkgManager は distro 系列のパッケージマネージャ。空文字は不明（コマンドを出さない）。
type pkgManager string

const (
	pkgUnknown pkgManager = ""
	pkgPacman  pkgManager = "pacman"
	pkgApt     pkgManager = "apt"
	pkgDnf     pkgManager = "dnf"
	pkgZypper  pkgManager = "zypper"
)

// osReleasePkgManager は /etc/os-release からパッケージマネージャを判定する。
func osReleasePkgManager(p depProbe) pkgManager {
	b, err := p.readFile("/etc/os-release")
	if err != nil {
		return pkgUnknown
	}
	return pkgManagerFromOSRelease(string(b))
}

// pkgManagerFromOSRelease は os-release の内容から ID → ID_LIKE の順でトークンを
// 既知の distro 系列に照合する（純関数）。
// cachyos を ID として直接持つのは、CachyOS の ID_LIKE=arch 自動付与が 2025-11 の
// hooks 修正以降で、未更新システムでは ID_LIKE が欠けうるため。
// apk(Alpine) を持たないのは意図的（Resonite は glibc 前提で musl では動かない）。
func pkgManagerFromOSRelease(content string) pkgManager {
	id, idLike := parseOSRelease(content)
	for _, token := range append([]string{id}, idLike...) {
		switch {
		case token == "arch" || token == "cachyos":
			return pkgPacman
		case token == "debian" || token == "ubuntu":
			return pkgApt
		case token == "fedora" || token == "rhel" || token == "centos":
			return pkgDnf
		case token == "suse" || token == "sles" || token == "opensuse" ||
			strings.HasPrefix(token, "opensuse-"):
			return pkgZypper
		}
	}
	return pkgUnknown
}

// parseOSRelease は os-release から ID と ID_LIKE（スペース区切り）を取り出す。
// 値のクォート（" / '）は除去する。
func parseOSRelease(content string) (id string, idLike []string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "ID="); ok {
			id = strings.Trim(v, `"'`)
		} else if v, ok := strings.CutPrefix(line, "ID_LIKE="); ok {
			idLike = strings.Fields(strings.Trim(v, `"'`))
		}
	}
	return id, idLike
}

// expandHomeLinux は先頭の "~" をホームへ展開する（Linux パス専用）。
// 本ファイルの検出対象は goos=="linux" に限られるため、OS 依存の filepath では
// なく常に "/" の path 系で扱う（Windows 上のテストも決定的になる）。
// home が空（取得失敗）の場合は入力をそのまま返す（ExpandHome と同じ縮退）。
func expandHomeLinux(p, home string) string {
	if home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return path.Join(home, rest)
	}
	return p
}

// freetypeInstallCmd は distro 系列ごとの freetype 導入コマンド。
// パッケージ名は distro で異なる（Arch=freetype2 / Debian系=libfreetype6 /
// Fedora系=freetype / openSUSE=libfreetype6。2026-06-05 Web 検証済み）。
func freetypeInstallCmd(pm pkgManager) string {
	switch pm {
	case pkgPacman:
		return "sudo pacman -S freetype2"
	case pkgApt:
		return "sudo apt install libfreetype6"
	case pkgDnf:
		return "sudo dnf install freetype"
	case pkgZypper:
		return "sudo zypper install libfreetype6"
	default:
		return ""
	}
}
