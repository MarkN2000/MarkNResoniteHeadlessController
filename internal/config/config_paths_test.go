package config

import (
	"path/filepath"
	"testing"
)

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
