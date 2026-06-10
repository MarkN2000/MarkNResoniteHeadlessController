package server

// announce_templates（templateStore 2系統）のテスト: フォールバック連鎖（remote→永続→builtin）・
// TTL・不正エントリのスキップ・templateId 解決・PUT 検証・orchestrator の実行時解決・
// スポーン＆パルスエンドポイント。PUT は favorites_test と同様 ResponseRecorder で
// ハンドラ直叩き（requireAuth を経由しない）。spawn-impulse は fakehl 統合（newTestServerFull）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

// tplJSON はリモートリストの正常系フィクスチャ（torazo-close の URL をビルトインから差し替え済み
// ＝「アイテム更新が配信される」シナリオ）。
const tplJSON = `{"version":1,"templates":[
	{"id":"torazo-close","label":{"ja":"とらぞ","en":"Torazo"},"url":"resrec:///U-MarkN/R-updated","tag":"MRHC.play"},
	{"id":"extra","label":{"ja":"追加テンプレ"},"url":"resrec:///U-MarkN/R-extra","tag":"MRHC.extra"}
]}`

// deadURL は確実に取得失敗する取得元（クローズ済みサーバー）＝オフライン相当。
func deadURL() string {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	return dead.URL
}

// newTplServer は dataDir=tmp の driver 未起動 Server を返し、告知テンプレの取得元を remote へ
// 差し替える。remote=nil はオフライン相当（deadURL）。
func newTplServer(t *testing.T, remote http.HandlerFunc) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mrhc.config.json")
	cfg := &config.Config{Version: config.SchemaVersion, AdminPasswordHash: "x", SessionSecret: "s"}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	s := New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	if remote == nil {
		s.announceTpl.url = deadURL()
	} else {
		ts := httptest.NewServer(remote)
		t.Cleanup(ts.Close)
		s.announceTpl.url = ts.URL
	}
	return s, dir
}

func TestItemTemplates_RemoteSuccessAndTTL(t *testing.T) {
	calls := 0
	s, dir := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(tplJSON))
	})
	list, source := s.announceTpl.templates(context.Background())
	if source != "remote" || len(list) != 2 {
		t.Fatalf("remote 取得が想定外: source=%s list=%+v", source, list)
	}
	if list[0].URL != "resrec:///U-MarkN/R-updated" {
		t.Fatalf("配信された新 URL になっていない: %+v", list[0])
	}
	// 最終取得分が -data に永続化されている
	if _, err := os.Stat(filepath.Join(dir, "announce-templates.json")); err != nil {
		t.Fatalf("永続キャッシュ未作成: %v", err)
	}
	// TTL 内の再呼び出しは再取得しない（メモリキャッシュ）
	if _, _ = s.announceTpl.templates(context.Background()); calls != 1 {
		t.Fatalf("TTL 内で再取得された: calls=%d", calls)
	}
}

func TestItemTemplates_FallbackToPersisted(t *testing.T) {
	s, dir := newTplServer(t, nil) // 取得は必ず失敗
	writeJSONFile(filepath.Join(dir, "announce-templates.json"), itemTemplateList{
		Version:   1,
		Templates: []itemTemplate{{ID: "saved", URL: "resrec:///saved", Tag: "T"}},
	})
	list, source := s.announceTpl.templates(context.Background())
	if source != "cache" || len(list) != 1 || list[0].ID != "saved" {
		t.Fatalf("永続キャッシュへフォールバックしない: source=%s list=%+v", source, list)
	}
}

func TestItemTemplates_FallbackToBuiltin(t *testing.T) {
	s, _ := newTplServer(t, nil) // 取得失敗・永続キャッシュなし
	list, source := s.announceTpl.templates(context.Background())
	if source != "builtin" || len(list) != len(builtinAnnounceTemplates) {
		t.Fatalf("ビルトインへフォールバックしない: source=%s list=%+v", source, list)
	}
	// 既定 config の templateId はビルトインで必ず解決できる（DefaultRestart との同期検証）
	id := config.DefaultRestart().PreActions.Announce.TemplateID
	if _, ok := s.announceTpl.lookup(context.Background(), id); !ok {
		t.Fatalf("既定 templateId %q がビルトインに無い（同期切れ）", id)
	}
}

