package server

// favorites（ワールドお気に入り）の単体テスト。状態メソッド（add/remove/list）は HTTP/auth 不要で
// 直接呼ぶ。検証は handleFavoriteAdd を ResponseRecorder で直接叩く（requireAuth を経由しない）。

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

// newFavServer は dataDir=dir（favorites.json の置き場）を持つ driver 未起動 Server を返す。
func newFavServer(t *testing.T, dir string) *Server {
	t.Helper()
	cfgPath := filepath.Join(dir, "mrhc.config.json")
	cfg := &config.Config{Version: config.SchemaVersion, AdminPasswordHash: "x", SessionSecret: "s"}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	return New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
}

func fav(i int) favoriteWorld {
	id := fmt.Sprintf("R-%08d-1111-2222-3333-444455556666", i)
	owner := "U-Owner"
	return favoriteWorld{
		Name:         fmt.Sprintf("World %d", i),
		OwnerID:      owner,
		RecordID:     id,
		ResoniteURL:  "resrec:///" + owner + "/" + id,
		ThumbnailURL: "https://go.resonite.com/x/thumbnail",
	}
}

func TestFavorites_AddListRemove(t *testing.T) {
	s := newFavServer(t, t.TempDir())
	if got := s.listFavorites(); len(got) != 0 {
		t.Fatalf("initial: want 0, got %d", len(got))
	}
	if _, err := s.addFavorite(fav(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.addFavorite(fav(2)); err != nil {
		t.Fatal(err)
	}
	got := s.listFavorites()
	if len(got) != 2 || got[0].RecordID != fav(1).RecordID || got[1].RecordID != fav(2).RecordID {
		t.Fatalf("after add: want [1,2] in order, got %+v", got)
	}
	s.removeFavorite(fav(1).RecordID)
	got = s.listFavorites()
	if len(got) != 1 || got[0].RecordID != fav(2).RecordID {
		t.Fatalf("after remove: want [2], got %+v", got)
	}
}

func TestFavorites_Idempotent(t *testing.T) {
	s := newFavServer(t, t.TempDir())
	_, _ = s.addFavorite(fav(1))
	_, _ = s.addFavorite(fav(1)) // 同 recordId
	if got := s.listFavorites(); len(got) != 1 {
		t.Fatalf("idempotent add: want 1, got %d", len(got))
	}
}

func TestFavorites_RemoveNonexistent(t *testing.T) {
	s := newFavServer(t, t.TempDir())
	_, _ = s.addFavorite(fav(1))
	s.removeFavorite("R-does-not-exist")
	if got := s.listFavorites(); len(got) != 1 {
		t.Fatalf("remove unknown: want 1 unchanged, got %d", len(got))
	}
}

func TestFavorites_Cap(t *testing.T) {
	s := newFavServer(t, t.TempDir())
	for i := 0; i < maxFavorites; i++ {
		if _, err := s.addFavorite(fav(i)); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if _, err := s.addFavorite(fav(9999)); !errors.Is(err, errFavoritesFull) {
		t.Fatalf("over cap: want errFavoritesFull, got %v", err)
	}
	if got := s.listFavorites(); len(got) != maxFavorites {
		t.Fatalf("cap: want %d, got %d", maxFavorites, len(got))
	}
}

func TestFavorites_Persistence(t *testing.T) {
	dir := t.TempDir()
	s1 := newFavServer(t, dir)
	if _, err := s1.addFavorite(fav(1)); err != nil {
		t.Fatal(err)
	}
	// 別インスタンス・同 dataDir → favorites.json から復元される
	s2 := newFavServer(t, dir)
	got := s2.listFavorites()
	if len(got) != 1 || got[0].RecordID != fav(1).RecordID {
		t.Fatalf("persistence: want [1] reloaded, got %+v", got)
	}
}

func TestFavorites_AddValidation(t *testing.T) {
	s := newFavServer(t, t.TempDir())
	post := func(jsonBody string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/favorites", strings.NewReader(jsonBody))
		s.handleFavoriteAdd(rec, req)
		return rec.Code
	}
	if c := post(`{"resoniteUrl":"not-a-resrec","ownerId":"U-x","recordId":"R-y"}`); c != http.StatusBadRequest {
		t.Errorf("bad url: want 400, got %d", c)
	}
	if c := post(`{"resoniteUrl":"resrec:///U-x/R-y","ownerId":"U-x","recordId":"R-y","thumbnailUrl":"http://insecure/x"}`); c != http.StatusBadRequest {
		t.Errorf("http thumbnail: want 400, got %d", c)
	}
	if c := post(`{"name":"W","resoniteUrl":"resrec:///U-x/R-y","ownerId":"U-x","recordId":"R-y","thumbnailUrl":"https://ok/x"}`); c != http.StatusOK {
		t.Errorf("valid: want 200, got %d", c)
	}
	if got := s.listFavorites(); len(got) != 1 {
		t.Fatalf("after valid add: want 1, got %d", len(got))
	}
}
