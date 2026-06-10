package server

// announce_templates.go は告知テンプレート（事前アクションで spawn するアイテム）のリモートリスト
// 取得・キャッシュ・解決を担う。正本はリポジトリ assets/announce-templates.json（main ブランチ）で、
// アイテム URL の差し替え・テンプレ追加をアプリのリリースなしに全インスタンスへ配信する。
// 設計: docs/design/announce-templates.md
//
// フォールバック連鎖（常にこの順・最悪でも焼き込みビルトイン＝従来同等の動作に退化する）:
//   メモリキャッシュ(TTL内) → リモート取得 → -data/announce-templates.json(最終取得分) → ビルトイン

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

// announceTemplatesURL は既定の取得元（main ブランチ＝リリースと独立に更新できる）。
// テストでは Server.tplURL で差し替える。
const announceTemplatesURL = "https://raw.githubusercontent.com/MarkN2000/MarkNResoniteHeadlessController/main/assets/announce-templates.json"

// announceTemplatesTTL はメモリキャッシュの有効期間。raw.githubusercontent.com の
// CDN キャッシュ（約5分）より長めに取り、UI 操作のたびの往復を抑える。
const announceTemplatesTTL = 10 * time.Minute

// announceTemplate は告知テンプレート1件。ID は config（announce.templateId）から参照される
// 永続キー＝リモートリストでの改名・削除は既存 config の参照を切るため運用上禁止
// （アイテムの更新は url の書き換えで行う）。
type announceTemplate struct {
	ID    string            `json:"id"`
	Label map[string]string `json:"label"` // 言語コード→表示名。UI 側で 現在言語→en→ja→先頭→id の順に解決
	URL   string            `json:"url"`   // spawn する resrec:/// URL
	Tag   string            `json:"tag"`   // dynamicimpulsestring のタグ
}

// announceTemplateList は配信 JSON のルート。Version は情報用で互換判定には使わない
// （変更はフィールド追加のみ＝未知キー無視で互換、という前提を配信側が守る）。
type announceTemplateList struct {
	Version   int                `json:"version"`
	Templates []announceTemplate `json:"templates"`
}

// builtinAnnounceTemplates は全フォールバック失敗時の最終手段
// （assets/announce-templates.json のスナップショット）。
// 先頭の ID は config.DefaultRestart() の TemplateID と同期すること。
var builtinAnnounceTemplates = []announceTemplate{
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

// announceTplPath は最終取得分の永続キャッシュの置き場（-data 配下・jsonstate 系）。
func (s *Server) announceTplPath() string {
	return filepath.Join(s.dataDir, "announce-templates.json")
}

// validAnnounceTemplates は id/url/tag いずれか空のエントリを除いて返す
// （壊れた1件で全体を捨てない）。label は任意＝UI が id へフォールバックする。
func validAnnounceTemplates(list []announceTemplate) []announceTemplate {
	out := make([]announceTemplate, 0, len(list))
	for _, t := range list {
		if t.ID != "" && t.URL != "" && t.Tag != "" {
			out = append(out, t)
		}
	}
	return out
}

// fetchAnnounceTemplates はリモートから1回取得する。HTTP/JSON 失敗・有効0件はエラー。
func (s *Server) fetchAnnounceTemplates(ctx context.Context) ([]announceTemplate, error) {
	if s.tplClient == nil || s.tplURL == "" {
		return nil, fmt.Errorf("取得元が未設定")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.tplURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.tplClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var list announceTemplateList
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&list); err != nil {
		return nil, err
	}
	got := validAnnounceTemplates(list.Templates)
	if len(got) == 0 {
		return nil, fmt.Errorf("有効なテンプレートが0件")
	}
	return got, nil
}

// announceTemplates は現在有効なテンプレ一覧と出所（remote|cache|builtin）を返す。
// 取得成功時はメモリ＋永続キャッシュを更新する。失敗時の cache は
// 「過去に取得成功したリスト」（期限切れメモリ または -data の永続分）。
func (s *Server) announceTemplates(ctx context.Context) ([]announceTemplate, string) {
	s.tplMu.Lock()
	defer s.tplMu.Unlock()
	if len(s.tplCache) > 0 && time.Since(s.tplFetched) < announceTemplatesTTL {
		return s.tplCache, "remote"
	}
	got, err := s.fetchAnnounceTemplates(ctx)
	if err == nil {
		s.tplCache, s.tplFetched = got, time.Now()
		if s.dataDir != "" {
			writeJSONFile(s.announceTplPath(), announceTemplateList{Version: 1, Templates: got})
		}
		return got, "remote"
	}
	log.Printf("[announce-templates] リモート取得に失敗（キャッシュへフォールバック）: %v", err)
	if len(s.tplCache) > 0 {
		return s.tplCache, "cache"
	}
	if s.dataDir != "" {
		st := readJSONFile[announceTemplateList](s.announceTplPath())
		if got := validAnnounceTemplates(st.Templates); len(got) > 0 {
			s.tplCache = got // tplFetched はゼロのまま＝次回も再取得を試みる
			return got, "cache"
		}
	}
	return builtinAnnounceTemplates, "builtin"
}

// lookupAnnounceTemplate は id のテンプレをフォールバック連鎖の結果から探す。
func (s *Server) lookupAnnounceTemplate(ctx context.Context, id string) (announceTemplate, bool) {
	list, _ := s.announceTemplates(ctx)
	for _, t := range list {
		if t.ID == id {
			return t, true
		}
	}
	return announceTemplate{}, false
}

// resolveAnnounce は templateId を URL/タグへ解決した AnnounceAction を返す。
// templateId 空（手動入力）はそのまま。未解決（リストから id が消えた等の異常系）は
// ok=false＝呼び出し側（orchestrator）が告知をスキップしてログに残す。
func (s *Server) resolveAnnounce(ctx context.Context, a config.AnnounceAction) (config.AnnounceAction, bool) {
	if a.TemplateID == "" {
		return a, true
	}
	t, ok := s.lookupAnnounceTemplate(ctx, a.TemplateID)
	if !ok {
		return a, false
	}
	a.ItemURL, a.ImpulseTag = t.URL, t.Tag
	return a, true
}

// handleAnnounceTemplates: GET /api/v1/announce-templates
// フォールバック連鎖により常に 200 で何かしらの一覧を返す（フロントの選択肢用）。
func (s *Server) handleAnnounceTemplates(w http.ResponseWriter, r *http.Request) {
	list, source := s.announceTemplates(r.Context())
	writeOK(w, map[string]any{"templates": list, "source": source})
}
