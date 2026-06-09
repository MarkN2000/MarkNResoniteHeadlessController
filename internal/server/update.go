// update.go は自己更新（MRHC 自身の入れ替え）と終了依頼の HTTP 層。
// 実体は internal/selfupdate（設計: docs/design/self-update.md）。
package server

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"runtime"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/selfupdate"
)

// updateCheckTTL は check 結果を再利用する期間（ログイン/リロード毎の GitHub 往復を抑える）。
const updateCheckTTL = 10 * time.Minute

// SetUpdater は自己更新の実行器を注入する（main が MRHC_UPDATE_BASE を解決して構築する）。
// New の引数にしないのは既存の呼び出し（テスト含む）を保つため＝他の post-New セッターと同じ流儀。
func (s *Server) SetUpdater(u *selfupdate.Updater) { s.updater = u }

// SetRestartRequest は MRHC プロセスの再起動依頼を main へ伝えるコールバックを注入する。
// Listen 前に1回だけ呼ぶこと（serving 中の書き換えは想定しない）。
func (s *Server) SetRestartRequest(fn func()) { s.requestRestart = fn }

func (s *Server) stagedVersion() string {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.updateStaged
}

func (s *Server) setStagedVersion(v string) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateStaged = v
}

// cachedCheck は TTL 内のキャッシュ済み check 結果を返す（無ければ ok=false）。
func (s *Server) cachedCheck() (selfupdate.Info, bool) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	if s.updateCheck != nil && time.Since(s.updateCheckAt) < updateCheckTTL {
		return *s.updateCheck, true
	}
	return selfupdate.Info{}, false
}

// storeCheck は check 結果をキャッシュする。
func (s *Server) storeCheck(info selfupdate.Info) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateCheck = &info
	s.updateCheckAt = time.Now()
}

// invalidateCheck はキャッシュを破棄する（適用後など最新性が必要になったとき）。
func (s *Server) invalidateCheck() {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateCheck = nil
}

// updateCheckResp は GET /api/v1/update/check の応答。
type updateCheckResp struct {
	Current          string `json:"current"`          // 実行中の版（焼込値そのまま。dev 等もありうる）
	Latest           string `json:"latest"`           // 最新リリースタグ
	UpdateAvailable  bool   `json:"updateAvailable"`  // 実行中の版より新しいリリースがあるか
	CurrentIsRelease bool   `json:"currentIsRelease"` // 適用可能なリリースビルドか
	Staged           string `json:"staged,omitempty"` // 適用済み・再起動待ちの版（このプロセスでの適用のみ）
	Goos             string `json:"goos"`             // 再起動手順の出し分け用（windows / linux）

	// GitHub への確認に失敗したとき true（current/staged/goos のローカル情報のみ有効）。
	// CheckError はその errCode（no_release / update_failed 等）。
	CheckFailed bool   `json:"checkFailed,omitempty"`
	CheckError  string `json:"checkError,omitempty"`
}

// handleUpdateCheck: GET /api/v1/update/check → updateCheckResp
// GitHub への問い合わせは本ハンドラ呼び出し時のみ（常時ポーリングはフロント側でもしない）。
// staged/current/goos はローカル情報なので、GitHub 不達でも 200 ＋ CheckFailed で返す
// （エラー応答にすると、適用済み＝再起動待ちの表示と「今すぐ再起動」導線が UI から消えてしまう）。
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil {
		writeErr(w, http.StatusServiceUnavailable, "update_unavailable", "自己更新が初期化されていません")
		return
	}
	resp := updateCheckResp{
		Current: s.updater.Version,
		Staged:  s.stagedVersion(),
		Goos:    runtime.GOOS,
	}
	// TTL 内はキャッシュ応答（ログイン/リロード毎の GitHub 往復を抑える）。GitHub 不達は
	// キャッシュしない（次回すぐ再試行できるよう・staged 等のローカル情報は常に最新を載せる）。
	info, ok := s.cachedCheck()
	if !ok {
		var err error
		info, err = s.updater.Check(r.Context())
		if err != nil {
			resp.CheckFailed = true
			resp.CheckError = updateErrCode(err)
			writeOK(w, resp)
			return
		}
		s.storeCheck(info)
	}
	resp.Latest = info.Latest
	resp.UpdateAvailable = info.UpdateAvailable
	resp.CurrentIsRelease = info.CurrentIsRelease
	writeOK(w, resp)
}

