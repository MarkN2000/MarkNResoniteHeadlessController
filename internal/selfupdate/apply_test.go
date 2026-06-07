package selfupdate

import (
	"bytes"
	"context"
	"debug/elf"
	"debug/pe"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// linuxArchive は標準エントリ名（mrhc-linux-amd64/mrhc）で body を包んだ tar.gz を返す。
func linuxArchive(t *testing.T, body []byte) []byte {
	t.Helper()
	return makeTarGz(t, []tarEntry{
		{name: "mrhc-linux-amd64/mrhc", body: body},
		{name: "mrhc-linux-amd64/README.md", body: []byte("readme")},
	})
}

// setupLinuxApply は linux/amd64・現行 v2.0.0 の標準シナリオを組み立てる。
// extra は配信ファイルの追加・上書き（"SHA256SUMS" を入れると自動生成を上書き＝改竄テスト）。
func setupLinuxApply(t *testing.T, tag string, archive []byte, extra map[string][]byte) (*Updater, string) {
	t.Helper()
	setPlatform(t, "linux/amd64")
	dir := t.TempDir()
	exe := filepath.Join(dir, "mrhc")
	writeExe(t, exe, fakeELF(elf.EM_X86_64, "current-binary"))
	files := map[string][]byte{"mrhc-linux-amd64.tar.gz": archive}
	for k, v := range extra {
		files[k] = v
	}
	srv := serveRelease(t, tag, files)
	return newTestUpdater(srv.URL, "v2.0.0", exe), exe
}

// assertUntouched は失敗系の後で「現状維持・残骸なし」を検証する。
func assertUntouched(t *testing.T, exe string, wantBody []byte) {
	t.Helper()
	if got := readFile(t, exe); !bytes.Equal(got, wantBody) {
		t.Error("失敗時に実行ファイルが変更されています")
	}
	mustNotExist(t, exe+".new")
	mustNotExist(t, exe+".old")
	mustNotExist(t, exe+".update.lock")
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".mrhc-update-*.partial")); len(leftovers) > 0 {
		t.Errorf("DL 一時ファイルが残っています: %v", leftovers)
	}
}

func TestApplyLinux(t *testing.T) {
	newBody := fakeELF(elf.EM_X86_64, "new-binary")
	u, exe := setupLinuxApply(t, "v2.1.0", linuxArchive(t, newBody), nil)

	got, err := u.Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "v2.1.0" {
		t.Errorf("staged = %q, want v2.1.0", got)
	}
	if !bytes.Equal(readFile(t, exe), newBody) {
		t.Error("実行ファイルが新版に入れ替わっていません")
	}
	if !bytes.Equal(readFile(t, exe+".old"), fakeELF(elf.EM_X86_64, "current-binary")) {
		t.Error(".old が旧版になっていません")
	}
	mustNotExist(t, exe+".new")
	mustNotExist(t, exe+".update.lock")
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".mrhc-update-*.partial")); len(leftovers) > 0 {
		t.Errorf("DL 一時ファイルが残っています: %v", leftovers)
	}
}

func TestApplyWindows(t *testing.T) {
	setPlatform(t, "windows/amd64")
	dir := t.TempDir()
	exe := filepath.Join(dir, "mrhc.exe")
	writeExe(t, exe, fakePE(uint16(pe.IMAGE_FILE_MACHINE_AMD64), "current-binary"))
	newBody := fakePE(uint16(pe.IMAGE_FILE_MACHINE_AMD64), "new-binary")
	archive := makeZip(t, []zipEntry{
		{name: "mrhc-windows-amd64/mrhc.exe", body: newBody},
		{name: "mrhc-windows-amd64/LICENSE", body: []byte("license")},
	})
	srv := serveRelease(t, "v2.1.0", map[string][]byte{"mrhc-windows-amd64.zip": archive})
	u := newTestUpdater(srv.URL, "v2.0.0", exe)

	if _, err := u.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readFile(t, exe), newBody) {
		t.Error("実行ファイルが新版に入れ替わっていません")
	}
}

// "./" 接頭辞付きエントリ名（tar の作り方による揺らぎ）も正規化して一致する。
func TestApplyDotSlashEntry(t *testing.T) {
	newBody := fakeELF(elf.EM_X86_64, "new-binary")
	archive := makeTarGz(t, []tarEntry{{name: "./mrhc-linux-amd64/mrhc", body: newBody}})
	u, exe := setupLinuxApply(t, "v2.1.0", archive, nil)
	if _, err := u.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readFile(t, exe), newBody) {
		t.Error("実行ファイルが新版に入れ替わっていません")
	}
}

