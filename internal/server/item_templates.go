package server

// item_templates.go はアイテムテンプレート（spawn するアイテム＋impulse タグ）のリモートリスト
// 取得・キャッシュ・解決を担う。アイテム URL の差し替え・テンプレ追加をアプリのリリースなしに
// 全インスタンスへ配信する。正本: docs/design/item-templates.md
//
// リストは1系統。各テンプレートの actions が利用箇所（単体スポーン／スポーン＆パルス／告知）を表す。
//
// フォールバック連鎖（常にこの順・最悪でも焼き込みビルトイン＝従来同等の動作に退化する）:
//   メモリキャッシュ(TTL内) → リモート取得 → -data の永続キャッシュ(最終取得分) → ビルトイン

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

// 既定の取得元（main ブランチ＝リリースと独立に更新できる）。テストでは templateStore.url を差し替える。
const itemTemplatesURL = "https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/item-templates.json"

// itemTemplatesTTL はメモリキャッシュの有効期間。raw.githubusercontent.com の
// CDN キャッシュ（約5分）より長めに取り、UI 操作のたびの往復を抑える。
const itemTemplatesTTL = 10 * time.Minute

// itemTemplate はテンプレート1件。ID は config（announce.templateId）や実行リクエストから参照される
// 永続キー＝リモートリストでの改名・削除は既存の参照を切るため運用上禁止
// （アイテムの更新は url の書き換えで行う）。
type itemTemplate struct {
	ID      string            `json:"id"`
	Label   map[string]string `json:"label"`   // 言語コード→表示名。UI 側で 現在言語→en→ja→先頭→id の順に解決
	URL     string            `json:"url"`     // spawn する resrec:/// URL
	Tag     string            `json:"tag"`     // dynamicimpulsestring のタグ
	Actions []templateAction  `json:"actions"` // 利用可能な操作
	Input   *templateInput    `json:"input,omitempty"`
}

// templateInput はテンプレートが要求する追加入力。未指定は従来どおりテキストのみ。
type templateInput struct {
	Kind string `json:"kind"`
}

const templateInputTTSVoice = "ttsVoice"

// templateAction はテンプレートを利用できる操作。
type templateAction string

const (
	templateActionSpawn        templateAction = "spawn"
	templateActionSpawnImpulse templateAction = "spawnImpulse"
	templateActionAnnounce     templateAction = "announce"
)

// itemTemplateList は配信 JSON のルート。Version は情報用で互換判定には使わない。
type itemTemplateList struct {
	Version   int            `json:"version"`
	Templates []itemTemplate `json:"templates"`
}

// builtinItemTemplates は単一テンプレートリストの最終フォールバック
// （assets/item-templates.json のスナップショット）。
// 先頭の ID は config.DefaultRestart() の TemplateID と同期すること。
var builtinItemTemplates = []itemTemplate{
	{
		ID:      "torazo-close",
		Label:   map[string]string{"ja": "とらぞセッション閉店アナウンス", "en": "Torazo session closing announce"},
		URL:     "resrec:///U-MarkN/R-ba48e002-7810-43b6-b12d-41e68863d5c4",
		Tag:     "MRHC.play",
		Actions: []templateAction{templateActionAnnounce},
	},
	{
		ID:      "tts-loop",
		Label:   map[string]string{"ja": "テキスト読み上げループ", "en": "Text-to-speech loop"},
		URL:     "resrec:///U-MarkN/R-01a038f3-6ea5-79a8-be58-c9673007f5dd",
		Tag:     "MRHC.play",
		Actions: []templateAction{templateActionSpawn, templateActionSpawnImpulse, templateActionAnnounce},
	},
	{
		ID:      "tts-single",
		Label:   map[string]string{"ja": "テキスト読み上げシングル", "en": "Text-to-speech single"},
		URL:     "", // resrec URL 設定後に有効化される
		Tag:     "MRHC.play",
		Actions: []templateAction{templateActionSpawnImpulse},
	},
	{
		ID:      "tts-voice-single",
		Label:   map[string]string{"ja": "テキスト読み上げボイス指定シングル", "en": "Voice-specified text-to-speech single"},
		URL:     "", // resrec URL 設定後に有効化される
		Tag:     "MRHC.play",
		Actions: []templateAction{templateActionSpawnImpulse},
		Input:   &templateInput{Kind: templateInputTTSVoice},
	},
	{
		ID:      "tts-voice-loop",
		Label:   map[string]string{"ja": "テキスト読み上げボイス指定ループ", "en": "Voice-specified text-to-speech loop"},
		URL:     "resrec:///U-MarkN/R-01a03939-ddd5-786d-a50f-02bc4da83d2a",
		Tag:     "MRHC.play",
		Actions: []templateAction{templateActionSpawnImpulse, templateActionAnnounce},
		Input:   &templateInput{Kind: templateInputTTSVoice},
	},
}

// templateStore は1系統のテンプレリストの取得・キャッシュ・解決器。
type templateStore struct {
	url     string         // 取得元（テストで差し替え）
	client  *http.Client   // 取得用（timeout 込み）
	path    string         // -data の永続キャッシュのフルパス（""=永続なし・テスト等）
	builtin []itemTemplate // 全フォールバック失敗時の最終手段
	mu      sync.Mutex
	cache   []itemTemplate // 最終取得成功分のメモリキャッシュ
	fetched time.Time      // cache の取得時刻（TTL 判定。永続分から復元したときはゼロ）
}

