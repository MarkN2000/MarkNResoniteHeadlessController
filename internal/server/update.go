// update.go は自己更新（MRHC 自身の入れ替え）と終了依頼の HTTP 層。
// 実体は internal/selfupdate（設計: docs/design/self-update.md）。
package server

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"runtime"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/selfupdate"
)

// SetUpdater は自己更新の実行器を注入する（main が MRHC_UPDATE_BASE を解決して構築する）。
// New の引数にしないのは既存の呼び出し（テスト含む）を保つため＝他の post-New セッターと同じ流儀。
func (s *Server) SetUpdater(u *selfupdate.Updater) { s.updater = u }

// SetShutdownRequest は MRHC プロセスの終了依頼を main へ伝えるコールバックを注入する。
// Listen 前に1回だけ呼ぶこと（serving 中の書き換えは想定しない）。
func (s *Server) SetShutdownRequest(fn func()) { s.requestShutdown = fn }

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
// （エラー応答にすると、適用済み＝再起動待ちの表示と「今すぐ終了」導線が UI から消えてしまう）。
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
	info, err := s.updater.Check(r.Context())
	if err != nil {
		resp.CheckFailed = true
		resp.CheckError = updateErrCode(err)
		writeOK(w, resp)
		return
	}
	resp.Latest = info.Latest
	resp.UpdateAvailable = info.UpdateAvailable
	resp.CurrentIsRelease = info.CurrentIsRelease
	writeOK(w, resp)
}

// handleUpdateApply: POST /api/v1/update/apply → {staged}
// 同期実行（バイナリは十数MB＝通常数秒〜十数秒）。リクエスト ctx ではなく Background で
// 走らせる＝ブラウザ切断でも入れ替えを中断しない（全体上限は DLClient の Timeout が担う）。
// 応答を取り逃しても check の staged で結果を回収できる。
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil {
		writeErr(w, http.StatusServiceUnavailable, "update_unavailable", "自己更新が初期化されていません")
		return
	}
	// 適用済み・再起動待ち中の再実行: 最新が staged と同じなら再DLせずそのまま返す（冪等）。
	// staged と異なる latest になっていれば（取り下げで古くなった場合も含め）続行して
	// latest を適用し直す（実行中の版より古ければ Apply 側が ErrUpToDate で拒否する）。
	if st := s.stagedVersion(); st != "" {
		if info, err := s.updater.Check(r.Context()); err == nil && info.Latest == st {
			writeOK(w, map[string]any{"staged": st})
			return
		}
	}
	staged, err := s.updater.Apply(context.Background())
	if err != nil {
		writeUpdateErr(w, err)
		return
	}
	s.setStagedVersion(staged)
	writeOK(w, map[string]any{"staged": staged})
}

// handleShutdown: POST /api/v1/shutdown → {accepted}
// MRHC プロセス自体の終了を依頼する（自己更新後の「今すぐ終了」用）。応答を返してから
// main の graceful 終了（ヘッドレス停止込み・Ctrl+C と同じ経路）を起動する。
// http.Server.Shutdown は進行中の応答の完了を待つため、即時呼び出しでも応答は届く。
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if s.requestShutdown == nil {
		writeErr(w, http.StatusServiceUnavailable, "shutdown_unavailable", "終了依頼が初期化されていません")
		return
	}
	writeOK(w, map[string]any{"accepted": true})
	s.requestShutdown()
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
