package platform

import (
	"encoding/binary"
	"os"
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

// writeTempFile はテスト用の一時ファイルを書き、そのパスを返す。
func writeTempFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dotnet")
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatalf("一時ファイル書き込み失敗: %v", err)
	}
	return path
}

// elfHeader は e_machine と EI_DATA(1=LE/2=BE) を指定した最小(20バイト) ELF64 ヘッダを組み立てる。
// elfArch は先頭 20 バイトしか読まないため section/program ヘッダは不要。
// e_machine は EI_DATA のエンディアンで書き込み、BE 読み取り経路も検証できるようにする。
func elfHeader(machine uint16, eiData byte) []byte {
	hdr := make([]byte, 20)
	copy(hdr[:4], []byte{0x7f, 'E', 'L', 'F'})
	hdr[4] = 2      // EI_CLASS   = ELFCLASS64
	hdr[5] = eiData // EI_DATA    = 1:LE / 2:BE
	hdr[6] = 1      // EI_VERSION = EV_CURRENT
	bo := binary.ByteOrder(binary.LittleEndian)
	if eiData == 2 {
		bo = binary.BigEndian
	}
	bo.PutUint16(hdr[18:], machine)
	return hdr
}

// writeELF は LE の ELF64 ヘッダ(e_machine 指定)を一時ファイルに書く。
func writeELF(t *testing.T, machine uint16) string {
	t.Helper()
	return writeTempFile(t, elfHeader(machine, 1))
}

// elfArch は e_machine を GOARCH 表記へ対応づける。誤読は x64 の起動挙動を変えうるため直接固定する。
// LE/BE 両エンディアン、不正 EI_DATA、ELFCLASS32（e_machine は offset 18 で不変）も網羅する。
func TestElfArch(t *testing.T) {
	// elfClass32 は ELFCLASS64 ヘッダの EI_CLASS を 32bit に書き換える（offset 18 不変の確認用）。
	elfClass32 := func(machine uint16) []byte {
		h := elfHeader(machine, 1)
		h[4] = 1 // EI_CLASS = ELFCLASS32
		return h
	}
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"x86_64 LE", elfHeader(62, 1), "amd64"},
		{"aarch64 LE", elfHeader(183, 1), "arm64"},
		{"x86_64 BE", elfHeader(62, 2), "amd64"},   // BigEndian 読み取り経路
		{"aarch64 BE", elfHeader(183, 2), "arm64"}, // BigEndian 読み取り経路
		{"i386 (EM_386)", elfHeader(3, 1), "386"},  // 32bit x86（Fedora /usr/lib 対策・R-C）
		{"arm32 (EM_ARM)", elfHeader(40, 1), "arm"},
		{"unknown machine (EM_RISCV)", elfHeader(243, 1), ""}, // マップ外は判定不能扱い
		{"invalid EI_DATA", elfHeader(62, 0), ""},             // EI_DATA=0 は判定不能
		{"x86_64 ELFCLASS32", elfClass32(62), "amd64"},        // 32bit でも e_machine は offset 18
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := elfArch(writeTempFile(t, tc.data)); got != tc.want {
				t.Errorf("elfArch(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// dotnetUsable は arch が判明し goarch と食い違う時だけ false。判定不能(ELF以外/読めない)は楽観 true。
// これにより ARM では x64 同梱を弾き（systemDotnet へ）、x64/Windows/mac は従来どおり同梱を採用する。
func TestDotnetUsable(t *testing.T) {
	amd64ELF := writeELF(t, 62)  // EM_X86_64
	arm64ELF := writeELF(t, 183) // EM_AARCH64
	nonELF := writeTempFile(t, []byte("#!/bin/sh\necho hi\n"))
	tooShort := writeTempFile(t, []byte{0x7f, 'E'}) // 20バイト未満
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	cases := []struct {
		name   string
		path   string
		goarch string
		want   bool
	}{
		{"amd64 同梱 × amd64 実行", amd64ELF, "amd64", true},
		{"amd64 同梱 × arm64 実行（ARMで弾く中核）", amd64ELF, "arm64", false},
		{"arm64 同梱 × arm64 実行（将来のARM同梱）", arm64ELF, "arm64", true},
		{"arm64 同梱 × amd64 実行", arm64ELF, "amd64", false},
		{"非ELF は楽観採用", nonELF, "arm64", true},
		{"短すぎは楽観採用", tooShort, "arm64", true},
		{"存在しないパスは楽観採用", missing, "arm64", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dotnetUsable(tc.path, tc.goarch); got != tc.want {
				t.Errorf("dotnetUsable(%q, %q) = %v, want %v", tc.path, tc.goarch, got, tc.want)
			}
		})
	}
}

// resolveDotnet の中核結線（同梱が arch 一致なら採用、不一致なら system へ）を統合検証する。
// 同梱に x64(amd64) ELF を置き、実行 arch を振って分岐を確かめる。
func TestResolveDotnetArchGate(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "Resonite", "dotnet-runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("dotnet-runtime 作成失敗: %v", err)
	}
	bundled := filepath.Join(runtimeDir, "dotnet")
	if err := os.WriteFile(bundled, elfHeader(62, 1), 0o755); err != nil { // x64 同梱
		t.Fatalf("同梱 dotnet 書き込み失敗: %v", err)
	}
	headlessDir := filepath.Join(root, "Resonite", "Headless")

	// 実行 arch=amd64: 同梱(x64)と一致 → 同梱を採用
	if got := resolveDotnet(headlessDir, "amd64"); got != bundled {
		t.Errorf("amd64: 同梱を採用すべき: got %q, want %q", got, bundled)
	}
	// 実行 arch=arm64: 同梱(x64)と不一致 → 同梱を弾き systemDotnet へ（同梱パスは返さない）
	if got := resolveDotnet(headlessDir, "arm64"); got == bundled {
		t.Errorf("arm64: x64 同梱を弾くべきなのに採用した: %q", got)
	}
}
