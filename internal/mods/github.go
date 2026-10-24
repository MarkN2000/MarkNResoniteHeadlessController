package mods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrDownload = errors.New("GitHubから取得できませんでした")
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$`)

func Repository(input string) (string, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "https://") {
		u, err := url.Parse(input)
		if err != nil || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", ErrInvalid
		}
		input = strings.Trim(u.Path, "/")
	}
	input = strings.TrimSuffix(input, ".git")
	if !repoPattern.MatchString(input) || strings.HasSuffix(input, "/.") || strings.HasSuffix(input, "/..") {
		return "", ErrInvalid
	}
	return input, nil
}

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}
type Releases struct {
	Releases []Release `json:"releases"`
	HasMore  bool      `json:"hasMore"`
}

type GitHub struct{ Client *http.Client }

func (g GitHub) get(ctx context.Context, address string, max int64) ([]byte, error) {
	client := http.Client{Timeout: 60 * time.Second}
	if g.Client != nil {
		client = *g.Client
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
			return ErrDownload
		}
		switch req.URL.Host {
		case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
			return nil
		}
		return ErrDownload
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MRHC-Mod-Manager")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDownload, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrDownload, resp.StatusCode)
	}
	if resp.ContentLength > max {
		return nil, fmt.Errorf("%w: サイズ上限を超えています", ErrDownload)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil || int64(len(b)) > max {
		return nil, fmt.Errorf("%w: 受信失敗またはサイズ超過", ErrDownload)
	}
	return b, nil
}

func (g GitHub) Release(ctx context.Context, repo, tag string, includePrereleases bool) (Release, error) {
	var r Release
	repo, err := Repository(repo)
	if err != nil {
		return r, err
	}
	suffix := "latest"
	if tag != "" {
		suffix = "tags/" + url.PathEscape(tag)
	}
	b, err := g.get(ctx, "https://api.github.com/repos/"+repo+"/releases/"+suffix, 4<<20)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("%w: 応答形式が不正です", ErrDownload)
	}
	if r.Draft || (r.Prerelease && !includePrereleases) || r.Tag == "" {
		return r, fmt.Errorf("%w: 指定したリリースは利用できません", ErrInvalid)
	}
	return r, nil
}

func (g GitHub) List(ctx context.Context, repo string, page int, includePrereleases bool) (Releases, error) {
	out := Releases{Releases: []Release{}}
	repo, err := Repository(repo)
	if err != nil {
		return out, err
	}
	if page < 1 || page > 1000 {
		return out, ErrInvalid
	}
	b, err := g.get(ctx, "https://api.github.com/repos/"+repo+"/releases?per_page=30&page="+strconv.Itoa(page), 4<<20)
	if err != nil {
		return out, err
	}
	var rs []Release
	if err = json.Unmarshal(b, &rs); err != nil {
		return out, fmt.Errorf("%w: 応答形式が不正です", ErrDownload)
	}
	out.HasMore = len(rs) == 30
	if page == 1 && len(rs) > 0 && !includePrereleases {
		latest, e := g.Release(ctx, repo, "", false)
		if e != nil {
			return out, e
		}
		out.Releases = append(out.Releases, latest)
	}
	for _, r := range rs {
		if r.Draft || (r.Prerelease && !includePrereleases) || r.Tag == "" {
			continue
		}
		if len(out.Releases) > 0 && r.Tag == out.Releases[0].Tag {
			continue
		}
		out.Releases = append(out.Releases, r)
	}
	for i := range out.Releases {
		assets := []Asset{}
		for _, a := range out.Releases[i].Assets {
			if ValidateName(a.Name) == nil {
				assets = append(assets, a)
			}
		}
		out.Releases[i].Assets = assets
	}
	return out, nil
}

func (r Release) Asset(name string) (Asset, error) {
	if ValidateName(name) != nil {
		return Asset{}, ErrInvalid
	}
	for _, a := range r.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("%w: 添付DLLを選択してください", ErrInvalid)
}

func (g GitHub) Download(ctx context.Context, repo string, r Release, name string) ([]byte, error) {
	repo, err := Repository(repo)
	if err != nil {
		return nil, err
	}
	a, err := r.Asset(name)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(a.URL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.EqualFold(u.Path, "/"+repo+"/releases/download/"+r.Tag+"/"+a.Name) {
		return nil, fmt.Errorf("%w: 配布URLが不正です", ErrDownload)
	}
	if a.Size <= 0 || a.Size > MaxDLL {
		return nil, fmt.Errorf("%w: DLLは32 MiBまでです", ErrInvalid)
	}
	b, err := g.get(ctx, a.URL, MaxDLL)
	if err == nil && int64(len(b)) != a.Size {
		err = fmt.Errorf("%w: 受信サイズが一致しません", ErrDownload)
	}
	return b, err
}