func newTemplateStore(url, path string, builtin []itemTemplate) *templateStore {
	return &templateStore{
		url:     url,
		path:    path,
		builtin: builtin,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// valid は不正エントリを除いて返す（壊れた1件で全体を捨てない）。label は任意＝UI が id へフォールバックする。
func (st *templateStore) valid(list []itemTemplate) []itemTemplate {
	out := make([]itemTemplate, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for _, t := range list {
		if t.ID == "" || t.URL == "" {
			continue
		}
		if _, exists := seen[t.ID]; exists {
			continue
		}
		if !validTemplateActions(t.Actions) {
			continue
		}
		if (hasTemplateAction(t.Actions, templateActionSpawnImpulse) || hasTemplateAction(t.Actions, templateActionAnnounce)) && t.Tag == "" {
			continue
		}
		if t.Input != nil {
			if t.Input.Kind != templateInputTTSVoice ||
				(!hasTemplateAction(t.Actions, templateActionSpawnImpulse) && !hasTemplateAction(t.Actions, templateActionAnnounce)) {
				continue
			}
		}
		seen[t.ID] = struct{}{}
		out = append(out, t)
	}
	return out
}

func validTemplateActions(actions []templateAction) bool {
	if len(actions) == 0 {
		return false
	}
	seen := make(map[templateAction]struct{}, len(actions))
	for _, action := range actions {
		switch action {
		case templateActionSpawn, templateActionSpawnImpulse, templateActionAnnounce:
		default:
			return false
		}
		if _, exists := seen[action]; exists {
			return false
		}
		seen[action] = struct{}{}
	}
	return true
}

func hasTemplateAction(actions []templateAction, wanted templateAction) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}

// fetch はリモートから1回取得する。HTTP/JSON 失敗・有効0件はエラー。
func (st *templateStore) fetch(ctx context.Context) ([]itemTemplate, error) {
	if st.client == nil || st.url == "" {
		return nil, fmt.Errorf("取得元が未設定")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, st.url, nil)
	if err != nil {
		return nil, err
	}
	res, err := st.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var list itemTemplateList
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&list); err != nil {
		return nil, err
	}
	got := st.valid(list.Templates)
	if len(got) == 0 {
		return nil, fmt.Errorf("有効なテンプレートが0件")
	}
	return got, nil
}

// templates は現在有効なテンプレ一覧と出所（remote|cache|builtin）を返す。
// 取得成功時はメモリ＋永続キャッシュを更新する。失敗時の cache は
// 「過去に取得成功したリスト」（期限切れメモリ または -data の永続分）。
func (st *templateStore) templates(ctx context.Context) ([]itemTemplate, string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.cache) > 0 && time.Since(st.fetched) < itemTemplatesTTL {
		return st.cache, "remote"
	}
	got, err := st.fetch(ctx)
	if err == nil {
		st.cache, st.fetched = got, time.Now()
		if st.path != "" {
			writeJSONFile(st.path, itemTemplateList{Version: 1, Templates: got})
		}
		return got, "remote"
	}
	log.Printf("[item-templates] リモート取得に失敗（キャッシュへフォールバック）: %v", err)
	if len(st.cache) > 0 {
		return st.cache, "cache"
	}
	if st.path != "" {
		persisted := readJSONFile[itemTemplateList](st.path)
		if got := st.valid(persisted.Templates); len(got) > 0 {
			st.cache = got // fetched はゼロのまま＝次回も再取得を試みる
			return got, "cache"
		}
	}
	return st.valid(st.builtin), "builtin"
}

// lookup は id と action の両方でテンプレを探す。テンプレートIDだけを知っていても、
// 許可されていない操作には利用できない。
func (st *templateStore) lookup(ctx context.Context, id string, action templateAction) (itemTemplate, bool) {
	list, _ := st.templates(ctx)
	for _, t := range list {
		if t.ID == id && hasTemplateAction(t.Actions, action) {
			return t, true
		}
	}
	return itemTemplate{}, false
}

// resolveAnnounce は告知の templateId を URL/タグへ解決した AnnounceAction を返す。
// templateId 空（手動入力）はそのまま。未解決（リストから id が消えた等の異常系）は
// ok=false＝呼び出し側（orchestrator）が告知をスキップしてログに残す。
func (s *Server) resolveAnnounce(ctx context.Context, a config.AnnounceAction) (config.AnnounceAction, bool) {
	if a.TemplateID == "" {
		if _, err := impulseValueForTemplate(nil, a.Message, a.SpeakerID); err != nil {
			return a, false
		}
		return a, true
	}
	t, ok := s.itemTpl.lookup(ctx, a.TemplateID, templateActionAnnounce)
	if !ok {
		return a, false
	}
	value, err := impulseValueForTemplate(&t, a.Message, a.SpeakerID)
	if err != nil {
		return a, false
	}
	a.ItemURL, a.ImpulseTag = t.URL, t.Tag
	a.Message = value
	return a, true
}

// writeTemplates は GET テンプレ一覧の共通実装。
// フォールバック連鎖により常に 200 で何かしらの一覧を返す（フロントの選択肢用）。
func writeTemplates(w http.ResponseWriter, r *http.Request, st *templateStore) {
	list, source := st.templates(r.Context())
	writeOK(w, map[string]any{"templates": list, "source": source})
}

// handleItemTemplates: GET /api/v1/item-templates（全操作のテンプレ一覧）
func (s *Server) handleItemTemplates(w http.ResponseWriter, r *http.Request) {
	writeTemplates(w, r, s.itemTpl)
}
