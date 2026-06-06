// deps.go はヘッドレス動作に必要な外部依存（Linux の freetype2）を検出し、不足時の
// 導入手段を組み立てる（R-C）。検出結果は 3 値（present/absent/unknown）で、
// 案内を出すのは absent（検出手段は機能したが無い）と確認できたときだけ。unknown
// （検出手段自体が機能しない環境）は黙る＝誤警告ゼロを優先する（R-B の「判定不能は楽観」と同思想）。
// .NET 10 の検出・案内（旧 dotnet10・ARM 限定）は自動設置（internal/dotnetruntime・
// docs/design/dotnet-runtime.md）へ置換され撤去した。freetype2 は sudo 必須＝自動化できない
// ため案内方式のまま存続する。詳細仕様: docs/design/deps-onboarding.md
package platform

import (
	"context"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
)

// DepIssue は不足している外部依存 1 件と導入手段。表示文言は持たないデータ専用
// （文言は Title/GuideText が config 言語で組み立てる。Kind は閉じた集合なので
// カタログキーを "deps.title.<Kind>" 形式で引ける）。
type DepIssue struct {
	Kind     string   // "freetype2"
	Commands []string // 導入コマンド（[Y/n] 実行・ログ提示の両方で使う。distro 不明時は空）
}

// Title は表示名（例「freetype2（Resonite のネイティブ依存）」）。
func (i DepIssue) Title(lang i18n.Lang) string {
	return i18n.T(lang, "deps.title."+i.Kind)
}

// GuideText は導入ガイド本文（コマンドがあればラベル付きで結合・無ければ Kind 別の手動案内）。
// 経路②（起動時ログ）と経路③（sys ログ）の文言選択を 1 か所に集約する。
func (i DepIssue) GuideText(lang i18n.Lang) string {
	if len(i.Commands) > 0 {
		return i18n.T(lang, "deps.guide.commands", strings.Join(i.Commands, " && "))
	}
	return i18n.T(lang, "deps.fallback."+i.Kind)
}

// CheckHeadlessDeps はヘッドレス動作に必要な外部依存の不足を検出する。
// absent と確認できたものだけ返す（present/unknown は返さない）。
// goos != "linux" は常に nil（Windows/mac は完全 no-op）。
func CheckHeadlessDeps(goos, goarch string) []DepIssue {
	return checkHeadlessDeps(realDepProbe(), goos, goarch)
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

func checkHeadlessDeps(p depProbe, goos, goarch string) []DepIssue {
	if goos != "linux" {
		return nil
	}
	var issues []DepIssue
	if freetypeStatus(p, goarch) == depAbsent {
		issue := DepIssue{Kind: "freetype2"}
		if cmd := freetypeInstallCmd(osReleasePkgManager(p)); cmd != "" {
			issue.Commands = []string{cmd}
		} // distro 不明は Commands 空＝GuideText が手動導入の案内（fallback）を返す
		issues = append(issues, issue)
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
