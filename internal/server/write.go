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

// restartTimeout はセッション restart の待ち時間。実機検証(2026-05-30)で restart は
// プロンプトを返すと確認済み（空ワールドで≈1s）。重いワールドの再生成に備えた余裕のある上限。
const restartTimeout = 180 * time.Second

// closeTimeout はセッション close の待ち時間。restart と同じ teardown 系の重さを伴うため
// restartTimeout に揃える（空ワールドでは≈0.2s だが上限は余裕を持たせる）。
const closeTimeout = restartTimeout

// saveTimeout はセッション save の待ち時間。save は世界のシリアライズ＋クラウドへの
// アセットアップロードを伴い、大規模ワールド×低速回線では数分かかり得る（Driver.Stop() が
// 「保存に数分かかる場合がある」として 180s 猶予を取るのと同じ理由）。タイムアウトは
// 上限（保険）であって待ち時間ではなく（プロンプト復帰で即返る）、プロセス死亡時は
// ErrProcessGone で即中断されるため、固まった save を失敗と見なすまでの猶予として長めに取る。
const saveTimeout = 600 * time.Second

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
	// user は識別子なので QuoteArg、message 本文はリッチテキスト/改行可なので QuoteRichText。
	s.execSession(w, r, idx, fmt.Sprintf("message %s %s", headless.QuoteArg(body.User), headless.QuoteRichText(body.Message)))
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
	s.execSession(w, r, idx, "name "+headless.QuoteRichText(body.Name))
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
	s.execSession(w, r, idx, "description "+headless.QuoteRichText(body.Description))
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

// handleSessionSpawn: spawn "<url>" <active> <persistent>（R14・focus idx → cmd）
// アイテムを record URL からフォーカス中ワールドに生成する。url 必須・active/persistent は bool。
func (s *Server) handleSessionSpawn(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		URL        string `json:"url"`
		Active     bool   `json:"active"`
		Persistent bool   `json:"persistent"`
	}
	if !decodeBody(w, r, &body) || !requireField(w, "url", body.URL) {
		return
	}
	s.execSession(w, r, idx, headless.SpawnCmd(body.URL, body.Active, body.Persistent))
}

// handleSessionImpulse: dynamicimpulsestring "<tag>" "<value>"（R14・focus idx → cmd）
// scene root へタグ＋文字列値の impulse を送る。tag は必須、value は任意（空可＝
// 受信アイテムが固定挙動で値を使わない場合がある。告知③と同条件）。
func (s *Server) handleSessionImpulse(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		Tag   string `json:"tag"`
		Value string `json:"value"`
	}
	if !decodeBody(w, r, &body) || !requireField(w, "tag", body.Tag) {
		return
	}
	s.execSession(w, r, idx, headless.DynamicImpulseStringCmd(body.Tag, body.Value))
}

// handleSessionSpawnImpulse: スポーン＆パルス＝告知③のセッション版（フォーカス中ワールドのみ）。
// spawn → 実体化待ち（spawnImpulseDelay）→ dynamicimpulsestring を1リクエストで完走する
// （途中でブラウザを閉じても impulse まで届く）。templateId 非空＝スポーン＆パルステンプレから
// URL/タグを実行直前に解決・空＝手動（itemUrl/impulseTag を使用）。告知③と同じく itemUrl 空は
// spawn 省略で impulse のみ・spawn は active=true / persistent=false 固定（一時アイテム）。
// 待機中は execMu を保持しない（spawn と impulse を別 ExecGroup に分ける＝他リクエストを妨げない）。
func (s *Server) handleSessionSpawnImpulse(w http.ResponseWriter, r *http.Request) {
	idx, ok := reqIdx(w, r)
	if !ok {
		return
	}
	var body struct {
		TemplateID string `json:"templateId"`
		ItemURL    string `json:"itemUrl"`
		ImpulseTag string `json:"impulseTag"`
		Message    string `json:"message"`
		SpeakerID  int64  `json:"speakerId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	url, tag := body.ItemURL, body.ImpulseTag
	var tpl *itemTemplate
	if body.TemplateID != "" {
		foundTpl, found := s.itemTpl.lookup(r.Context(), body.TemplateID, templateActionSpawnImpulse)
		if !found {
			writeErr(w, http.StatusBadRequest, "bad_request", "テンプレートが見つかりません: "+body.TemplateID)
			return
		}
		tpl = &foundTpl
		url, tag = tpl.URL, tpl.Tag
	}
	if !requireField(w, "impulseTag", tag) {
		return
	}
	value, err := impulseValueForTemplate(tpl, body.Message, body.SpeakerID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if url != "" {
		err := s.driver.ExecGroup(r.Context(), func(tx headless.Tx) error {
			if _, e := tx.Exec(fmt.Sprintf("focus %d", idx)); e != nil {
				return e
			}
			_, e := tx.Exec(headless.SpawnCmd(url, true, false))
			return e
		})
		if err != nil {
			writeExecErr(w, err)
			return
		}
		select { // spawn したアイテムの実体化（アセット読込）を待ってから impulse
		case <-r.Context().Done():
			return
		case <-time.After(s.spawnImpulseDelay):
		}
	}
	s.execSession(w, r, idx, headless.DynamicImpulseStringCmd(tag, value))
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

// handleBanUnban: unbanByID <userId>（listbans の UserID。引用なしトークン）
// ※ 実機検証(2026-05-30)で確定: 素の unban は usage が <username> なので userId では効かない。
// listbans が返すのは UserID なので ID 指定の unbanByID <user ID> を使う。
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
	s.execGlobal(w, r, "unbanByID "+id)
}

// handleBanByID: banByID <userId>（全セッションから BAN・focus 不要・R1）。
// 検索結果など在席していないユーザーを ID で BAN するのに使う（unbanByID と対称）。
// userId は引用なしトークン（U-xxxx 形式）。help 確定: "Bans user with given User ID from all sessions"。
func (s *Server) handleBanByID(w http.ResponseWriter, r *http.Request) {
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
	s.execGlobal(w, r, "banByID "+id)
}
