package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupStale(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mrhc")
	writeExe(t, exe, []byte("current"))
	// 更新の残骸（消えるべきもの）
	writeExe(t, exe+".old", []byte("old"))
	writeExe(t, exe+".old-1700000000", []byte("older"))
	staleNew := exe + ".new"
	writeExe(t, staleNew, []byte("staged"))
	stalePartial := filepath.Join(dir, ".mrhc-update-1.partial")
	writeExe(t, stalePartial, []byte("x"))
	staleLock := exe + ".update.lock"
	writeExe(t, staleLock, []byte("123\n"))
	staleAt := time.Now().Add(-2 * lockStaleAfter)
	for _, p := range []string{staleNew, stalePartial, staleLock} {
		if err := os.Chtimes(p, staleAt, staleAt); err != nil {
			t.Fatal(err)
		}
	}
	// 進行中の別プロセスの作業ファイル（新しい＝消えてはいけないもの）
	freshPartial := filepath.Join(dir, ".mrhc-update-2.partial")
	writeExe(t, freshPartial, []byte("y"))

	if !cleanupStale(exe) {
		t.Error("updated = false, want true（.old があった）")
	}
	for _, p := range []string{exe + ".old", exe + ".old-1700000000", staleNew, stalePartial, staleLock} {
		mustNotExist(t, p)
	}
	if _, err := os.Stat(freshPartial); err != nil {
		t.Error("進行中の .partial が消されています")
	}
	if _, err := os.Stat(exe); err != nil {
		t.Error("実行ファイル本体が消されています")
	}
}

// 進行中の `mrhc update`（extract 完了〜swap の間）が書いた新しい .new は消さない。
func TestCleanupStaleKeepsFreshNew(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mrhc")
	writeExe(t, exe, []byte("current"))
	writeExe(t, exe+".new", []byte("staging-in-progress"))
	if cleanupStale(exe) {
		t.Error("updated = true, want false")
	}
	if _, err := os.Stat(exe + ".new"); err != nil {
		t.Error("進行中の .new が消されています")
	}
}

func TestCleanupStaleNothing(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mrhc")
	writeExe(t, exe, []byte("current"))
	if cleanupStale(exe) {
		t.Error("updated = true, want false（残骸なし）")
	}
}

// 復旧のため .old を直接起動しているケースでは自分自身を消さない。
// exePath と .old が同一ファイル（ハードリンク）でも SameFile ガードで保護される。
func TestCleanupStaleSelfGuard(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mrhc")
	writeExe(t, exe, []byte("current"))
	if err := os.Link(exe, exe+".old"); err != nil {
		t.Skipf("ハードリンク不可の環境: %v", err)
	}
	if cleanupStale(exe) {
		t.Error("updated = true, want false（自分自身はスキップ）")
	}
	if _, err := os.Stat(exe + ".old"); err != nil {
		t.Error("自分自身と同一の .old が消されています")
	}
}
