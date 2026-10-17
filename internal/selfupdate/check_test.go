package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckPrereleases(t *testing.T) {
	for _, tc := range []struct {
		name, stable, current, minimum, want string
		include, available                   bool
	}{
		{"既定は安定版", "v2.1.0", "v2.0.0", "", "v2.1.0", false, true},
		{"順序によらず最大版", "v2.1.0", "v2.0.0", "", "v2.2.0-beta.10", true, true},
		{"安定版未公開", "", "v2.0.0", "", "v2.2.0-beta.10", true, true},
		{"フィードより安定版が新しい", "v2.3.0", "v2.0.0", "", "v2.3.0", true, true},
		{"オフにしても降格しない", "v2.1.0", "v2.2.0-beta.10", "", "v2.1.0", false, false},
		{"正式版へ移行", "v2.2.0", "v2.2.0-beta.10", "", "v2.2.0", false, true},
		{"適用済みより古い版を提示しない", "v2.1.0", "v2.0.0", "v2.2.0-beta.10", "v2.1.0", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/releases/latest" {
					if tc.stable == "" {
						http.NotFound(w, r)
						return
					}
					w.Header().Set("Location", "/releases/tag/"+tc.stable)
					w.WriteHeader(http.StatusFound)
					return
				}
				if r.URL.Path != "/releases.atom" || !tc.include {
					t.Errorf("想定外のリクエスト: %s", r.URL)
					http.NotFound(w, r)
					return
				}
				fmt.Fprint(w, `<feed xmlns="http://www.w3.org/2005/Atom">`)
				for _, tag := range []string{"v2.2.0-beta.10", "nightly", "v2.1.0", "v2.2.0-beta.2", "v9.0.0%2F.."} {
					fmt.Fprintf(w, `<entry><link rel="alternate" href="http://%s/releases/tag/%s"/></entry>`, r.Host, tag)
				}
				fmt.Fprint(w, `<entry><link rel="alternate" href="https://example.com/releases/tag/v99.0.0"/></entry></feed>`)
			}))
			defer srv.Close()
			u := newTestUpdater(srv.URL, tc.current, "")
			u.IncludePrereleases, u.MinimumVersion = tc.include, tc.minimum
			info, err := u.Check(context.Background())
			if err != nil || info.Latest != tc.want || info.UpdateAvailable != tc.available || info.IncludePrereleases != tc.include {
				t.Fatalf("info=%+v err=%v", info, err)
			}
		})
	}
}

func TestCheckPrereleaseFeedErrors(t *testing.T) {
	for _, body := range []string{"<feed>", "<html/>", strings.Repeat("x", (4<<20)+1), `<feed xmlns="http://www.w3.org/2005/Atom"/>`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/releases/latest" {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, body)
		}))
		u := newTestUpdater(srv.URL, "v2.0.0", "")
		u.IncludePrereleases = true
		_, err := u.Check(context.Background())
		srv.Close()
		if err == nil {
			t.Fatal("不正・空フィードを成功扱いしました")
		}
	}
}

func TestCheckUpdateAvailable(t *testing.T) {
	srv := serveRelease(t, "v2.1.0", nil)
	u := newTestUpdater(srv.URL, "v2.0.0", "")
	info, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Info{Current: "v2.0.0", Latest: "v2.1.0", UpdateAvailable: true, CurrentIsRelease: true}
	if info != want {
		t.Errorf("info = %+v, want %+v", info, want)
	}
}

func TestCheckUpToDate(t *testing.T) {
	srv := serveRelease(t, "v2.0.0", nil)
	u := newTestUpdater(srv.URL, "v2.0.0", "")
	info, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.UpdateAvailable {
		t.Errorf("UpdateAvailable = true, want false")
	}
}

// latest が現行より古い（latest の付け替え等）場合も「更新あり」にしない＝ダウングレード防止。
func TestCheckOlderLatest(t *testing.T) {
	srv := serveRelease(t, "v1.9.0", nil)
	u := newTestUpdater(srv.URL, "v2.0.0", "")
	info, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.UpdateAvailable {
		t.Errorf("UpdateAvailable = true, want false（ダウングレードは提示しない）")
	}
}

// 非 semver の焼込（dev・ブランチ名）は適用不可ビルドとして報告される。
func TestCheckNonReleaseBuild(t *testing.T) {
	for _, version := range []string{"dev", "rewrite"} {
		srv := serveRelease(t, "v2.1.0", nil)
		u := newTestUpdater(srv.URL, version, "")
		info, err := u.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.CurrentIsRelease || info.UpdateAvailable {
			t.Errorf("version=%q: CurrentIsRelease=%v UpdateAvailable=%v, want false/false",
				version, info.CurrentIsRelease, info.UpdateAvailable)
		}
		if info.Latest != "v2.1.0" {
			t.Errorf("version=%q: Latest=%q（最新の表示はできるべき）", version, info.Latest)
		}
	}
}

// リリース未公開（latest が 404）は ErrNoRelease。
func TestCheckNoRelease(t *testing.T) {
	srv := serveRelease(t, "", nil)
	u := newTestUpdater(srv.URL, "v2.0.0", "")
	if _, err := u.Check(context.Background()); !errors.Is(err, ErrNoRelease) {
		t.Errorf("err = %v, want ErrNoRelease", err)
	}
}

// タグ URL でないリダイレクト（リポジトリ改名の 301 等）・非 semver タグはエラー。
func TestCheckBadRedirect(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"タグでないLocation": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/NewName/NewRepo")
			w.WriteHeader(http.StatusMovedPermanently)
		},
		"非semverタグ": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/releases/tag/nightly")
			w.WriteHeader(http.StatusFound)
		},
		"リダイレクトなし200": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			u := newTestUpdater(srv.URL, "v2.0.0", "")
			if _, err := u.Check(context.Background()); err == nil {
				t.Error("err = nil, want error")
			}
		})
	}
}
