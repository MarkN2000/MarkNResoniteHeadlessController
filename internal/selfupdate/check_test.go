package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
