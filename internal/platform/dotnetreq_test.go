package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeRuntimeConfig は headlessDir に Resonite.runtimeconfig.json を書く。
func writeRuntimeConfig(t *testing.T, headlessDir, content string) {
	t.Helper()
	if err := os.MkdirAll(headlessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(headlessDir, runtimeConfigName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadRuntimeRequirement(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    RuntimeRequirement
		wantOK  bool
	}{
		{
			"framework 単数（実物の形式）",
			`{"runtimeOptions":{"tfm":"net10.0","framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}`,
			RuntimeRequirement{10, 0, 0, "10.0.0"}, true,
		},
		{
			"frameworks 配列",
			`{"runtimeOptions":{"frameworks":[{"name":"Microsoft.AspNetCore.App","version":"10.0.0"},{"name":"Microsoft.NETCore.App","version":"10.0.3"}]}}`,
			RuntimeRequirement{10, 0, 3, "10.0.3"}, true,
		},
		{
			"NETCore.App が無い",
			`{"runtimeOptions":{"framework":{"name":"Microsoft.WindowsDesktop.App","version":"10.0.0"}}}`,
			RuntimeRequirement{}, false,
		},
		{"壊れた JSON", `{"runtimeOptions":`, RuntimeRequirement{}, false},
		{"runtimeOptions 欠落", `{}`, RuntimeRequirement{}, false},
		{
			"版がパース不能",
			`{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"latest"}}}`,
			RuntimeRequirement{}, false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRuntimeConfig(t, dir, tc.content)
			got, ok := ReadRuntimeRequirement(dir)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("ReadRuntimeRequirement = %+v, %v / want %+v, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}

	t.Run("ファイル無し", func(t *testing.T) {
		if _, ok := ReadRuntimeRequirement(t.TempDir()); ok {
			t.Error("ファイル無しで ok=true")
		}
	})
}

func TestRuntimeRequirementChannel(t *testing.T) {
	if got := (RuntimeRequirement{Major: 10, Minor: 0, Patch: 8}).Channel(); got != "10.0" {
		t.Errorf("Channel() = %q, want %q", got, "10.0")
	}
}

func TestParseVersionTriple(t *testing.T) {
	cases := []struct {
		in            string
		maj, min, pat int
		pre, ok       bool
	}{
		{"10.0.8", 10, 0, 8, false, true},
		{"10.0", 10, 0, 0, false, true},
		{"11.0.0-preview.4.26230.115", 11, 0, 0, true, true},
		{"10.0.8+abc", 10, 0, 8, true, true},
		{"latest", 0, 0, 0, false, false},
		{"10", 0, 0, 0, false, false},
		{"10.0.8.1", 0, 0, 0, false, false},
		{"10.x.0", 0, 0, 0, false, false},
		{"", 0, 0, 0, false, false},
	}
	for _, tc := range cases {
		maj, min, pat, pre, ok := parseVersionTriple(tc.in)
		if maj != tc.maj || min != tc.min || pat != tc.pat || pre != tc.pre || ok != tc.ok {
			t.Errorf("parseVersionTriple(%q) = %d,%d,%d,%v,%v / want %d,%d,%d,%v,%v",
				tc.in, maj, min, pat, pre, ok, tc.maj, tc.min, tc.pat, tc.pre, tc.ok)
		}
	}
}

// installRuntimeFixture は <dir>/dotnet-runtime に host と shared 版ディレクトリを作る。
// hostData が nil なら host を作らない。
func installRuntimeFixture(t *testing.T, dir string, hostData []byte, versions ...string) {
	t.Helper()
	rt := filepath.Join(dir, "dotnet-runtime")
	if hostData != nil {
		if err := os.MkdirAll(rt, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rt, "dotnet"), hostData, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range versions {
		if err := os.MkdirAll(filepath.Join(rt, "shared", netCoreAppFramework, v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocalRuntimeSatisfies(t *testing.T) {
	req := RuntimeRequirement{Major: 10, Minor: 0, Patch: 0, Raw: "10.0.0"}
	x64Host := elfHeader(62, 1)    // EM_X86_64
	arm64Host := elfHeader(183, 1) // EM_AARCH64

	cases := []struct {
		name     string
		hostData []byte
		versions []string
		req      RuntimeRequirement
		goarch   string
		want     bool
	}{
		{"同版あり", x64Host, []string{"10.0.0"}, req, "amd64", true},
		{"patch 上位あり", x64Host, []string{"10.0.8"}, req, "amd64", true},
		{"minor 上位あり（roll-forward Minor）", x64Host, []string{"10.1.0"}, req, "amd64", true},
		{"patch 不足", x64Host, []string{"10.0.2"}, RuntimeRequirement{10, 0, 5, "10.0.5"}, "amd64", false},
		{"major 下位のみ", x64Host, []string{"9.0.11"}, req, "amd64", false},
		{"major 上位のみ", x64Host, []string{"11.0.0"}, req, "amd64", false},
		{"prerelease のみ＝不充足", x64Host, []string{"10.0.0-preview.4.26230.115"}, req, "amd64", false},
		{"prerelease と確定版の併存", x64Host, []string{"10.0.0-rc.1", "10.0.8"}, req, "amd64", true},
		{"host 欠落", nil, []string{"10.0.8"}, req, "amd64", false},
		{"shared 欠落", x64Host, nil, req, "amd64", false},
		{"host の arch 不一致（ARM 機に x64 残骸）", x64Host, []string{"10.0.8"}, req, "arm64", false},
		{"host が ELF でない（Windows zip 展開）は楽観", []byte("MZ not elf"), []string{"10.0.8"}, req, "amd64", true},
		{"arm64 host + arm64 実行", arm64Host, []string{"10.0.8"}, req, "arm64", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			installRuntimeFixture(t, dir, tc.hostData, tc.versions...)
			if got := LocalRuntimeSatisfies(dir, tc.req, tc.goarch); got != tc.want {
				t.Errorf("LocalRuntimeSatisfies = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("dotnet-runtime 自体が無い", func(t *testing.T) {
		if LocalRuntimeSatisfies(t.TempDir(), req, "amd64") {
			t.Error("空ディレクトリで true")
		}
	})
}

func TestListedRuntimeSatisfies(t *testing.T) {
	req := RuntimeRequirement{Major: 10, Minor: 0, Patch: 0, Raw: "10.0.0"}
	cases := []struct {
		name string
		out  string
		want bool
	}{
		{"10.x あり", "Microsoft.NETCore.App 10.0.8 [/usr/share/dotnet]\n", true},
		{"9.x のみ", "Microsoft.NETCore.App 9.0.11 [/usr/share/dotnet]\n", false},
		{"AspNetCore のみ", "Microsoft.AspNetCore.App 10.0.8 [/usr/share/dotnet]\n", false},
		{"prerelease のみ", "Microsoft.NETCore.App 10.0.0-preview.4 [/x]\n", false},
		{"複数行で後方に一致", "Microsoft.NETCore.App 8.0.1 [/x]\nMicrosoft.NETCore.App 10.0.1 [/x]\n", true},
		{"空出力", "", false},
	}
	for _, tc := range cases {
		if got := listedRuntimeSatisfies(tc.out, req); got != tc.want {
			t.Errorf("%s: listedRuntimeSatisfies = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// sysProbe は systemRuntimeSatisfies 用の偽 probe を組み立てる。
func sysProbe(home string, statOK map[string]bool, lookPathResult string, runOut string, runErr error) depProbe {
	return depProbe{
		lookPath: func(name string) (string, error) {
			if lookPathResult == "" {
				return "", errors.New("not found")
			}
			return lookPathResult, nil
		},
		stat: func(p string) (os.FileInfo, error) {
			if statOK[filepath.ToSlash(p)] {
				return nil, nil
			}
			return nil, os.ErrNotExist
		},
		runCmd: func(timeout time.Duration, name string, args ...string) (string, error) {
			return runOut, runErr
		},
		elfArch: func(string) string { return "" },
		home:    home,
	}
}

func TestSystemRuntimeSatisfies(t *testing.T) {
	req := RuntimeRequirement{Major: 10, Minor: 0, Patch: 0, Raw: "10.0.0"}
	listed := "Microsoft.NETCore.App 10.0.8 [/x]\n"

	t.Run("linux: ~/.dotnet 充足", func(t *testing.T) {
		p := sysProbe("/home/u", map[string]bool{"/home/u/.dotnet/dotnet": true}, "", listed, nil)
		if !systemRuntimeSatisfies(p, "linux", "amd64", req, "") {
			t.Error("want true")
		}
	})
	t.Run("linux: PATH フォールバック充足", func(t *testing.T) {
		p := sysProbe("/home/u", nil, "/usr/bin/dotnet", listed, nil)
		if !systemRuntimeSatisfies(p, "linux", "amd64", req, "") {
			t.Error("want true")
		}
	})
	t.Run("linux: 候補なし", func(t *testing.T) {
		p := sysProbe("/home/u", nil, "", "", nil)
		if systemRuntimeSatisfies(p, "linux", "amd64", req, "") {
			t.Error("want false")
		}
	})
	t.Run("linux: list-runtimes 失敗＝unknown は false", func(t *testing.T) {
		p := sysProbe("/home/u", map[string]bool{"/home/u/.dotnet/dotnet": true}, "", "", errors.New("timeout"))
		if systemRuntimeSatisfies(p, "linux", "amd64", req, "") {
			t.Error("want false")
		}
	})
	t.Run("linux: 版不足", func(t *testing.T) {
		p := sysProbe("/home/u", map[string]bool{"/home/u/.dotnet/dotnet": true}, "", "Microsoft.NETCore.App 9.0.1 [/x]\n", nil)
		if systemRuntimeSatisfies(p, "linux", "amd64", req, "") {
			t.Error("want false")
		}
	})
	t.Run("linux: arch 不一致候補は不採用", func(t *testing.T) {
		p := sysProbe("/home/u", map[string]bool{"/home/u/.dotnet/dotnet": true}, "", listed, nil)
		p.elfArch = func(string) string { return "amd64" } // arm64 実行に対し x64 dotnet
		if systemRuntimeSatisfies(p, "linux", "arm64", req, "") {
			t.Error("want false")
		}
	})
	t.Run("windows: PATH 充足", func(t *testing.T) {
		p := sysProbe("", nil, `C:\Program Files\dotnet\dotnet.exe`, listed, nil)
		if !systemRuntimeSatisfies(p, "windows", "amd64", req, `C:\Program Files`) {
			t.Error("want true")
		}
	})
	t.Run("windows: ProgramFiles フォールバック充足", func(t *testing.T) {
		p := sysProbe("", map[string]bool{"C:/Program Files/dotnet/dotnet.exe": true}, "", listed, nil)
		if !systemRuntimeSatisfies(p, "windows", "amd64", req, `C:\Program Files`) {
			t.Error("want true")
		}
	})
	t.Run("windows: 候補なし", func(t *testing.T) {
		p := sysProbe("", nil, "", "", nil)
		if systemRuntimeSatisfies(p, "windows", "amd64", req, `C:\Program Files`) {
			t.Error("want false")
		}
	})
}
