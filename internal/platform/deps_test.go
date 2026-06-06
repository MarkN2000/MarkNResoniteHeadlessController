package platform

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
)

// --- テスト用の偽 probe ---

// emptyProbe は全ての I/O が失敗する depProbe（個々のテストで必要な口だけ差し替える）。
// 実ホストの /usr/lib 等に依存せず決定的に検証するための土台。
func emptyProbe() depProbe {
	notFound := errors.New("not found (test)")
	return depProbe{
		lookPath: func(string) (string, error) { return "", notFound },
		stat:     func(string) (os.FileInfo, error) { return nil, fs.ErrNotExist },
		readDir:  func(string) ([]os.DirEntry, error) { return nil, fs.ErrNotExist },
		readFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
		runCmd: func(time.Duration, string, ...string) (string, error) {
			return "", notFound
		},
		elfArch: func(string) string { return "" },
		home:    "/home/test",
	}
}

// fakeDirEntry は readDir の偽戻り値（Name しか使われない）。
type fakeDirEntry struct{ name string }

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return false }
func (f fakeDirEntry) Type() fs.FileMode          { return 0 }
func (f fakeDirEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrNotExist }

// statOK は指定パス集合だけ stat 成功にする。
func statOK(paths ...string) func(string) (os.FileInfo, error) {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(p string) (os.FileInfo, error) {
		if set[p] {
			return nil, nil // 呼び出し側は err しか見ない
		}
		return nil, fs.ErrNotExist
	}
}

// archOf はパス→GOARCH の対応表で elfArch を偽装する（表に無いパスは ""=判定不能）。
func archOf(m map[string]string) func(string) string {
	return func(p string) string { return m[p] }
}

// --- distro 判定 ---

