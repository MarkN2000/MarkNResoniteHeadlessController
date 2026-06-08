package server

// キャッシュ管理（手動全削除 + 停止時の自動「古いファイル削除」）。
// 対象は既定キャッシュ {dataDir}/headless-cache のみ（hlconfig.DefaultFolders と同一導出＝
// コンフィグに焼き込む値と一致。独自 cacheFolder を設定した場合は対象外＝既定運用前提）。
//
// 安全原則: 削除はヘッドレスが停止中のときだけ行う。
//   - 手動全削除: State!=stopped は 409。
//   - 自動古削除: driver.SetOnStopped 経由（プロセス reap 済み＝キャッシュ未使用が保証される）。
// 削除コア（evictOlderThan / emptyDir）は純関数で Win/Linux 共通（os 標準のみ）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/hlconfig"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
)

// cacheDir は削除対象＝既定キャッシュ {dataDir}/headless-cache を返す。
// hlconfig.DefaultFolders と同一導出（単一情報源・コンフィグ焼き込み値と一致）。
func (s *Server) cacheDir() (string, error) {
	_, cacheFolder, err := hlconfig.DefaultFolders(s.dataDir)
	return cacheFolder, err
}

// --- 設定（自動キャッシュ削除のトグル / しきい値） ---

type cacheConfigResp struct {
	Enabled    bool `json:"enabled"`
	MaxAgeDays int  `json:"maxAgeDays"`
}

// handleCacheConfigGet: GET /api/v1/cache/config
func (s *Server) handleCacheConfigGet(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	cc := s.cfg.CacheCleanupOrDefault()
	s.cfgMu.RUnlock()
	writeOK(w, cacheConfigResp{Enabled: cc.Enabled, MaxAgeDays: cc.MaxAgeDays})
}

// handleCacheConfigPut: PUT /api/v1/cache/config {enabled, maxAgeDays}
func (s *Server) handleCacheConfigPut(w http.ResponseWriter, r *http.Request) {
	var body cacheConfigResp
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	if body.MaxAgeDays < 1 {
		writeErr(w, http.StatusBadRequest, "bad_request", "削除しきい値（日数）は1以上で指定してください")
		return
	}
	s.cfgMu.Lock()
	old := s.cfg.CacheCleanup
	s.cfg.CacheCleanup = &config.CacheCleanup{Enabled: body.Enabled, MaxAgeDays: body.MaxAgeDays}
	saveErr := s.cfg.SaveTo(s.cfgPath)
	if saveErr != nil {
		s.cfg.CacheCleanup = old // 保存失敗時は in-memory を巻き戻す
	}
	s.cfgMu.Unlock()
	if saveErr != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", saveErr.Error())
		return
	}
	writeOK(w, cacheConfigResp{Enabled: body.Enabled, MaxAgeDays: body.MaxAgeDays})
}

// --- 情報（パス + サイズ。サイズ集計は走査するため UI のボタン押下時のみ呼ぶ想定） ---

type cacheInfoResp struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	Exists    bool   `json:"exists"`
}

// handleCacheInfo: GET /api/v1/cache/info → {path, sizeBytes, exists}
func (s *Server) handleCacheInfo(w http.ResponseWriter, r *http.Request) {
	dir, err := s.cacheDir()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "cache_error", err.Error())
		return
	}
	size, exists := dirSize(dir)
	writeOK(w, cacheInfoResp{Path: dir, SizeBytes: size, Exists: exists})
}

// --- 手動全削除（停止中のみ・中身を空に） ---

// handleCacheClear: POST /api/v1/cache/clear
func (s *Server) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if s.driver.Status().State != headless.StateStopped {
		writeErr(w, http.StatusConflict, "headless_running",
			"ヘッドレスが停止中のときのみキャッシュを削除できます。先に停止してください")
		return
	}
	dir, err := s.cacheDir()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "cache_error", err.Error())
		return
	}
	s.cacheMu.Lock()
	removed, freed, err := emptyDir(dir)
	s.cacheMu.Unlock()
	if err != nil {
		// 一部が削除できなかった（ロック中ファイル等）。原因を明示する。
		writeErr(w, http.StatusInternalServerError, "cache_error", err.Error())
		return
	}
	writeOK(w, map[string]any{"removed": removed, "freedBytes": freed})
}

// --- 自動削除（停止時フック） ---

