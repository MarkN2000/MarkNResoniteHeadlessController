package resonite

// worlds.go は go.resonite.com の公開ワールド一覧（HTML）をスクレイピングして
// 検索結果を返す。公式 api.resonite.com にワールド検索が無いための代替で、
// HTML 構造変更で壊れ得る点は受容（docs/DESIGN.md Should・phase-7-spec §3.12）。
//
// 抽出規則（実機 HTML で確認・2026-06-04）:
//   GET go.resonite.com/world?term=<q>
//     ol.listing li a.listing-item       … 1件のアンカー
//       h2.listing-item__heading のテキスト … ワールド名
//       href 内 /R-xxx                      … レコードID
//       href 内 /(U|G)-xxx                  … 所有者ID（ユーザー or グループ）
//       img src（相対は origin で絶対化）   … サムネ（空可）
//   → resrec:///<owner>/<record> を組み立て、既存の startworldurl 経路へ流せる形にする。

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// maxWorldResults は1検索で返す上限。サムネ画像の読み込み・描画負荷を抑えるため
// go.resonite.com の並び順上位 N 件で打ち切る（UI 仕様・phase-7-spec §3.12）。
const maxWorldResults = 24

// World は go.resonite.com のワールド検索結果1件（起動に必要な最小限）。
type World struct {
	Name         string `json:"name"`         // ワールド名（HTMLエンティティ復号済）
	OwnerID      string `json:"ownerId"`      // U-xxx（ユーザー）または G-xxx（グループ）
	RecordID     string `json:"recordId"`     // R-xxx
	ResoniteURL  string `json:"resoniteUrl"`  // resrec:///<owner>/<record>（起動に渡す値）
	ThumbnailURL string `json:"thumbnailUrl"` // https 絶対URL（空可）
}

var (
	reRecordID = regexp.MustCompile(`/(R-[A-Za-z0-9_-]+)`)
	reOwnerID  = regexp.MustCompile(`/((?:U|G)-[A-Za-z0-9_-]+)`)
)

// SearchWorlds は term で go.resonite.com の公開ワールドを検索する。
// term 空は nil,nil（呼び出し側で 400）。結果ゼロは空スライス。非200は error（呼び出し側で 502）。
// 上位 maxWorldResults 件で打ち切る。
func (c *Client) SearchWorlds(ctx context.Context, term string) ([]World, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	url := c.worldsBaseURL + "/world?term=" + neturl.QueryEscape(term)
	body, status, err := c.get(ctx, url, "text/html")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("go.resonite.com: status %d", status)
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("go.resonite.com: parse html: %w", err)
	}

	isListingItem := func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "a" && hasClass(n, "listing-item")
	}
	out := make([]World, 0, maxWorldResults)
	for _, a := range findAll(doc, isListingItem) {
		if w, ok := parseListingItem(a, c.worldsBaseURL); ok {
			out = append(out, w)
			if len(out) >= maxWorldResults {
				break
			}
		}
	}
	return out, nil
}

// parseListingItem は a.listing-item ノード1つから World を抽出する。
// name / ownerId / recordId のいずれかが欠ける要素は ok=false（呼び出し側でスキップ）。
func parseListingItem(a *html.Node, origin string) (World, bool) {
	href := attrValue(a, "href")
	rec := reRecordID.FindStringSubmatch(href)
	owner := reOwnerID.FindStringSubmatch(href)
	if rec == nil || owner == nil {
		return World{}, false
	}

	heading := findFirst(a, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "h2" && hasClass(n, "listing-item__heading")
	})
	name := ""
	if heading != nil {
		name = textContent(heading)
	}
	if name == "" {
		return World{}, false
	}

	thumb := ""
	if img := findFirst(a, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "img"
	}); img != nil {
		src := attrValue(img, "src")
		if src == "" {
			src = attrValue(img, "data-src")
		}
		thumb = absolutize(src, origin)
	}

	ownerID, recordID := owner[1], rec[1]
	return World{
		Name:         name,
		OwnerID:      ownerID,
		RecordID:     recordID,
		ResoniteURL:  "resrec:///" + ownerID + "/" + recordID,
		ThumbnailURL: thumb,
	}, true
}

// absolutize は go.resonite.com の相対URL（/...）を origin 付き絶対URLに正規化する。
// 既に http(s) はそのまま、`//` は https: 付与、それ以外（空・未知形式）は ""（描画しない）。
func absolutize(u, origin string) string {
	u = strings.TrimSpace(u)
	switch {
	case u == "":
		return ""
	case strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://"):
		return u
	case strings.HasPrefix(u, "//"):
		return "https:" + u
	case strings.HasPrefix(u, "/"):
		return origin + u
	default:
		return ""
	}
}

// --- html ノード走査ヘルパ（worlds.go 専用の小道具） ---

// findAll は n を根に深さ優先で走査し pred を満たすノードを全て返す（n 自身も対象）。
func findAll(n *html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if pred(node) {
			out = append(out, node)
		}
		for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return out
}

// findFirst は pred を満たす最初の子孫ノードを返す（n 自身は対象外・無ければ nil）。
func findFirst(n *html.Node, pred func(*html.Node) bool) *html.Node {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if pred(ch) {
			return ch
		}
		if got := findFirst(ch, pred); got != nil {
			return got
		}
	}
	return nil
}

// attrValue は要素ノードの指定属性値を返す（無ければ ""）。
func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// hasClass は class 属性に token がスペース区切りトークンとして含まれるか。
func hasClass(n *html.Node, token string) bool {
	for _, f := range strings.Fields(attrValue(n, "class")) {
		if f == token {
			return true
		}
	}
	return false
}

// textContent は n 配下の全テキストを連結しトリムして返す（h2 配下の span 名前取得用）。
func textContent(n *html.Node) string {
	var b strings.Builder
	for _, t := range findAll(n, func(x *html.Node) bool { return x.Type == html.TextNode }) {
		b.WriteString(t.Data)
	}
	return strings.TrimSpace(b.String())
}
