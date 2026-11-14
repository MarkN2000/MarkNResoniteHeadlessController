package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := WriteFileAtomic(path, []byte(`{"value":"old"}`)); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte(`{"value":"new"}`)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != `{"value":"new"}` {
		t.Fatalf("replacement: %s, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private permissions: %v, %v", info, err)
		}
	}
	// 置換できない宛先でも、既存の内容と一時ファイルの後始末を保つ。
	if err := WriteFileAtomic(dir, []byte("cannot replace directory")); err == nil {
		t.Fatal("replacing a directory succeeded")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != `{"value":"new"}` {
		t.Fatalf("failed replacement changed contents: %s, %v", got, err)
	}
	if leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(dir), ".mrhc-config-*.tmp")); err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary files remain: %v, %v", leftovers, err)
	}
}

// TestInstallDirOrDefault は install 先導出の優先順（明示 Steam.InstallDir → 既定）を検証する（R-A / 一本化）。
func TestInstallDirOrDefault(t *testing.T) {
	dataDir := filepath.Join("data", "mrhc")
	def := filepath.Join(dataDir, "resonite")

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"未設定は既定 {dataDir}/resonite", Config{}, def},
		{"Steam.InstallDir 明示が最優先", Config{Steam: &Steam{InstallDir: "/opt/Resonite"}}, "/opt/Resonite"},
		{"Steam.InstallDir 空白は無視", Config{Steam: &Steam{InstallDir: "   "}}, def},
	}
	for _, c := range cases {
		if got := c.cfg.InstallDirOrDefault(dataDir); got != c.want {
			t.Errorf("%s: InstallDirOrDefault=%q want %q", c.name, got, c.want)
		}
	}
}

// TestHeadlessPathOrDefault はヘッドレスパス導出（install 先から OS 名で組む）を検証する（R-A / 一本化）。
func TestHeadlessPathOrDefault(t *testing.T) {
	dataDir := filepath.Join("data", "mrhc")
	const bin = "Resonite.dll"

	// 未設定 → {dataDir}/resonite/Headless/Resonite.dll
	if got, want := (&Config{}).HeadlessPathOrDefault(dataDir, bin), filepath.Join(dataDir, "resonite", "Headless", bin); got != want {
		t.Errorf("未設定: HeadlessPathOrDefault=%q want %q", got, want)
	}
	// Steam.InstallDir 明示 → そこから導出
	if got, want := (&Config{Steam: &Steam{InstallDir: "/opt/Resonite"}}).HeadlessPathOrDefault(dataDir, bin), filepath.Join("/opt/Resonite", "Headless", bin); got != want {
		t.Errorf("InstallDir 導出: HeadlessPathOrDefault=%q want %q", got, want)
	}
}