// maybeAutoEvictCache は driver.SetOnStopped から停止完了時に同期呼び出しされる。
// 設定 ON のとき既定キャッシュから mtime が maxAgeDays より古いファイルを削除し、結果を sys ログへ。
// プロセスは終了済み＝キャッシュ未使用が保証されるため安全にファイル削除できる。
func (s *Server) maybeAutoEvictCache() {
	s.cfgMu.RLock()
	cc := s.cfg.CacheCleanupOrDefault()
	lang := s.cfg.LangOrDefault()
	s.cfgMu.RUnlock()
	if !cc.Enabled {
		return
	}
	dir, err := s.cacheDir()
	if err != nil {
		log.Printf("[cache] 自動削除: キャッシュパス解決に失敗: %v", err)
		return
	}
	cutoff := time.Now().Add(-time.Duration(cc.MaxAgeDays) * 24 * time.Hour)
	s.cacheMu.Lock()
	removed, freed, evErr := evictOlderThan(dir, cutoff)
	s.cacheMu.Unlock()
	if evErr != nil {
		log.Printf("[cache] 自動削除中にエラー（一部のみ削除）: %v", evErr)
	}
	if removed > 0 {
		s.driver.PublishSys(i18n.T(lang, "cache.autoEvicted", removed, humanBytes(freed), cc.MaxAgeDays))
	}
}

// --- 削除コア（純関数・Win/Linux 共通・テスト容易） ---

// evictOlderThan は dir 配下で最終更新(mtime)が cutoff より古い通常ファイルを削除し、
// 削除件数と合計バイトを返す。削除で空になったサブフォルダも掃除する（dir 自体は残す）。
// シンボリックリンクは辿らない（WalkDir 既定）。個々の失敗は集約し最初のエラーを返す（best-effort 継続）。
func evictOlderThan(dir string, cutoff time.Time) (removed int, freed int64, firstErr error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil // フォルダ無し＝何もしない
	}
	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, e error) error {
		if e != nil || d.IsDir() {
			return nil // アクセス不能/ディレクトリはスキップ
		}
		fi, ie := d.Info()
		if ie != nil || !fi.Mode().IsRegular() {
			return nil
		}
		if fi.ModTime().Before(cutoff) {
			sz := fi.Size()
			if rmErr := os.Remove(p); rmErr != nil {
				if firstErr == nil {
					firstErr = rmErr
				}
				return nil
			}
			removed++
			freed += sz
		}
		return nil
	})
	if walkErr != nil && firstErr == nil {
		firstErr = walkErr
	}
	pruneEmptyDirs(dir)
	return removed, freed, firstErr
}

// emptyDir は dir 直下の全エントリを削除し（中身を空に・dir 自体は残す）、削除した
// 通常ファイル数と合計バイトを返す。フォルダ無しは no-op。個々の失敗は集約し最初を返す。
func emptyDir(dir string) (removed int, freed int64, firstErr error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		n, b := countTree(p)
		if rmErr := os.RemoveAll(p); rmErr != nil {
			if firstErr == nil {
				firstErr = rmErr
			}
			continue // 失敗分は件数に数えない
		}
		removed += n
		freed += b
	}
	return removed, freed, firstErr
}

// pruneEmptyDirs は root 配下の空サブフォルダを深い順に削除する（root 自体は残す）。
func pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e == nil && d.IsDir() && p != root {
			dirs = append(dirs, p)
		}
		return nil
	})
	// 深い順（セパレータ数）に消す＝親より先に子を消す。空でなければ os.Remove は失敗（無視）。
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) > strings.Count(dirs[j], string(filepath.Separator))
	})
	for _, d := range dirs {
		_ = os.Remove(d)
	}
}

// countTree は path 配下の通常ファイル数と合計バイトを返す（削除前の集計用）。
func countTree(path string) (files int, bytes int64) {
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, e error) error {
		if e != nil || d.IsDir() {
			return nil
		}
		if fi, ie := d.Info(); ie == nil && fi.Mode().IsRegular() {
			files++
			bytes += fi.Size()
		}
		return nil
	})
	return files, bytes
}

// dirSize は dir 配下の通常ファイルの合計サイズと、フォルダ有無を返す（best-effort）。
func dirSize(dir string) (size int64, exists bool) {
	info, err := os.Stat(dir)
	if err != nil {
		return 0, false // 無し/アクセス不能＝0・存在しない扱い
	}
	if !info.IsDir() {
		return 0, true
	}
	_, b := countTree(dir)
	return b, true
}

// humanBytes は B/KB/MB/… の人間可読表記を返す（sys ログ用）。
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