func TestItemTemplates_InvalidEntriesSkipped(t *testing.T) {
	s, _ := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":1,"templates":[
			{"id":"ok","url":"resrec:///ok","tag":"T"},
			{"id":"","url":"resrec:///no-id","tag":"T"},
			{"id":"no-url","url":"","tag":"T"},
			{"id":"no-tag","url":"resrec:///x","tag":""}
		]}`))
	})
	list, source := s.announceTpl.templates(context.Background())
	if source != "remote" || len(list) != 1 || list[0].ID != "ok" {
		t.Fatalf("不正エントリがスキップされない: source=%s list=%+v", source, list)
	}
}

func TestItemTemplates_AllInvalidFallsBack(t *testing.T) {
	s, _ := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":1,"templates":[{"id":"","url":"","tag":""}]}`))
	})
	if _, source := s.announceTpl.templates(context.Background()); source != "builtin" {
		t.Fatalf("有効0件は取得失敗扱いのはず: source=%s", source)
	}
}

// 2系統は独立したキャッシュ/取得元を持つ（announce の取得が spawn に混ざらない）。
func TestItemTemplates_TwoStoresIndependent(t *testing.T) {
	s, _ := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tplJSON))
	})
	s.spawnTpl.url = deadURL()
	if list, source := s.announceTpl.templates(context.Background()); source != "remote" || len(list) != 2 {
		t.Fatalf("announce 側が remote にならない: %s %+v", source, list)
	}
	list, source := s.spawnTpl.templates(context.Background())
	if source != "builtin" || len(list) != 1 || list[0].ID != "tts-loop" {
		t.Fatalf("spawn 側がビルトイン(tts-loop)にならない: %s %+v", source, list)
	}
}

func TestResolveAnnounce(t *testing.T) {
	s, _ := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tplJSON))
	})
	ctx := context.Background()
	// 手動入力（templateId 空）はそのまま通す
	manual := config.AnnounceAction{ItemURL: "resrec:///manual", ImpulseTag: "tag"}
	if got, ok := s.resolveAnnounce(ctx, manual); !ok || got != manual {
		t.Fatalf("手動入力が変更された: ok=%v got=%+v", ok, got)
	}
	// テンプレ参照は URL/タグが解決される
	got, ok := s.resolveAnnounce(ctx, config.AnnounceAction{TemplateID: "extra", Message: "m"})
	if !ok || got.ItemURL != "resrec:///U-MarkN/R-extra" || got.ImpulseTag != "MRHC.extra" || got.Message != "m" {
		t.Fatalf("テンプレ解決が想定外: ok=%v got=%+v", ok, got)
	}
	// 未知 id は ok=false（呼び出し側が告知をスキップ）
	if _, ok := s.resolveAnnounce(ctx, config.AnnounceAction{TemplateID: "ghost"}); ok {
		t.Fatal("未知の templateId が解決できてしまった")
	}
}

// PUT /restart-config は有効な告知の templateId 実在を検証する（無効時は検証しない）。
func TestRestartConfigPut_TemplateIDValidation(t *testing.T) {
	s, _ := newTplServer(t, nil) // 取得失敗→ビルトインで検証
	put := func(body string) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/restart-config", strings.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleRestartConfigPut(rec, req)
		return rec.Code
	}
	base := `"scheduled":[],"waitControl":{"quietWaitMin":58,"announceWaitMin":2},"crashRecovery":{"enabled":true,"maxCrashes":3,"windowMinutes":10}`
	if code := put(`{` + base + `,"preActions":{"announce":{"enabled":true,"templateId":"torazo-close"}}}`); code != http.StatusOK {
		t.Fatalf("ビルトインに実在する templateId が保存できない: %d", code)
	}
	if code := put(`{` + base + `,"preActions":{"announce":{"enabled":true,"templateId":"ghost"}}}`); code != http.StatusBadRequest {
		t.Fatalf("未知の templateId が 400 にならない: %d", code)
	}
	// 無効化されていれば未知 id でも保存できる（テンプレ消滅時に無効化保存を妨げない）
	if code := put(`{` + base + `,"preActions":{"announce":{"enabled":false,"templateId":"ghost"}}}`); code != http.StatusOK {
		t.Fatalf("無効化した告知の未知 templateId で保存が弾かれた: %d", code)
	}
}

