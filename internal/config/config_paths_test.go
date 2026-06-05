package config

import (
	"path/filepath"
	"testing"
)

// TestInstallDirOrDefault は install 先導出の優先順（明示→ヘッドレスから導出→既定）を検証する（R-A）。
func TestInstallDirOrDefault(t *testing.T) {
	dataDir := filepath.Join("data", "mrhc")
	def := filepath.Join(dataDir, "resonite")
	headless := filepath.Join("root", "Resonite", "Headless", "Resonite.dll")
	fromHeadless := filepath.Join("root", "Resonite")

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"未設定は既定 {dataDir}/resonite", Config{}, def},
		{"Steam.InstallDir 明示が最優先", Config{Steam: &Steam{InstallDir: "/opt/Resonite"}}, "/opt/Resonite"},
		{"Steam.InstallDir 空白は無視", Config{Steam: &Steam{InstallDir: "   "}}, def},
		{"ResoniteHeadless から2つ上を導出", Config{ResoniteHeadless: headless}, fromHeadless},
		{"両方あれば InstallDir 優先", Config{ResoniteHeadless: headless, Steam: &Steam{InstallDir: "/opt/Resonite"}}, "/opt/Resonite"},
		// 非正規レイアウト（.../Headless/<bin> でない）の明示パスは 2つ上が install 根とズレる＝
		// 既知の制約（doc 明記）。更新先を確実にしたい場合は Steam.InstallDir を明示する。意図的挙動を固定。
		{"非正規パスは2つ上のまま(footgun・意図的)", Config{ResoniteHeadless: filepath.Join("only", "Resonite.exe")}, "."},
	}
	for _, c := range cases {
		if got := c.cfg.InstallDirOrDefault(dataDir); got != c.want {
			t.Errorf("%s: InstallDirOrDefault=%q want %q", c.name, got, c.want)
		}
	}
}

// TestHeadlessPathOrDefault はヘッドレスパス導出（明示優先・無ければ install 先から OS 名で組む）を検証する（R-A）。
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
	// ResoniteHeadless 明示 → そのまま（OS 名や install 先を無視）
	explicit := filepath.Join("custom", "path", "Resonite.exe")
	if got := (&Config{ResoniteHeadless: explicit}).HeadlessPathOrDefault(dataDir, bin); got != explicit {
		t.Errorf("明示パス: HeadlessPathOrDefault=%q want %q", got, explicit)
	}
}