func TestApplyShaMismatch(t *testing.T) {
	archive := linuxArchive(t, fakeELF(elf.EM_X86_64, "new-binary"))
	tampered := strings.Repeat("0", 64) + "  mrhc-linux-amd64.tar.gz\n"
	u, exe := setupLinuxApply(t, "v2.1.0", archive, map[string][]byte{"SHA256SUMS": []byte(tampered)})

	_, err := u.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("err = %v, want SHA-256 不一致", err)
	}
	assertUntouched(t, exe, fakeELF(elf.EM_X86_64, "current-binary"))
}

func TestApplyUpToDate(t *testing.T) {
	u, exe := setupLinuxApply(t, "v2.0.0", nil, nil)
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrUpToDate) {
		t.Errorf("err = %v, want ErrUpToDate", err)
	}
	assertUntouched(t, exe, fakeELF(elf.EM_X86_64, "current-binary"))
}

func TestApplyNotReleaseBuild(t *testing.T) {
	u, _ := setupLinuxApply(t, "v2.1.0", nil, nil)
	u.Version = "dev"
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrNotReleaseBuild) {
		t.Errorf("err = %v, want ErrNotReleaseBuild", err)
	}
}

func TestApplyUnsupportedPlatform(t *testing.T) {
	u, _ := setupLinuxApply(t, "v2.1.0", nil, nil)
	setPlatform(t, "plan9/386")
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("err = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestApplyBusyLock(t *testing.T) {
	u, exe := setupLinuxApply(t, "v2.1.0", linuxArchive(t, fakeELF(elf.EM_X86_64, "new-binary")), nil)
	lock := exe + ".update.lock"
	writeExe(t, lock, []byte("123\n"))

	// 新しいロック＝進行中の別更新 → ErrBusy
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	// 古いロック＝中断の残骸 → 除去して続行できる
	staleAt := time.Now().Add(-2 * lockStaleAfter)
	if err := os.Chtimes(lock, staleAt, staleAt); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Apply(context.Background()); err != nil {
		t.Fatalf("stale ロックを越えて適用できるべき: %v", err)
	}
	mustNotExist(t, lock)
}

// 再起動せず2回目の更新（.old が実行中イメージ等で削除できない）でも一意名へ退避して成功する。
// 「削除できない .old」は非空ディレクトリで模擬する（os.Remove が全OSで失敗する）。
func TestApplyOldUndeletable(t *testing.T) {
	newBody := fakeELF(elf.EM_X86_64, "new-binary")
	u, exe := setupLinuxApply(t, "v2.1.0", linuxArchive(t, newBody), nil)
	if err := os.MkdirAll(filepath.Join(exe+".old", "keep"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := u.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readFile(t, exe), newBody) {
		t.Error("実行ファイルが新版に入れ替わっていません")
	}
	if fi, err := os.Stat(exe + ".old"); err != nil || !fi.IsDir() {
		t.Error("削除できない .old が壊されています")
	}
	uniques, _ := filepath.Glob(exe + ".old-*")
	if len(uniques) != 1 {
		t.Errorf("一意名の退避が %d 個（want 1）: %v", len(uniques), uniques)
	}
}

// アーカイブ内の該当エントリが symlink 等の非通常ファイルなら拒否する。
func TestApplyRejectsSymlinkEntry(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{
		{name: "mrhc-linux-amd64/mrhc", typ: '2' /* tar.TypeSymlink */, link: "/usr/bin/true"},
	})
	u, exe := setupLinuxApply(t, "v2.1.0", archive, nil)
	_, err := u.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "通常ファイル") {
		t.Fatalf("err = %v, want 非通常ファイル拒否", err)
	}
	assertUntouched(t, exe, fakeELF(elf.EM_X86_64, "current-binary"))
}

// 取り出したバイナリの形式・アーキテクチャが合わなければ入れ替え前に拒否する。
func TestApplyRejectsWrongBinary(t *testing.T) {
	cases := map[string][]byte{
		"別アーキのELF":  fakeELF(elf.EM_AARCH64, "arm-binary"),
		"実行ファイル以外": []byte("#!/bin/sh\necho not a binary\n"),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			u, exe := setupLinuxApply(t, "v2.1.0", linuxArchive(t, body), nil)
			if _, err := u.Apply(context.Background()); err == nil {
				t.Fatal("err = nil, want 形式検査エラー")
			}
			assertUntouched(t, exe, fakeELF(elf.EM_X86_64, "current-binary"))
		})
	}
}

// zip の symlink エントリ（Open がリンク先文字列を返す）も拒否する。
func TestExtractZipRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "a.zip")
	writeExe(t, arc, makeZip(t, []zipEntry{
		{name: "mrhc-windows-amd64/mrhc.exe", body: []byte("C:\\evil"), symlink: true},
	}))
	err := extractZipBinary(arc, "mrhc-windows-amd64/mrhc.exe", filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "通常ファイル") {
		t.Fatalf("err = %v, want 非通常ファイル拒否", err)
	}
}