// orchestrator: テンプレ参照の告知は実行直前に解決された URL/タグで spawn/impulse する。
func TestAnnounce_TemplateResolvedAtRuntime(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{present: 2} // 在席→締切まで待機→告知→強制
	rc := config.DefaultRestart()
	rc.WaitControl = config.WaitControl{QuietWaitMin: 50, AnnounceWaitMin: 50}
	rc.PreActions.Announce = config.AnnounceAction{Enabled: true, TemplateID: "tpl-x", Message: "再起動します"}
	o := newTestOrch(d, fw, rc, "night")
	o.resolveAnnounce = func(_ context.Context, a config.AnnounceAction) (config.AnnounceAction, bool) {
		a.ItemURL, a.ImpulseTag = "resrec:///resolved", "TAG.resolved"
		return a, true
	}
	if err := o.Trigger("manual", "day"); err != nil {
		t.Fatalf("trigger 失敗: %v", err)
	}
	waitUntil(t, func() bool { _, _, starts, _ := d.snap(); return starts == 1 }, 5*time.Second, "再起動完了")
	cmds := fw.commands()
	if !hasCmd(cmds, "resrec:///resolved") {
		t.Fatalf("解決済み URL で spawn されていない: %v", cmds)
	}
	if !hasCmd(cmds, "TAG.resolved") {
		t.Fatalf("解決済みタグで impulse されていない: %v", cmds)
	}
}

// orchestrator: templateId が解決できないときは告知をスキップし、再起動自体は進める。
func TestAnnounce_UnresolvableTemplateSkipsAnnounce(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{present: 2}
	rc := config.DefaultRestart()
	rc.WaitControl = config.WaitControl{QuietWaitMin: 50, AnnounceWaitMin: 50}
	rc.PreActions.Announce = config.AnnounceAction{Enabled: true, TemplateID: "ghost"}
	o := newTestOrch(d, fw, rc, "night")
	o.resolveAnnounce = func(_ context.Context, a config.AnnounceAction) (config.AnnounceAction, bool) {
		return a, false
	}
	if err := o.Trigger("manual", "day"); err != nil {
		t.Fatalf("trigger 失敗: %v", err)
	}
	waitUntil(t, func() bool { _, _, starts, _ := d.snap(); return starts == 1 }, 5*time.Second, "再起動完了")
	cmds := fw.commands()
	if hasCmd(cmds, "spawn") || hasCmd(cmds, "dynamicimpulsestring") {
		t.Fatalf("未解決テンプレで告知が実行された: %v", cmds)
	}
}

// スポーン＆パルス: fakehl 統合（手動/テンプレ/impulse のみ/未知 id/タグ欠落）。
// 待機は 1ms に短縮（spawn→impulse の順序自体は実装の select で保証）。
func TestServer_Write_SpawnImpulse(t *testing.T) {
	ts, pw, srv := newTestServerFull(t)
	srv.spawnImpulseDelay = time.Millisecond
	srv.spawnTpl.url = deadURL() // テンプレ解決はビルトイン（tts-loop）で行う

	// 手動（itemUrl+impulseTag）
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw,
		`{"itemUrl":"resrec:///U-MarkN/R-abc","impulseTag":"T.manual","message":"hello"}`)
	if code != http.StatusOK || env.Data["executed"] != true {
		t.Fatalf("手動 spawn-impulse 失敗: code=%d env=%+v", code, env)
	}
	// テンプレ参照（ビルトイン tts-loop）
	code, env = postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw,
		`{"templateId":"tts-loop","message":"読み上げテスト"}`)
	if code != http.StatusOK || env.Data["executed"] != true {
		t.Fatalf("テンプレ spawn-impulse 失敗: code=%d env=%+v", code, env)
	}
	// itemUrl 空＝spawn 省略で impulse のみ（常設受け機構前提・告知③と同条件）
	code, env = postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw,
		`{"impulseTag":"T.only","message":""}`)
	if code != http.StatusOK || env.Data["executed"] != true {
		t.Fatalf("impulse のみの spawn-impulse 失敗: code=%d env=%+v", code, env)
	}
	// 未知 templateId → 400
	if code, _ := postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw, `{"templateId":"ghost"}`); code != http.StatusBadRequest {
		t.Fatalf("未知 templateId が 400 にならない: %d", code)
	}
	// 手動でタグ欠落 → 400
	if code, _ := postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw, `{"itemUrl":"resrec:///x"}`); code != http.StatusBadRequest {
		t.Fatalf("タグ欠落が 400 にならない: %d", code)
	}
}
