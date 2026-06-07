// cleanup.go は過去の更新・中断が exe の隣に残した中間ファイルの掃除を担う（起動時に呼ぶ）。
package selfupdate

import (
	"os"
	"path/filepath"
	"time"
)

// CleanupStale は実行ファイルの隣に残った更新の残骸を best-effort で掃除する。
// 戻り値は「退避された旧バイナリ（.old*）が在った」＝直前の起動間に更新が適用されたか
// （呼び出し側が「vX に更新されました」の起動ログに使う）。
//
// 削除失敗は無視する（Windows では終了直後の旧プロセスが .old を掴んでいることがある。
// 次回起動で再掃除される）。
func CleanupStale() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return cleanupStale(exe)
}

func cleanupStale(exePath string) bool {
	updated := false
	// 退避済み旧バイナリ（mrhc.exe.old / mrhc.exe.old-<ts>）。
	// ユーザーが復旧のため .old を直接起動しているケースで自分自身を消さないよう、
	// 自分の実体と同一のファイルはスキップする。
	old, _ := filepath.Glob(exePath + ".old*")
	selfInfo, selfErr := os.Stat(exePath)
	for _, p := range old {
		if selfErr == nil {
			if fi, err := os.Stat(p); err == nil && os.SameFile(fi, selfInfo) {
				continue
			}
		}
		updated = true
		_ = os.Remove(p)
	}
	// 展開済みバイナリ・DL 途中のアーカイブ・ロックは、進行中の `mrhc update`（別プロセス。
	// extract 完了〜swap の間 .new が秒オーダーで存在する）を壊さないよう、
	// 古いものだけを残骸とみなして削除する。
	removeIfStale(exePath+".new", lockStaleAfter)
	removeIfStale(exePath+".update.lock", lockStaleAfter)
	partials, _ := filepath.Glob(filepath.Join(filepath.Dir(exePath), ".mrhc-update-*.partial"))
	for _, p := range partials {
		removeIfStale(p, lockStaleAfter)
	}
	return updated
}

func removeIfStale(path string, age time.Duration) {
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) >= age {
		_ = os.Remove(path)
	}
}