// handleUpdateApply: POST /api/v1/update/apply
// 進捗を SSE でストリーミングする（イベント: update-progress / update-result / update-error）。
// Apply はリクエスト ctx ではなく Background で走らせる＝ブラウザ切断でも入れ替えを中断しない
// （全体上限は DLClient の Timeout が担う）。応答を取り逃しても check の staged で結果を回収できる。
// http.Flusher 非対応の writer（想定外）では従来の同期 JSON 応答へフォールバックする。
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil {
		writeErr(w, http.StatusServiceUnavailable, "update_unavailable", "自己更新が初期化されていません")
		return
	}
	fl, streaming := w.(http.Flusher)

	// 適用済み・再起動待ち中の再実行: 最新が staged と同じなら再DLせずそのまま返す（冪等）。
	// staged と異なる latest になっていれば（取り下げで古くなった場合も含め）続行して
	// latest を適用し直す（実行中の版より古ければ Apply 側が ErrUpToDate で拒否する）。
	if st := s.stagedVersion(); st != "" {
		if info, err := s.updater.Check(r.Context()); err == nil && info.Latest == st {
			if streaming {
				sseHeader(w)
				writeSSE(w, "update-result", map[string]any{"staged": st})
				fl.Flush()
			} else {
				writeOK(w, map[string]any{"staged": st})
			}
			return
		}
	}

	if !streaming {
		staged, err := s.updater.Apply(context.Background())
		if err != nil {
			writeUpdateErr(w, err)
			return
		}
		s.setStagedVersion(staged)
		s.invalidateCheck()
		writeOK(w, map[string]any{"staged": staged})
		return
	}

	sseHeader(w)
	// 進捗コールバックは Apply と同じハンドラ goroutine から同期的に呼ばれる（並行書込なし）。
	progress := func(downloaded, total int64) {
		writeSSE(w, "update-progress", map[string]any{"downloaded": downloaded, "total": total})
		fl.Flush()
	}
	staged, err := s.updater.ApplyWithProgress(context.Background(), progress)
	if err != nil {
		writeSSE(w, "update-error", map[string]string{"code": updateErrCode(err), "message": err.Error()})
		fl.Flush()
		return
	}
	s.setStagedVersion(staged)
	s.invalidateCheck()
	writeSSE(w, "update-result", map[string]any{"staged": staged})
	fl.Flush()
}

// handleRestart: POST /api/v1/restart → {accepted}
// MRHC プロセスの再起動を依頼する（自己更新後の「今すぐ再起動」用）。応答を返してから
// main の graceful 終了（ヘッドレス停止込み）＋新バイナリ起動の経路を起動する。
// http.Server.Shutdown は進行中の応答の完了を待つため、即時呼び出しでも応答は届く。
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if s.requestRestart == nil {
		writeErr(w, http.StatusServiceUnavailable, "restart_unavailable", "再起動依頼が初期化されていません")
		return
	}
	writeOK(w, map[string]any{"accepted": true})
	s.requestRestart()
}

// sseHeader は SSE 応答のヘッダを設定する（steam の events と同形）。
func sseHeader(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
}

// updateErrCode は selfupdate の sentinel を errCode へマップする
// （フロントは code で locale 変換・steam の sentinel+errorCode 方式と同じ）。
func updateErrCode(err error) string {
	switch {
	case errors.Is(err, selfupdate.ErrNoRelease):
		return "no_release"
	case errors.Is(err, selfupdate.ErrUpToDate):
		return "up_to_date"
	case errors.Is(err, selfupdate.ErrBusy):
		return "update_busy"
	case errors.Is(err, selfupdate.ErrNotReleaseBuild):
		return "not_release_build"
	case errors.Is(err, fs.ErrPermission):
		return "exe_dir_not_writable"
	default:
		return "update_failed"
	}
}

// writeUpdateErr は errCode に応じた HTTP ステータスでエラー応答を書く（apply 用）。
func writeUpdateErr(w http.ResponseWriter, err error) {
	code := updateErrCode(err)
	status := http.StatusBadGateway
	switch code {
	case "no_release":
		status = http.StatusNotFound
	case "up_to_date", "update_busy", "not_release_build":
		status = http.StatusConflict
	case "exe_dir_not_writable":
		status = http.StatusInternalServerError
	}
	writeErr(w, status, code, err.Error())
}
