// Package resonite は Resonite 公開クラウドAPI（api.resonite.com）への薄いクライアント。
// 現状はユーザー検索のみ（無認証・公開エンドポイント）。フレンド申請/招待の「相手探し」に使う。
// world 検索（go.resonite.com）は対象外（docs/DESIGN.md §Won't）。
//
// エンドポイント（v1 実装・無認証で確認済）:
//   - 名前検索: GET /users/?name=<q>  → ユーザー配列
//   - ID検索:   GET /users/<U-id>     → 単一ユーザー
package resonite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.resonite.com"
	assetsBaseURL  = "https://assets.resonite.com"
	userAgent      = "MRHC/2.0"
	httpTimeout    = 8 * time.Second
	maxBodyBytes   = 1 << 20 // 1 MiB（公開APIの応答は十分小さい・安全網）
)

// User は検索結果1件（UI に必要な最小限）。
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	IconURL  string `json:"iconUrl"` // https に正規化済（空可）
}

// Client は Resonite 公開API クライアント。baseURL はテストで差し替え可能。
type Client struct {
	http    *http.Client
	baseURL string
}

// NewClient は本番用クライアント（api.resonite.com）を返す。
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: httpTimeout}, baseURL: defaultBaseURL}
}

// NewClientWithBase はテスト用に baseURL を差し替えたクライアントを返す。
func NewClientWithBase(base string) *Client {
	return &Client{http: &http.Client{Timeout: httpTimeout}, baseURL: base}
}

// apiUser は /users レスポンスのうち必要なフィールドだけ拾う。
type apiUser struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	NormalizedUsername string `json:"normalizedUsername"`
	Profile            struct {
		IconURL string `json:"iconUrl"`
	} `json:"profile"`
}

// SearchUsers は term でユーザーを検索する。
//   - term が "U-" 始まり → GET /users/<term>（単一・1要素に正規化）
//   - それ以外            → GET /users/?name=<term>（配列）
//
// 見つからない（404 / 空配列）は「結果ゼロ」として nil, nil を返す（エラーにしない）。
func (c *Client) SearchUsers(ctx context.Context, term string) ([]User, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	byID := strings.HasPrefix(term, "U-")

	var url string
	if byID {
		url = c.baseURL + "/users/" + neturl.PathEscape(term)
	} else {
		url = c.baseURL + "/users/?name=" + neturl.QueryEscape(term)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // ID検索で存在しない等 → 結果ゼロ
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resonite api: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}

	var raws []apiUser
	if byID {
		var one apiUser
		if err := json.Unmarshal(body, &one); err != nil {
			return nil, fmt.Errorf("resonite api: decode user: %w", err)
		}
		if one.ID == "" {
			return nil, nil
		}
		raws = []apiUser{one}
	} else if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("resonite api: decode users: %w", err)
	}

	out := make([]User, 0, len(raws))
	for _, r := range raws {
		name := r.Username
		if name == "" {
			name = r.NormalizedUsername
		}
		out = append(out, User{ID: r.ID, Username: name, IconURL: convertIconURL(r.Profile.IconURL)})
	}
	return out, nil
}

// convertIconURL は Resonite の resdb:///<hash>.<ext> を https://assets.resonite.com/<hash> に
// 正規化する。既に http(s) ならそのまま。それ以外（空・未知スキーム）は "" を返す。
func convertIconURL(u string) string {
	if id, ok := strings.CutPrefix(u, "resdb:///"); ok {
		if i := strings.LastIndexByte(id, '.'); i >= 0 {
			id = id[:i] // 拡張子除去（assets は拡張子不要）
		}
		if id == "" {
			return ""
		}
		return assetsBaseURL + "/" + id
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return ""
}
