package server

// write API（Pre-7c）。Resonite ヘッドレスへの状態変更コマンドを HTTP で公開する。
// 正本仕様: docs/design/phase-7-spec.md §2.4/2.5。
//
// 設計方針:
//   - idx は path、識別子/引数は body（JSON）。全 POST・認証必須。
//   - セッション操作 = ExecGroup(focus idx → cmd)、グローバル操作 = Exec（focus 不要）。
//   - idx は信頼（focus 前の worlds 検証なし。range 外 idx の誤爆は妥協＝read 系と同挙動）。
//   - 方針A: write 出力はパースせず、プロンプト復帰 = 成功で {"executed":true} を返す。
//     UI はトースト + 該当データ再取得で実状態を見せる。
//   - 引数サニタイズは internal/headless の QuoteArg / SanitizeToken に集約。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
)

// startTimeout は新規ワールド開始（startworldurl/startWorldTemplate）の待ち時間。
const startTimeout = 60 * time.Second

// restartTimeout はセッション restart の待ち時間。restart は再生成中にプロンプトを
// 返さない可能性があり（driver.go の警告）、その場合 ErrTimeout→500 を誤報し得る。
// 実機検証（spec §2.5.2 項目3）までは長めに取って様子を見る楽観実装。
const restartTimeout = 180 * time.Second

// --- 実行 helper ---

// execSession は ExecGroup(focus idx → cmd) を実行し、方針A で結果を返す。
// focus と cmd は execMu を握ったまま連続実行されるため、他リクエストに割り込まれない。
func (s *Server) execSession(w http.ResponseWriter, r *http.Request, idx int, cmd string, opts ...headless.ExecOption) {
	err := s.driver.ExecGroup(r.Context(), func(tx headless.Tx) error {
		if _, e := tx.Exec(fmt.Sprintf("focus %d", idx)); e != nil {
			return e
		}
		_, e := tx.Exec(cmd, opts...)
		return e
	})
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, map[string]any{"executed": true})
}

// execGlobal は focus 不要のグローバルコマンドを Exec で実行し、方針A で結果を返す。
func (s *Server) execGlobal(w http.ResponseWriter, r *http.Request, cmd string, opts ...headless.ExecOption) {
	if _, err := s.driver.Exec(r.Context(), cmd, opts...); err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, map[string]any{"executed": true})
}

// --- 入力 helper ---

// decodeBody は JSON body を target にデコードする。失敗時 400 を返し false を返す。
func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正な JSON")
		return false
	}
	return true
}

// requireField は必須文字列が空（空白のみ含む）でないか検証する。空なら 400 を返し false。
func requireField(w http.ResponseWriter, name, value string) bool {
	if strings.TrimSpace(value) == "" {
		writeErr(w, http.StatusBadRequest, "missing_field", fmt.Sprintf("%s は必須です", name))
		return false
	}
	return true
}

// reqIdx は path の {idx} を取り出す。失敗時 400 を返し ok=false。
func reqIdx(w http.ResponseWriter, r *http.Request) (int, bool) {
	idx, err := parseSessionIdx(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return 0, false
	}
	return idx, true
}

// --- ファクトリ（反復ハンドラの集約）---

// sessionUserOp は「focus idx → <verb> "<user>"」型の単一引数ユーザー操作ハンドラを生成する。
// kick/ban/silence/unsilence/respawn/invite が該当（出力はノイジーだが方針A で無視）。
func (s *Server) sessionUserOp(verb string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idx, ok := reqIdx(w, r)
		if !ok {
			return
		}
		var body struct {
			User string `json:"user"`
		}
		if !decodeBody(w, r, &body) || !requireField(w, "user", body.User) {
			return
		}
		s.execSession(w, r, idx, verb+" "+headless.QuoteArg(body.User))
	}
}

// sessionCmdOp は「focus idx → <cmd>」型の引数なしセッション操作ハンドラを生成する。
// restart/save/close が該当。opts で個別 timeout を渡す。
func (s *Server) sessionCmdOp(cmd string, opts ...headless.ExecOption) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idx, ok := reqIdx(w, r)
		if !ok {
			return
		}
		s.execSession(w, r, idx, cmd, opts...)
	}
}

