package server

// Resonite ヘッドレスのログ閲覧（読み取り専用）。ログは {InstallDir}/Headless/Logs に
// 出力される（logsFolder=null の既定。実機確認済 docs/resonite-domain-facts.md）。
// ログファイルは両OSとも UTF-8（コンソール stdout が Shift_JIS なのと別。実機249件で確認）
// のため変換せずそのまま返す。logsFolder を独自設定した場合は対象外（既定運用前提・UI にパス明示）。

import (
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

// maxLogTailBytes はログ取得の上限（末尾10MiB）。これを超えるファイルは末尾だけ返し truncated=true。
// 全文を返すとブラウザの描画/転送が重くなるため末尾（最新側）に絞る。
const maxLogTailBytes = 10 << 20 // 10 MiB

// logsDir は Resonite ヘッドレスのログフォルダ（{InstallDir}/Headless/Logs）を解決する。
// InstallDirOrDefault（Steam.InstallDir→既定 {dataDir}/resonite）から導出し "~" を展開する
// （起動パス HeadlessPathOrDefault と同じインストール場所に収束する）。
func (s *Server) logsDir() string {
	s.cfgMu.RLock()
	installDir := s.cfg.InstallDirOrDefault(s.dataDir)
	s.cfgMu.RUnlock()
	return filepath.Join(platform.ExpandHome(installDir), "Headless", "Logs")
}

// logFileInfo は一覧の1要素。
type logFileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"` // RFC3339（UTC）
}

// openLogFile はパス検証済みのログファイルとメタデータを返す。
// 閲覧とダウンロードで同じ制約・エラー応答を使い、成功時のファイルは呼び出し側が閉じる。
func (s *Server) openLogFile(w http.ResponseWriter, name string) (*os.File, fs.FileInfo, bool) {
	// トラバーサル防止: basename と一致しない（区切り文字や ".." を含む）名前は拒否。
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なログファイル名です")
		return nil, nil, false
	}
	// Logs フォルダ内の想定外ファイルを返さないよう .log のみに限定。
	if !strings.EqualFold(filepath.Ext(name), ".log") {
		writeErr(w, http.StatusBadRequest, "bad_request", "ログファイル(.log)のみ指定できます")
		return nil, nil, false
	}
	f, err := os.Open(filepath.Join(s.logsDir(), name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeErr(w, http.StatusNotFound, "log_not_found", "指定のログが見つかりません")
			return nil, nil, false
		}
		// 稼働中の現行ログがロックされている等。握り潰さず明示する。
		writeErr(w, http.StatusInternalServerError, "log_read_failed", err.Error())
		return nil, nil, false
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		writeErr(w, http.StatusInternalServerError, "log_read_failed", err.Error())
		return nil, nil, false
	}
	return f, fi, true
}

// handleLogList: GET /api/v1/logs → []logFileInfo（更新時刻の新しい順＝現行ログが先頭）。
// フォルダが無い（未DL/未起動）場合は空配列を返す（500 にしない）。
func (s *Server) handleLogList(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.logsDir())
	if err != nil {
		writeOK(w, []logFileInfo{}) // フォルダ無し等＝ログ0件扱い
		return
	}
	type item struct {
		name string
		size int64
		mod  time.Time
	}
	items := make([]item, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".log") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue // 列挙中に消えた等はスキップ
		}
		items = append(items, item{name: e.Name(), size: fi.Size(), mod: fi.ModTime()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	out := make([]logFileInfo, len(items))
	for i, it := range items {
		out[i] = logFileInfo{Name: it.name, Size: it.size, ModTime: it.mod.UTC().Format(time.RFC3339)}
	}
	writeOK(w, out)
}

// handleLogGet: GET /api/v1/logs/{name} → {name, size, truncated, content}
// name は basename のみ許可（パストラバーサル防止）。サイズが上限超なら末尾だけ返し truncated=true。
func (s *Server) handleLogGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	f, fi, ok := s.openLogFile(w, name)
	if !ok {
		return
	}
	defer f.Close()

	size := fi.Size()
	truncated := false
	if size > maxLogTailBytes {
		if _, err := f.Seek(size-maxLogTailBytes, io.SeekStart); err != nil {
			writeErr(w, http.StatusInternalServerError, "log_read_failed", err.Error())
			return
		}
		truncated = true
	}
	data, err := io.ReadAll(f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "log_read_failed", err.Error())
		return
	}
	content := string(data)
	// 末尾切り出しはマルチバイト文字を途中で切り得るため、最初の改行まで捨てて行頭から表示する
	// （先頭1行の文字化け防止）。改行が無ければそのまま。
	if truncated {
		if idx := strings.IndexByte(content, '\n'); idx >= 0 {
			content = content[idx+1:]
		}
	}
	writeOK(w, map[string]any{"name": name, "size": size, "truncated": truncated, "content": content})
}

// handleLogDownload: GET /api/v1/logs/{name}/download → 元のログファイル全文。
// 表示用の10MiB上限は適用せず、http.ServeContent でブラウザへ直接ストリーミングする。
func (s *Server) handleLogDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	f, fi, ok := s.openLogFile(w, name)
	if !ok {
		return
	}
	defer f.Close()

	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, name, fi.ModTime(), f)
}
