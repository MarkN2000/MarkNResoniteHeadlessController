package resonite

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 名前検索: /users/?name=<q> が配列を返す → User 一覧に整形・iconUrl 正規化。
func TestSearchUsers_ByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/" || r.URL.Query().Get("name") != "alice" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(`[
			{"id":"U-alice","username":"Alice","profile":{"iconUrl":"resdb:///abc123.webp"}},
			{"id":"U-alice2","username":"AliceTwo","profile":{"iconUrl":""}}
		]`))
	}))
	defer srv.Close()

	users, err := NewClientWithBase(srv.URL).SearchUsers(context.Background(), "alice")
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("want 2 users, got %d: %+v", len(users), users)
	}
	if users[0].ID != "U-alice" || users[0].Username != "Alice" {
		t.Errorf("user0 mismatch: %+v", users[0])
	}
	if users[0].IconURL != "https://assets.resonite.com/abc123" {
		t.Errorf("iconUrl not normalized: %q", users[0].IconURL)
	}
	if users[1].IconURL != "" {
		t.Errorf("empty iconUrl should stay empty, got %q", users[1].IconURL)
	}
}

// ID検索: "U-" 始まりは /users/<id> を単一オブジェクトで返す → 1要素に正規化。
func TestSearchUsers_ByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/U-bob" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"id":"U-bob","username":"Bob","profile":{"iconUrl":"https://cdn.example/bob.png"}}`))
	}))
	defer srv.Close()

	users, err := NewClientWithBase(srv.URL).SearchUsers(context.Background(), "U-bob")
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(users) != 1 || users[0].ID != "U-bob" || users[0].Username != "Bob" {
		t.Fatalf("byID mismatch: %+v", users)
	}
	if users[0].IconURL != "https://cdn.example/bob.png" {
		t.Errorf("http iconUrl should pass through: %q", users[0].IconURL)
	}
}

// 404（ID検索で存在しない等）→ 結果ゼロ（エラーにしない）。
func TestSearchUsers_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	users, err := NewClientWithBase(srv.URL).SearchUsers(context.Background(), "U-none")
	if err != nil {
		t.Fatalf("404 should not error: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("want 0 users, got %+v", users)
	}
}

// 空 term は HTTP を投げずに空を返す。
func TestSearchUsers_EmptyTerm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("must not hit the API for empty term")
	}))
	defer srv.Close()

	users, err := NewClientWithBase(srv.URL).SearchUsers(context.Background(), "   ")
	if err != nil || len(users) != 0 {
		t.Fatalf("empty term: users=%+v err=%v", users, err)
	}
}

func TestConvertIconURL(t *testing.T) {
	cases := map[string]string{
		"resdb:///deadbeef.webp":    "https://assets.resonite.com/deadbeef",
		"resdb:///deadbeef":         "https://assets.resonite.com/deadbeef",
		"https://cdn.example/x.png": "https://cdn.example/x.png",
		"":                          "",
		"ftp://weird/x":             "",
	}
	for in, want := range cases {
		if got := convertIconURL(in); got != want {
			t.Errorf("convertIconURL(%q)=%q want %q", in, got, want)
		}
	}
}
