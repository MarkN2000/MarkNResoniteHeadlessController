package server

// announce_templates.go はアイテムテンプレート（spawn するアイテム＋impulse タグ）のリモートリスト
// 取得・キャッシュ・解決を担う。アイテム URL の差し替え・テンプレ追加をアプリのリリースなしに
// 全インスタンスへ配信する。正本: docs/design/announce-templates.md
//
// リストは3系統（templateStore の3インスタンス・スキーマ/機構は共通）:
//   - 告知テンプレ（assets/announce-templates.json・事前アクション③が参照）
//   - スポーン＆パルステンプレ（assets/spawn-templates.json・セッションタブが参照）
//   - 単体スポーンテンプレ（assets/item-spawn-templates.json・セッションタブのアイテムスポーンが参照。
//     tag を使わないため tag 任意＝requireTag=false）
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
const (
	announceTemplatesURL  = "https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/announce-templates.json"
	spawnTemplatesURL     = "https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/spawn-templates.json"
	itemSpawnTemplatesURL = "https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/item-spawn-templates.json"
)

// itemTemplatesTTL はメモリキャッシュの有効期間。raw.githubusercontent.com の
// CDN キャッシュ（約5分）より長めに取り、UI 操作のたびの往復を抑える。
const itemTemplatesTTL = 10 * time.Minute

// itemTemplate はテンプレート1件。ID は config（announce.templateId）や実行リクエストから参照される
// 永続キー＝リモートリストでの改名・削除は既存の参照を切るため運用上禁止
// （アイテムの更新は url の書き換えで行う）。
type itemTemplate struct {
	ID    string            `json:"id"`
	Label map[string]string `json:"label"` // 言語コード→表示名。UI 側で 現在言語→en→ja→先頭→id の順に解決
	URL   string            `json:"url"`   // spawn する resrec:/// URL
	Tag   string            `json:"tag"`   // dynamicimpulsestring のタグ
}

// itemTemplateList は配信 JSON のルート。Version は情報用で互換判定には使わない
// （変更はフィールド追加のみ＝未知キー無視で互換、という前提を配信側が守る）。
type itemTemplateList struct {
	Version   int            `json:"version"`
	Templates []itemTemplate `json:"templates"`
}

// builtinAnnounceTemplates は告知テンプレの最終フォールバック
// （assets/announce-templates.json のスナップショット）。
// 先頭の ID は config.DefaultRestart() の TemplateID と同期すること。
var builtinAnnounceTemplates = []itemTemplate{
	{
		ID:    "torazo-close",
		Label: map[string]string{"ja": "とらぞセッション閉店アナウンス", "en": "Torazo session closing announce"},
		URL:   "resrec:///U-MarkN/R-ba48e002-7810-43b6-b12d-41e68863d5c4",
		Tag:   "MRHC.play",
	},
	{
		ID:    "tts-loop",
		Label: map[string]string{"ja": "テキスト読み上げループ", "en": "Text-to-speech loop"},
		URL:   "resrec:///U-MarkN/R-019ead5f-846d-7ee4-abb3-1db92b61068a",
		Tag:   "MRHC.play",
	},
}

// builtinSpawnTemplates はスポーン＆パルステンプレの最終フォールバック
// （assets/spawn-templates.json のスナップショット）。
var builtinSpawnTemplates = []itemTemplate{
	{
		ID:    "tts-loop",
		Label: map[string]string{"ja": "テキスト読み上げループ", "en": "Text-to-speech loop"},
		URL:   "resrec:///U-MarkN/R-019ead5f-846d-7ee4-abb3-1db92b61068a",
		Tag:   "MRHC.play",
	},
}

// builtinItemSpawnTemplates は単体スポーンテンプレの最終フォールバック
// （assets/item-spawn-templates.json のスナップショット）。
var builtinItemSpawnTemplates = []itemTemplate{
	{
		ID:    "tts-loop",
		Label: map[string]string{"ja": "テキスト読み上げループ", "en": "Text-to-speech loop"},
		URL:   "resrec:///U-MarkN/R-019ead5f-846d-7ee4-abb3-1db92b61068a",
		Tag:   "MRHC.play",
	},
}

// templateStore は1系統のテンプレリストの取得・キャッシュ・解決器。
type templateStore struct {
	url        string         // 取得元（テストで差し替え）
	client     *http.Client   // 取得用（timeout 込み）
	path       string         // -data の永続キャッシュのフルパス（""=永続なし・テスト等）
	builtin    []itemTemplate // 全フォールバック失敗時の最終手段
	requireTag bool           // tag を必須として検証するか（impulse を送る系統＝告知/スポーン＆パルスは true）

	mu      sync.Mutex
	cache   []itemTemplate // 最終取得成功分のメモリキャッシュ
	fetched time.Time      // cache の取得時刻（TTL 判定。永続分から復元したときはゼロ）
}

func newTemplateStore(url, path string, builtin []itemTemplate, requireTag bool) *templateStore {
	return &templateStore{
		url:        url,
		path:       path,
		builtin:    builtin,
		requireTag: requireTag,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// valid は不正エントリ（id/url が空・requireTag の系統では tag 空も）を除いて返す
// （壊れた1件で全体を捨てない）。label は任意＝UI が id へフォールバックする。
func (st *templateStore) valid(list []itemTemplate) []itemTemplate {
	out := make([]itemTemplate, 0, len(list))
	for _, t := range list {
		if t.ID == "" || t.URL == "" {
			continue
		}
		if st.requireTag && t.Tag == "" {
			continue
		}
		out = append(out, t)
	}
	return out
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
	return st.builtin, "builtin"
}

// lookup は id のテンプレをフォールバック連鎖の結果から探す。
func (st *templateStore) lookup(ctx context.Context, id string) (itemTemplate, bool) {
	list, _ := st.templates(ctx)
	for _, t := range list {
		if t.ID == id {
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
		return a, true
	}
	t, ok := s.announceTpl.lookup(ctx, a.TemplateID)
	if !ok {
		return a, false
	}
	a.ItemURL, a.ImpulseTag = t.URL, t.Tag
	return a, true
}

// writeTemplates は GET テンプレ一覧の共通実装。
// フォールバック連鎖により常に 200 で何かしらの一覧を返す（フロントの選択肢用）。
func writeTemplates(w http.ResponseWriter, r *http.Request, st *templateStore) {
	list, source := st.templates(r.Context())
	writeOK(w, map[string]any{"templates": list, "source": source})
}

// handleAnnounceTemplates: GET /api/v1/announce-templates（告知テンプレ一覧）
func (s *Server) handleAnnounceTemplates(w http.ResponseWriter, r *http.Request) {
	writeTemplates(w, r, s.announceTpl)
}

// handleSpawnTemplates: GET /api/v1/spawn-templates（スポーン＆パルステンプレ一覧）
func (s *Server) handleSpawnTemplates(w http.ResponseWriter, r *http.Request) {
	writeTemplates(w, r, s.spawnTpl)
}

// handleItemSpawnTemplates: GET /api/v1/item-spawn-templates（単体スポーンテンプレ一覧）
func (s *Server) handleItemSpawnTemplates(w http.ResponseWriter, r *http.Request) {
	writeTemplates(w, r, s.itemSpawnTpl)
}