// globalUserOp は「<cmd> "<user>"」型の単一引数グローバル操作ハンドラを生成する。
// acceptfriendrequest/sendFriendRequest/removeFriend が該当（focus 不要）。
func (s *Server) globalUserOp(cmd string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			User string `json:"user"`
		}
		if !decodeBody(w, r, &body) || !requireField(w, "user", body.User) {
			return
		}
		s.execGlobal(w, r, cmd+" "+headless.QuoteArg(body.User))
	}
}

// --- 個別ハンドラ（引数付き／型付き）---

// handleSessionRole: role "<user>" "<role>"
func (s *Server) handleSessionRole(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		User string `json:"user"`
		Role string `json:"role"`
	}
	if !decodeBody(w, r, &body) || !requireField(w, "user", body.User) || !requireField(w, "role", body.Role) {
		return
	}
	s.execSession(w, r, idx, fmt.Sprintf("role %s %s", headless.QuoteArg(body.User), headless.QuoteArg(body.Role)))
}

// handleSessionMessage: message "<user>" "<text>"（個別 DM。全体ブロードキャストではない）
func (s *Server) handleSessionMessage(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		User    string `json:"user"`
		Message string `json:"message"`
	}
	if !decodeBody(w, r, &body) || !requireField(w, "user", body.User) || !requireField(w, "message", body.Message) {
		return
	}
	s.execSession(w, r, idx, fmt.Sprintf("message %s %s", headless.QuoteArg(body.User), headless.QuoteArg(body.Message)))
}

// handleSessionAccessLevel: accesslevel <Level>（引用なしトークン）
func (s *Server) handleSessionAccessLevel(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		Level string `json:"level"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	level, err := headless.SanitizeToken(body.Level)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_value", "level: "+err.Error())
		return
	}
	s.execSession(w, r, idx, "accesslevel "+level)
}

// handleSessionMaxUsers: maxusers <N>（正の整数のみ）
func (s *Server) handleSessionMaxUsers(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		MaxUsers int `json:"maxUsers"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.MaxUsers <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid_value", "maxUsers は正の整数である必要があります")
		return
	}
	s.execSession(w, r, idx, fmt.Sprintf("maxusers %d", body.MaxUsers))
}

// handleSessionName: name "<name>"
func (s *Server) handleSessionName(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &body) || !requireField(w, "name", body.Name) {
		return
	}
	s.execSession(w, r, idx, "name "+headless.QuoteArg(body.Name))
}

// handleSessionDescription: description "<text>"（空文字許容）
func (s *Server) handleSessionDescription(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		Description string `json:"description"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	s.execSession(w, r, idx, "description "+headless.QuoteArg(body.Description))
}

// handleSessionHideFromListing: hideFromListing <bool>
func (s *Server) handleSessionHideFromListing(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		Hide bool `json:"hide"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	s.execSession(w, r, idx, fmt.Sprintf("hideFromListing %t", body.Hide))
}

// handleSessionStart: 稼働中に新規ワールドを開始（/start のプロセス起動とは別物）。
//   - mode=url      → startworldurl "<url>"
//   - mode=template → startWorldTemplate "<name>"（テンプレ名に空白があり得るため引用。要実機検証）
func (s *Server) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode     string `json:"mode"`
		URL      string `json:"url"`
		Template string `json:"template"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var cmd string
	switch body.Mode {
	case "url":
		if !requireField(w, "url", body.URL) {
			return
		}
		cmd = "startworldurl " + headless.QuoteArg(body.URL)
	case "template":
		if !requireField(w, "template", body.Template) {
			return
		}
		cmd = "startWorldTemplate " + headless.QuoteArg(body.Template)
	default:
		writeErr(w, http.StatusBadRequest, "invalid_value", `mode は "url" または "template" を指定してください`)
		return
	}
	s.execGlobal(w, r, cmd, headless.WithTimeout(startTimeout))
}

// handleBanUnban: unban <userId>（listbans の ID。引用なしトークン）
func (s *Server) handleBanUnban(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"userId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	id, err := headless.SanitizeToken(body.UserID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_value", "userId: "+err.Error())
		return
	}
	s.execGlobal(w, r, "unban "+id)
}
