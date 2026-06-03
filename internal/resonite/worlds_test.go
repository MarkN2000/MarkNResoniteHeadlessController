package resonite

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fixtureWorldsHTML は go.resonite.com/world?term= の実 HTML 構造（2026-06-04 採取）を縮小したもの。
//   - 1件目: U- 所有・相対サムネ（origin で絶対化されること）
//   - 2件目: G- 所有・プレースホルダ相対サムネ
//   - 3件目: 名前に HTML エンティティ（&amp; → & に復号されること）・サムネ無し
//   - 4件目: href に R- が無い壊れた要素（スキップされること）
const fixtureWorldsHTML = `<!DOCTYPE html><html><body>
<ol class="listing">
  <li><a class="listing-item" href="/world/U-Sharkmare/R-019e61d4-2dfe-7b6e-8c4d-f0d67db009b4">
    <img src="/world/U-Sharkmare/R-019e61d4-2dfe-7b6e-8c4d-f0d67db009b4/thumbnail" alt="">
    <h2 class="listing-item__heading"><span>Card Game Example World</span></h2>
  </a></li>
  <li><a class="listing-item" href="/world/G-1StJATqZXay/R-019d60b3-0317-7698-ac9c-ffe5854922d9">
    <img src="/images/resonite.png" alt="">
    <h2 class="listing-item__heading"><span>Group Hangout</span></h2>
  </a></li>
  <li><a class="listing-item" href="/world/U-torazo/R-6ab61ab2-dc71-4ec5-badc-ad6c9fa3e0ea">
    <h2 class="listing-item__heading"><span>Tom &amp; Jerry</span></h2>
  </a></li>
  <li><a class="listing-item" href="/user/U-NoRecord">
    <h2 class="listing-item__heading"><span>Broken (no record)</span></h2>
  </a></li>
</ol>
</body></html>`

func serveHTML(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
}

func TestSearchWorlds_Parse(t *testing.T) {
	srv := serveHTML(fixtureWorldsHTML)
	defer srv.Close()

	worlds, err := NewClientWithBases("", srv.URL).SearchWorlds(context.Background(), "game")
	if err != nil {
		t.Fatalf("SearchWorlds error: %v", err)
	}
	if len(worlds) != 3 { // 壊れた4件目はスキップ
		t.Fatalf("want 3 worlds, got %d: %+v", len(worlds), worlds)
	}

	// 1件目: U- 所有・相対サムネが origin で絶対化される
	w0 := worlds[0]
	if w0.Name != "Card Game Example World" {
		t.Errorf("w0.Name = %q", w0.Name)
	}
	if w0.OwnerID != "U-Sharkmare" || w0.RecordID != "R-019e61d4-2dfe-7b6e-8c4d-f0d67db009b4" {
		t.Errorf("w0 ids = %q / %q", w0.OwnerID, w0.RecordID)
	}
	if w0.ResoniteURL != "resrec:///U-Sharkmare/R-019e61d4-2dfe-7b6e-8c4d-f0d67db009b4" {
		t.Errorf("w0.ResoniteURL = %q", w0.ResoniteURL)
	}
	if !strings.HasPrefix(w0.ThumbnailURL, srv.URL+"/") || !strings.HasSuffix(w0.ThumbnailURL, "/thumbnail") {
		t.Errorf("w0.ThumbnailURL not absolutized: %q", w0.ThumbnailURL)
	}

	// 2件目: G- 所有
	if worlds[1].OwnerID != "G-1StJATqZXay" {
		t.Errorf("w1.OwnerID = %q", worlds[1].OwnerID)
	}
	if worlds[1].ResoniteURL != "resrec:///G-1StJATqZXay/R-019d60b3-0317-7698-ac9c-ffe5854922d9" {
		t.Errorf("w1.ResoniteURL = %q", worlds[1].ResoniteURL)
	}

	// 3件目: エンティティ復号・サムネ無し
	if worlds[2].Name != "Tom & Jerry" {
		t.Errorf("w2.Name = %q (want decoded entity)", worlds[2].Name)
	}
	if worlds[2].ThumbnailURL != "" {
		t.Errorf("w2.ThumbnailURL = %q (want empty)", worlds[2].ThumbnailURL)
	}
}

func TestSearchWorlds_EmptyTerm(t *testing.T) {
	worlds, err := NewClientWithBases("", "http://unused.invalid").SearchWorlds(context.Background(), "  ")
	if err != nil || worlds != nil {
		t.Fatalf("empty term: want nil,nil got %+v, %v", worlds, err)
	}
}

func TestSearchWorlds_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if _, err := NewClientWithBases("", srv.URL).SearchWorlds(context.Background(), "game"); err == nil {
		t.Fatal("want error on non-200 upstream, got nil")
	}
}

func TestSearchWorlds_NoResults(t *testing.T) {
	srv := serveHTML(`<html><body><ol class="listing"></ol></body></html>`)
	defer srv.Close()

	worlds, err := NewClientWithBases("", srv.URL).SearchWorlds(context.Background(), "zzz")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(worlds) != 0 {
		t.Fatalf("want 0 worlds, got %d", len(worlds))
	}
}

func TestSearchWorlds_CapAt24(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<html><body><ol class="listing">`)
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&b, `<li><a class="listing-item" href="/world/U-Owner%d/R-rec%d">`+
			`<h2 class="listing-item__heading"><span>World %d</span></h2></a></li>`, i, i, i)
	}
	b.WriteString(`</ol></body></html>`)
	srv := serveHTML(b.String())
	defer srv.Close()

	worlds, err := NewClientWithBases("", srv.URL).SearchWorlds(context.Background(), "many")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(worlds) != maxWorldResults {
		t.Fatalf("want cap %d, got %d", maxWorldResults, len(worlds))
	}
}
