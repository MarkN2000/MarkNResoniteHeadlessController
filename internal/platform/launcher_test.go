package platform

import (
	"path/filepath"
	"testing"
)

// configArg は cmd.Args 中の -HeadlessConfig の次要素（設定パス）を返す。無ければ ""。
func configArg(args []string) string {
	for i, a := range args {
		if a == "-HeadlessConfig" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// 相対パスで渡しても -HeadlessConfig と cmd.Dir は絶対になること。
// headless の cwd は Headless フォルダに変わるため、相対のままだと headless 側で解決先が
// ズレて "Config file not found!" で即終了する（回帰防止）。分岐は GOOS ではなく拡張子で
// 決まるので、.exe(Windows) と .dll(Linux) の両方を同一 OS 上で検証する。
func TestBuildHeadlessCommandMakesPathsAbsolute(t *testing.T) {
	relCfg := filepath.Join("data", ".run", "default.json")
	cases := []struct {
		name         string
		headlessPath string
	}{
		{"windows exe", filepath.Join("rel", "Headless", "Resonite.exe")},
		{"linux dll", filepath.Join("rel", "Headless", "Resonite.dll")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := BuildHeadlessCommand(tc.headlessPath, relCfg)

			cfg := configArg(cmd.Args)
			if cfg == "" {
				t.Fatalf("cmd.Args に -HeadlessConfig が無い: %v", cmd.Args)
			}
			if !filepath.IsAbs(cfg) {
				t.Errorf("-HeadlessConfig が絶対パスでない: %q (args=%v)", cfg, cmd.Args)
			}
			if !filepath.IsAbs(cmd.Dir) {
				t.Errorf("cmd.Dir が絶対パスでない: %q", cmd.Dir)
			}
		})
	}
}

// configPath が空なら -HeadlessConfig を付けない（無config起動は backend が別途弾く）。
func TestBuildHeadlessCommandNoConfig(t *testing.T) {
	cmd := BuildHeadlessCommand(filepath.Join("rel", "Headless", "Resonite.exe"), "")
	if cfg := configArg(cmd.Args); cfg != "" {
		t.Errorf("configPath 空なのに -HeadlessConfig が付いた: %v", cmd.Args)
	}
}