func TestPkgManagerFromOSRelease(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    pkgManager
	}{
		{"arch", "NAME=\"Arch Linux\"\nID=arch\n", pkgPacman},
		{"cachyos ID_LIKE あり", "ID=cachyos\nID_LIKE=arch\n", pkgPacman},
		// CachyOS の ID_LIKE 自動付与は 2025-11 以降＝未更新システムでは欠ける（ID 直接で拾う）
		{"cachyos ID_LIKE 欠落", "ID=cachyos\n", pkgPacman},
		{"debian", "ID=debian\n", pkgApt},
		{"ubuntu", "ID=ubuntu\nID_LIKE=debian\n", pkgApt},
		{"mint（ID_LIKE で拾う）", "ID=linuxmint\nID_LIKE=\"ubuntu debian\"\n", pkgApt},
		{"fedora（ID_LIKE 無し）", "ID=fedora\n", pkgDnf},
		{"almalinux", "ID=\"almalinux\"\nID_LIKE=\"rhel centos fedora\"\n", pkgDnf},
		{"rocky", "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\n", pkgDnf},
		{"opensuse-tumbleweed", "ID=\"opensuse-tumbleweed\"\nID_LIKE=\"opensuse suse\"\n", pkgZypper},
		{"opensuse-leap", "ID=\"opensuse-leap\"\nID_LIKE=\"suse opensuse\"\n", pkgZypper},
		{"sles", "ID=\"sles\"\nID_LIKE=\"suse\"\n", pkgZypper},
		{"未知 distro", "ID=gentoo\n", pkgUnknown},
		{"alpine は意図的にマップ外", "ID=alpine\n", pkgUnknown},
		{"空文字", "", pkgUnknown},
		{"シングルクォート", "ID='ubuntu'\n", pkgApt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pkgManagerFromOSRelease(tc.content); got != tc.want {
				t.Errorf("pkgManagerFromOSRelease(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

// --- .NET 10 判定 ---

func TestHasDotnet10(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{
		{"10 あり", "Microsoft.NETCore.App 10.0.2 [/home/u/.dotnet/shared/Microsoft.NETCore.App]\n", true},
		{"9 と 10 混在", "Microsoft.NETCore.App 9.0.7 [/p]\nMicrosoft.NETCore.App 10.0.0 [/p]\n", true},
		{"9 のみ", "Microsoft.NETCore.App 9.0.7 [/p]\n", false},
		// AspNetCore だけでは不足（ヘッドレスに必要なのはベースランタイム）
		{"AspNetCore 10 のみ", "Microsoft.AspNetCore.App 10.0.0 [/p]\n", false},
		{"空出力", "", false},
		// 10.x の前方一致であって 100.x 等を誤検知しない（"10." 区切りで判定）
		{"バージョン誤検知なし", "Microsoft.NETCore.App 100.0.0 [/p]\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasDotnet10(tc.out); got != tc.want {
				t.Errorf("hasDotnet10(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

func TestDotnet10Status(t *testing.T) {
	const homeDotnet = "/home/test/.dotnet/dotnet"
	list10 := "Microsoft.NETCore.App 10.0.2 [/p]\n"
	list9 := "Microsoft.NETCore.App 9.0.7 [/p]\n"

	t.Run("同梱 dotnet が arch 一致なら present（list-runtimes 不要）", func(t *testing.T) {
		p := emptyProbe()
		p.stat = statOK("/inst/dotnet-runtime/dotnet")
		p.elfArch = archOf(map[string]string{"/inst/dotnet-runtime/dotnet": "arm64"})
		if got := dotnet10Status(p, "arm64", "/inst"); got != depPresent {
			t.Errorf("got %v, want depPresent", got)
		}
	})
	t.Run("同梱が他 arch（x64）ならシステム側へフォールバック→どこにも無く absent", func(t *testing.T) {
		p := emptyProbe()
		p.stat = statOK("/inst/dotnet-runtime/dotnet")
		p.elfArch = archOf(map[string]string{"/inst/dotnet-runtime/dotnet": "amd64"})
		if got := dotnet10Status(p, "arm64", "/inst"); got != depAbsent {
			t.Errorf("got %v, want depAbsent", got)
		}
	})
	t.Run("~/.dotnet の dotnet に 10 があれば present", func(t *testing.T) {
		p := emptyProbe()
		p.stat = statOK(homeDotnet)
		p.runCmd = func(_ time.Duration, name string, args ...string) (string, error) {
			if name != homeDotnet {
				t.Errorf("~/.dotnet を最優先で実行すべき: %q", name)
			}
			return list10, nil
		}
		if got := dotnet10Status(p, "arm64", "/inst"); got != depPresent {
			t.Errorf("got %v, want depPresent", got)
		}
	})
	t.Run("PATH の dotnet に 10 が無ければ absent", func(t *testing.T) {
		p := emptyProbe()
		p.lookPath = func(string) (string, error) { return "/usr/bin/dotnet", nil }
		p.runCmd = func(_ time.Duration, _ string, _ ...string) (string, error) { return list9, nil }
		if got := dotnet10Status(p, "arm64", "/inst"); got != depAbsent {
			t.Errorf("got %v, want depAbsent", got)
		}
	})
	t.Run("候補がどこにも無ければ absent", func(t *testing.T) {
		p := emptyProbe()
		if got := dotnet10Status(p, "arm64", "/inst"); got != depAbsent {
			t.Errorf("got %v, want depAbsent", got)
		}
	})
	t.Run("~/.dotnet が他 arch（x64 誤導入）なら absent", func(t *testing.T) {
		p := emptyProbe()
		p.stat = statOK(homeDotnet)
		p.elfArch = archOf(map[string]string{homeDotnet: "amd64"})
		if got := dotnet10Status(p, "arm64", "/inst"); got != depAbsent {
			t.Errorf("got %v, want depAbsent", got)
		}
	})
	t.Run("候補は在るが実行失敗なら unknown（黙る）", func(t *testing.T) {
		p := emptyProbe()
		p.stat = statOK(homeDotnet)
		if got := dotnet10Status(p, "arm64", "/inst"); got != depUnknown {
			t.Errorf("got %v, want depUnknown", got)
		}
	})
}

// --- freetype2 判定 ---

func TestLdconfigCandidates(t *testing.T) {
	out := "813 libs found in cache `/etc/ld.so.cache'\n" +
		"\tlibfreetype.so.6 (libc6,x86-64) => /usr/lib/libfreetype.so.6\n" +
		"\tlibfoo.so.1 (libc6,x86-64) => /usr/lib/libfoo.so.1\n" +
		"\tlibfreetype.so.6 (libc6) => /usr/lib32/libfreetype.so.6\n"
	got := ldconfigCandidates(out)
	want := []string{"/usr/lib/libfreetype.so.6", "/usr/lib32/libfreetype.so.6"}
	if len(got) != len(want) {
		t.Fatalf("候補数 = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("候補[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFreetypeStatus(t *testing.T) {
	const soPath = "/usr/lib/libfreetype.so.6"
	ldOut := "\tlibfreetype.so.6 (libc6,x86-64) => " + soPath + "\n"
	withLdconfig := func(p depProbe, out string) depProbe {
		p.lookPath = func(name string) (string, error) {
			if name == "ldconfig" {
				return "/sbin/ldconfig", nil
			}
			return "", errors.New("not found")
		}
		p.runCmd = func(_ time.Duration, _ string, _ ...string) (string, error) { return out, nil }
		return p
	}

	t.Run("ldconfig 経由で present（arch 一致）", func(t *testing.T) {
		p := withLdconfig(emptyProbe(), ldOut)
		p.stat = statOK(soPath, "/sbin/ldconfig")
		p.elfArch = archOf(map[string]string{soPath: "amd64"})
		if got := freetypeStatus(p, "amd64"); got != depPresent {
			t.Errorf("got %v, want depPresent", got)
		}
	})
	t.Run("ディレクトリ走査で present（ldconfig 無し・elf 判定不能は楽観）", func(t *testing.T) {
		p := emptyProbe()
		p.readDir = func(dir string) ([]os.DirEntry, error) {
			if dir == "/usr/lib" {
				return []os.DirEntry{fakeDirEntry{"libfreetype.so.6.20.4"}}, nil
			}
			return nil, fs.ErrNotExist
		}
		p.stat = statOK("/usr/lib/libfreetype.so.6.20.4")
		if got := freetypeStatus(p, "amd64"); got != depPresent {
			t.Errorf("got %v, want depPresent", got)
		}
	})
	t.Run("検出手段は機能・候補ゼロ → absent", func(t *testing.T) {
		p := withLdconfig(emptyProbe(), "0 libs found in cache\n")
		if got := freetypeStatus(p, "amd64"); got != depAbsent {
			t.Errorf("got %v, want depAbsent", got)
		}
	})
	t.Run("候補が全て他 arch → absent（ARM 機に amd64 so だけの multiarch）", func(t *testing.T) {
		p := withLdconfig(emptyProbe(), ldOut)
		p.stat = statOK(soPath, "/sbin/ldconfig")
		p.elfArch = archOf(map[string]string{soPath: "amd64"})
		if got := freetypeStatus(p, "arm64"); got != depAbsent {
			t.Errorf("got %v, want depAbsent", got)
		}
	})
	t.Run("dangling symlink（stat 失敗）は候補から除外 → absent", func(t *testing.T) {
		p := withLdconfig(emptyProbe(), ldOut)
		p.stat = statOK("/sbin/ldconfig") // so 本体は stat 失敗のまま
		if got := freetypeStatus(p, "amd64"); got != depAbsent {
			t.Errorf("got %v, want depAbsent", got)
		}
	})
	t.Run("ldconfig も既知ディレクトリも全滅 → unknown（黙る）", func(t *testing.T) {
		p := emptyProbe()
		if got := freetypeStatus(p, "amd64"); got != depUnknown {
			t.Errorf("got %v, want depUnknown", got)
		}
	})
	t.Run("triplet ディレクトリは実行 arch のものだけ見る", func(t *testing.T) {
		dirs := freetypeLibDirs("arm64")
		for _, d := range dirs {
			if strings.Contains(d, "x86_64") {
				t.Errorf("arm64 の走査対象に x86_64 triplet が混入: %v", dirs)
			}
		}
	})
}

// GuideText は経路②③共用のガイド本文（コマンド=ラベル付き結合 / 無ければ Kind 別の手動案内）。
// 文言は i18n カタログ（config 言語）から組み立てる。
func TestDepIssueGuideText(t *testing.T) {
	withCmd := DepIssue{Kind: "freetype2", Commands: []string{"sudo pacman -S freetype2"}}
	if got := withCmd.GuideText(i18n.Ja); got != "導入コマンド: sudo pacman -S freetype2" {
		t.Errorf("GuideText(ja) = %q", got)
	}
	if got := withCmd.GuideText(i18n.En); got != "Install command: sudo pacman -S freetype2" {
		t.Errorf("GuideText(en) = %q", got)
	}
	fallback := DepIssue{Kind: "freetype2"} // Commands 空＝distro 不明
	if got := fallback.GuideText(i18n.Ja); !strings.Contains(got, "パッケージマネージャ") {
		t.Errorf("GuideText(ja, fallback) = %q", got)
	}
	if got := (DepIssue{Kind: "freetype2"}).Title(i18n.Ja); got != "freetype2（Resonite のネイティブ依存）" {
		t.Errorf("Title(ja) = %q", got)
	}
	if got := (DepIssue{Kind: "dotnet10"}).Title(i18n.En); got != ".NET 10 runtime (required on ARM Linux)" {
		t.Errorf("Title(en) = %q", got)
	}
}

// --- CheckHeadlessDeps マトリクス ---

func TestCheckHeadlessDepsMatrix(t *testing.T) {
	kinds := func(issues []DepIssue) []string {
		var ks []string
		for _, i := range issues {
			ks = append(ks, i.Kind)
		}
		return ks
	}
	// freetype absent（ldconfig 成功・候補ゼロ）＋ os-release=arch の probe を組み立てる
	freetypeAbsentProbe := func() depProbe {
		p := emptyProbe()
		p.lookPath = func(name string) (string, error) {
			if name == "ldconfig" {
				return "/sbin/ldconfig", nil
			}
			return "", errors.New("not found")
		}
		p.runCmd = func(_ time.Duration, name string, _ ...string) (string, error) {
			if name == "/sbin/ldconfig" {
				return "0 libs found\n", nil
			}
			return "", errors.New("not found")
		}
		p.readFile = func(path string) ([]byte, error) {
			if path == "/etc/os-release" {
				return []byte("ID=cachyos\n"), nil
			}
			return nil, fs.ErrNotExist
		}
		return p
	}

	t.Run("windows は常に nil", func(t *testing.T) {
		if got := checkHeadlessDeps(freetypeAbsentProbe(), "windows", "amd64", "C:\\x"); got != nil {
			t.Errorf("windows で issue が出た: %v", got)
		}
	})
	t.Run("linux/amd64 は freetype のみ（dotnet はチェックしない）", func(t *testing.T) {
		got := checkHeadlessDeps(freetypeAbsentProbe(), "linux", "amd64", "/inst")
		if len(got) != 1 || got[0].Kind != "freetype2" {
			t.Fatalf("issues = %v, want [freetype2]", kinds(got))
		}
		if len(got[0].Commands) != 1 || got[0].Commands[0] != "sudo pacman -S freetype2" {
			t.Errorf("cachyos→pacman コマンドが出るべき: %v", got[0].Commands)
		}
	})
	t.Run("linux/arm64 は freetype＋dotnet の両方", func(t *testing.T) {
		got := checkHeadlessDeps(freetypeAbsentProbe(), "linux", "arm64", "/inst")
		if len(got) != 2 || got[0].Kind != "freetype2" || got[1].Kind != "dotnet10" {
			t.Fatalf("issues = %v, want [freetype2 dotnet10]", kinds(got))
		}
		if len(got[1].Commands) != 1 || !strings.Contains(got[1].Commands[0], "dotnet-install.sh") {
			t.Errorf("dotnet-install.sh コマンドが出るべき: %v", got[1].Commands)
		}
	})
	t.Run("linux/arm64 同梱 usable dotnet で dotnet issue 抑制（~ 展開込み）", func(t *testing.T) {
		p := freetypeAbsentProbe()
		// installDir="~/resonite" → home 展開後の同梱 dotnet が arm64
		bundled := "/home/test/resonite/dotnet-runtime/dotnet"
		p.stat = statOK(bundled)
		p.elfArch = archOf(map[string]string{bundled: "arm64"})
		got := checkHeadlessDeps(p, "linux", "arm64", "~/resonite")
		if len(got) != 1 || got[0].Kind != "freetype2" {
			t.Fatalf("issues = %v, want [freetype2]（dotnet は同梱で充足）", kinds(got))
		}
	})
	t.Run("freetype present＋dotnet present なら issue ゼロ", func(t *testing.T) {
		p := freetypeAbsentProbe()
		const soPath = "/usr/lib/libfreetype.so.6"
		const homeDotnet = "/home/test/.dotnet/dotnet"
		p.runCmd = func(_ time.Duration, name string, _ ...string) (string, error) {
			if name == "/sbin/ldconfig" {
				return "\tlibfreetype.so.6 (libc6,AArch64) => " + soPath + "\n", nil
			}
			return "Microsoft.NETCore.App 10.0.2 [/p]\n", nil
		}
		p.stat = statOK(soPath, homeDotnet)
		p.elfArch = archOf(map[string]string{soPath: "arm64", homeDotnet: "arm64"})
		if got := checkHeadlessDeps(p, "linux", "arm64", "/inst"); len(got) != 0 {
			t.Errorf("issues = %v, want []", kinds(got))
		}
	})
	t.Run("distro 不明なら freetype の Commands は空（汎用文言は呼び出し側）", func(t *testing.T) {
		p := freetypeAbsentProbe()
		p.readFile = func(string) ([]byte, error) { return nil, fs.ErrNotExist }
		got := checkHeadlessDeps(p, "linux", "amd64", "/inst")
		if len(got) != 1 || len(got[0].Commands) != 0 {
			t.Fatalf("issues = %v, Commands は空であるべき: %+v", kinds(got), got)
		}
	})
	t.Run("unknown（検出不能）は黙る＝issue ゼロ", func(t *testing.T) {
		if got := checkHeadlessDeps(emptyProbe(), "linux", "amd64", "/inst"); len(got) != 0 {
			t.Errorf("issues = %v, want []（案A）", kinds(got))
		}
	})
}
