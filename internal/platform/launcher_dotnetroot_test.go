package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installFixture は <root>/Headless/Resonite.exe＋runtimeconfig と（任意で）ローカルランタイムを作る。
// 戻り値は headlessPath。
func installFixture(t *testing.T, withRuntimeConfig bool, runtimeVersions ...string) (root, headlessPath string) {
	t.Helper()
	root = t.TempDir()
	headlessDir := filepath.Join(root, "Headless")
	if err := os.MkdirAll(headlessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	headlessPath = filepath.Join(headlessDir, "Resonite.exe")
	if err := os.WriteFile(headlessPath, []byte("exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if withRuntimeConfig {
		writeRuntimeConfig(t, headlessDir,
			`{"runtimeOptions":{"tfm":"net10.0","framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}`)
	}
	if len(runtimeVersions) > 0 {
		// host は非 ELF（Windows の dotnet.exe 相当）＝ arch 判定は楽観で通る
		installRuntimeFixture(t, root, []byte("host"), runtimeVersions...)
	}
	return root, headlessPath
}

// dotnetRootEnv は cmd.Env から DOTNET_ROOT の値を取り出す（無ければ ""・env 未設定なら not set）。
func dotnetRootEnv(env []string) (string, bool) {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "DOTNET_ROOT="); ok {
			return v, true
		}
	}
	return "", false
}

func TestBuildHeadlessCommandDotnetRootSatisfied(t *testing.T) {
	root, headlessPath := installFixture(t, true, "10.0.8")
	cmd := BuildHeadlessCommand(headlessPath, "")

	if cmd.Env == nil {
		t.Fatal("ローカルランタイム充足時は DOTNET_ROOT を設定すべき（Env が nil）")
	}
	got, ok := dotnetRootEnv(cmd.Env)
	want := filepath.Join(root, "dotnet-runtime")
	// filepath.Abs 適用後の root と比較（テストの TempDir は絶対パスなのでそのまま一致する）
	if !ok || got != want {
		t.Errorf("DOTNET_ROOT = %q (set=%v), want %q", got, ok, want)
	}
}

func TestBuildHeadlessCommandDotnetRootNotSatisfied(t *testing.T) {
	cases := []struct {
		name              string
		withRuntimeConfig bool
		versions          []string
	}{
		{"ローカルランタイム不在", true, nil},
		{"版不足（9.x のみ）", true, []string{"9.0.11"}},
		{"runtimeconfig 無し（fakehl 等）", false, []string{"10.0.8"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, headlessPath := installFixture(t, tc.withRuntimeConfig, tc.versions...)
			cmd := BuildHeadlessCommand(headlessPath, "")
			if cmd.Env != nil {
				if v, ok := dotnetRootEnv(cmd.Env); ok {
					t.Errorf("DOTNET_ROOT を設定すべきでない（=%q）。システム .NET の従来挙動を変えてしまう", v)
				}
			}
		})
	}
}

// TestResolveDotnetStaleLocal は「ローカルが在るが要求版を満たさない」場合にシステムへ
// フォールバックすることを確認する（設置失敗で stale が残ったケース）。
func TestResolveDotnetStaleLocal(t *testing.T) {
	root := t.TempDir()
	headlessDir := filepath.Join(root, "Headless")
	writeRuntimeConfig(t, headlessDir,
		`{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}`)
	installRuntimeFixture(t, root, []byte("host"), "9.0.11") // 旧版のみ＝不充足

	if got := resolveDotnet(headlessDir, runtime.GOARCH); got != systemDotnet() {
		t.Errorf("stale ローカルはシステムへフォールバックすべき: got %q", got)
	}
}

// TestResolveDotnetLocalSatisfied は充足するローカルが採用されることを確認する。
func TestResolveDotnetLocalSatisfied(t *testing.T) {
	root := t.TempDir()
	headlessDir := filepath.Join(root, "Headless")
	writeRuntimeConfig(t, headlessDir,
		`{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}`)
	installRuntimeFixture(t, root, []byte("host"), "10.0.8")

	want := filepath.Join(root, "dotnet-runtime", "dotnet")
	if got := resolveDotnet(headlessDir, runtime.GOARCH); got != want {
		t.Errorf("充足するローカルを採用すべき: got %q want %q", got, want)
	}
}

// TestResolveDotnetNoRuntimeConfigKeepsLegacy は runtimeconfig が無い環境で従来挙動
// （ローカル在＝採用）が変わらないことを確認する。
func TestResolveDotnetNoRuntimeConfigKeepsLegacy(t *testing.T) {
	root := t.TempDir()
	headlessDir := filepath.Join(root, "Headless")
	if err := os.MkdirAll(headlessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	installRuntimeFixture(t, root, []byte("host")) // shared 無しでも従来は host 存在だけで採用

	want := filepath.Join(root, "dotnet-runtime", "dotnet")
	if got := resolveDotnet(headlessDir, runtime.GOARCH); got != want {
		t.Errorf("runtimeconfig 無しは従来どおりローカル採用すべき: got %q want %q", got, want)
	}
}
