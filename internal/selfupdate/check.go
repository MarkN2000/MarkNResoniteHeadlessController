// check.go は最新リリースタグの解決とバージョン比較（更新有無の判定）を担う。
package selfupdate

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/mod/semver"
)

// Info は更新チェックの結果。
type Info struct {
	IncludePrereleases bool   // 確認に使用した選択（キャッシュの混同防止）
	Current            string // 実行中バイナリの焼込バージョン（そのまま）
	Latest             string // 最新リリースタグ（例 "v2.1.0"）
	UpdateAvailable    bool   // Latest > Current（semver 比較）。Current が非 semver なら常に false
	CurrentIsRelease   bool   // Current が semver として有効＝適用可能なビルドか
}

// Check は最新リリースを調べ、現行版との比較結果を返す。
// GitHub API は使わず releases/latest と、選択時は releases.atom からタグを読む
// （API の未認証レート制限 60req/h を受けないため。Web エンドポイントは対象外）。
func (u *Updater) Check(ctx context.Context) (Info, error) {
	latest, err := u.resolveLatestTag(ctx)
	if err != nil {
		return Info{}, err
	}
	info := Info{
		IncludePrereleases: u.IncludePrereleases,
		Current:            u.Version,
		Latest:             latest,
		CurrentIsRelease:   semver.IsValid(u.Version),
	}
	// 非 semver ビルド（dev・workflow_dispatch のブランチ名焼込）は比較不能＝更新は提示しない。
	// 「最新が現行より新しい」場合のみ true（latest を過去版へ付け替えてもダウングレードさせない）。
	info.UpdateAvailable = info.CurrentIsRelease && semver.Compare(latest, u.Version) > 0 && semver.Compare(latest, u.MinimumVersion) > 0
	return info, nil
}

// resolveLatestTag は選択に応じて安定版と公開フィードの最大バージョンを返す。
func (u *Updater) resolveLatestTag(ctx context.Context) (string, error) {
	latest, err := u.resolveStableTag(ctx)
	if !u.IncludePrereleases || (err != nil && !errors.Is(err, ErrNoRelease)) {
		return latest, err
	}
	resp, err := u.get(ctx, strings.TrimSuffix(u.BaseURL, "/")+"/releases.atom")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var feed struct {
		XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
		Entries []struct {
			Links []struct {
				Href string `xml:"href,attr"`
				Rel  string `xml:"rel,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return "", fmt.Errorf("リリースフィードの読み取りに失敗: %w", err)
	}
	if len(data) > 4<<20 {
		return "", fmt.Errorf("リリースフィードがサイズ上限（4 MiB）を超えています")
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return "", fmt.Errorf("リリースフィードの解析に失敗: %w", err)
	}
	// ponytail: フィード掲載分だけを探索する。全履歴が必要になったら配布側に更新一覧を用意する。
	prefix := strings.TrimSuffix(u.BaseURL, "/") + "/releases/tag/"
	for _, entry := range feed.Entries {
		for _, link := range entry.Links {
			if link.Rel != "alternate" || !strings.HasPrefix(link.Href, prefix) {
				continue
			}
			tag, err := url.PathUnescape(strings.TrimPrefix(link.Href, prefix))
			if err == nil && semver.IsValid(tag) && semver.Compare(tag, latest) > 0 {
				latest = tag
			}
		}
	}
	if latest == "" {
		return "", ErrNoRelease
	}
	return latest, nil
}

// resolveStableTag は releases/latest の Location ヘッダから安定版タグを返す。
func (u *Updater) resolveStableTag(ctx context.Context) (string, error) {
	reqURL := strings.TrimSuffix(u.BaseURL, "/") + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "MRHC")
	resp, err := u.CheckClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("最新リリースの確認に失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNoRelease
	}
	loc := resp.Header.Get("Location")
	// 想定は 302 →（リポジトリ改名時はタグ URL でない 301 が先に来るため marker 検証で弾く）
	const marker = "/releases/tag/"
	i := strings.LastIndex(loc, marker)
	if resp.StatusCode/100 != 3 || i < 0 {
		return "", fmt.Errorf("最新リリースの確認に失敗: 予期しない応答 HTTP %d (Location=%q)", resp.StatusCode, loc)
	}
	tag, err := url.PathUnescape(loc[i+len(marker):])
	if err != nil || !semver.IsValid(tag) {
		return "", fmt.Errorf("リリースタグを解釈できません: %q", loc)
	}
	return tag, nil
}
