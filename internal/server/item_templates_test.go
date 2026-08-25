package server

import (
	"context"
	"encoding/json"
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

const tplJSON = `{"version":1,"templates":[
	{"id":"torazo-close","label":{"ja":"とらぞ"},"url":"resrec:///U-MarkN/R-updated","tag":"MRHC.play","actions":["announce"]},
	{"id":"deco","url":"resrec:///U-MarkN/R-deco","actions":["spawn"]},
	{"id":"voice-loop","url":"resrec:///U-MarkN/R-voice","tag":"MRHC.play","actions":["spawnImpulse","announce"],"input":{"kind":"ttsVoice"}}
]}`

func deadURL() string {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	return dead.URL
}

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
		s.itemTpl.url = deadURL()
	} else {
		ts := httptest.NewServer(remote)
		t.Cleanup(ts.Close)
		s.itemTpl.url = ts.URL
	}
	return s, dir
}

func TestItemTemplates_RemoteCacheAndFallback(t *testing.T) {
	calls := 0
	s, dir := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(tplJSON))
	})
	list, source := s.itemTpl.templates(context.Background())
	if source != "remote" || len(list) != 3 {
		t.Fatalf("remote 取得が想定外: source=%s list=%+v", source, list)
	}
	if _, err := os.Stat(filepath.Join(dir, "item-templates.json")); err != nil {
		t.Fatalf("永続キャッシュ未作成: %v", err)
	}
	if _, _ = s.itemTpl.templates(context.Background()); calls != 1 {
		t.Fatalf("TTL 内で再取得された: calls=%d", calls)
	}

	offline, offlineDir := newTplServer(t, nil)
	writeJSONFile(filepath.Join(offlineDir, "item-templates.json"), itemTemplateList{
		Version: 1,
		Templates: []itemTemplate{{
			ID: "saved", URL: "resrec:///saved", Actions: []templateAction{templateActionSpawn},
		}},
	})
	if got, gotSource := offline.itemTpl.templates(context.Background()); gotSource != "cache" || len(got) != 1 || got[0].ID != "saved" {
		t.Fatalf("永続キャッシュへフォールバックしない: source=%s list=%+v", gotSource, got)
	}

	builtin, _ := newTplServer(t, nil)
	if got, gotSource := builtin.itemTpl.templates(context.Background()); gotSource != "builtin" || len(got) == 0 {
		t.Fatalf("ビルトインへフォールバックしない: source=%s list=%+v", gotSource, got)
	}
	if _, ok := builtin.itemTpl.lookup(context.Background(), config.DefaultRestart().PreActions.Announce.TemplateID, templateActionAnnounce); !ok {
		t.Fatal("既定 announce template がビルトインで解決できない")
	}
}

func TestItemTemplates_ValidatesActionsAndLookup(t *testing.T) {
	s, _ := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":1,"templates":[
			{"id":"spawn","url":"resrec:///spawn","actions":["spawn"]},
			{"id":"impulse","url":"resrec:///impulse","tag":"T","actions":["spawnImpulse"]},
			{"id":"announce","url":"resrec:///announce","tag":"T","actions":["announce"]},
			{"id":"bad-url","url":"","actions":["spawn"]},
			{"id":"bad-action","url":"resrec:///x","actions":["other"]},
			{"id":"duplicate-action","url":"resrec:///x","actions":["spawn","spawn"]},
			{"id":"tag-required","url":"resrec:///x","actions":["announce"]},
			{"id":"bad-input","url":"resrec:///x","tag":"T","actions":["spawn"],"input":{"kind":"ttsVoice"}},
			{"id":"spawn","url":"resrec:///duplicate","actions":["spawn"]}
		]}`))
	})
	list, source := s.itemTpl.templates(context.Background())
	if source != "remote" || len(list) != 3 {
		t.Fatalf("action 検証が想定外: source=%s list=%+v", source, list)
	}
	if _, ok := s.itemTpl.lookup(context.Background(), "spawn", templateActionSpawn); !ok {
		t.Fatal("spawn action の lookup に失敗")
	}
	if _, ok := s.itemTpl.lookup(context.Background(), "spawn", templateActionAnnounce); ok {
		t.Fatal("許可されていない action で lookup できた")
	}
}

func TestItemTemplates_HandlerReturnsAllActions(t *testing.T) {
	s, _ := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tplJSON)) })
	rec := httptest.NewRecorder()
	s.handleItemTemplates(rec, httptest.NewRequest(http.MethodGet, "/api/v1/item-templates", nil))
	var env okEnv[struct {
		Templates []itemTemplate `json:"templates"`
	}]
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&env) != nil || !env.OK || len(env.Data.Templates) != 3 {
		t.Fatalf("全actionのテンプレートを返さない: status=%d env=%+v", rec.Code, env)
	}
}

func TestResolveAnnounce_TTSAndActionConstraint(t *testing.T) {
	s, _ := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tplJSON)) })
	if _, ok := s.resolveAnnounce(context.Background(), config.AnnounceAction{TemplateID: "deco"}); ok {
		t.Fatal("announce 非許可テンプレートが告知に解決された")
	}
	got, ok := s.resolveAnnounce(context.Background(), config.AnnounceAction{
		TemplateID: "voice-loop", Message: "声を出します", SpeakerID: 42,
	})
	want := "https://tts.markn2000.com/api/v1/tts?text=%E5%A3%B0%E3%82%92%E5%87%BA%E3%81%97%E3%81%BE%E3%81%99&speaker=42"
	if !ok || got.ItemURL != "resrec:///U-MarkN/R-voice" || got.Message != want {
		t.Fatalf("TTS告知テンプレートが想定外: ok=%v got=%+v", ok, got)
	}
}

func TestImpulseValueForTemplate(t *testing.T) {
	voice := &itemTemplate{Input: &templateInput{Kind: templateInputTTSVoice}}
	got, err := impulseValueForTemplate(voice, "a&b\n次", 12)
	if err != nil || got != "https://tts.markn2000.com/api/v1/tts?text=a%26b%0A%E6%AC%A1&speaker=12" {
		t.Fatalf("TTS URL が想定外: got=%q err=%v", got, err)
	}
	for _, tc := range []struct {
		tpl       *itemTemplate
		message   string
		speakerID int64
	}{
		{voice, "  ", 1},
		{voice, "text", 0},
		{&itemTemplate{}, "text", 1},
		{nil, "text", 1},
	} {
		if _, err := impulseValueForTemplate(tc.tpl, tc.message, tc.speakerID); err == nil {
			t.Fatal("不正なspeakerId/TTS値が通った")
		}
	}
}

func TestTTSSpeakers(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"Alice","styles":[{"name":"Normal","id":10},{"name":"Invalid","id":0}]}]`))
	}))
	defer upstream.Close()
	s, _ := newTplServer(t, nil)
	s.ttsSpeakersURL = upstream.URL
	s.ttsHTTPClient = upstream.Client()
	rec := httptest.NewRecorder()
	s.handleTTSSpeakers(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tts-speakers", nil))
	var env okEnv[struct {
		Voices []ttsVoice `json:"voices"`
	}]
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&env) != nil || !env.OK || len(env.Data.Voices) != 1 {
		t.Fatalf("話者一覧が想定外: status=%d env=%+v", rec.Code, env)
	}
}

func TestRestartConfigPut_AnnounceActionConstraint(t *testing.T) {
	s, _ := newTplServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tplJSON)) })
	put := func(announce string) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/restart-config", strings.NewReader(`{"scheduled":[],"waitControl":{"quietWaitMin":58,"announceWaitMin":2},"preActions":{"announce":`+announce+`},"crashRecovery":{"enabled":true,"maxCrashes":3,"windowMinutes":10}}`))
		rec := httptest.NewRecorder()
		s.handleRestartConfigPut(rec, req)
		return rec.Code
	}
	if code := put(`{"enabled":true,"templateId":"voice-loop","message":"text","speakerId":9}`); code != http.StatusOK {
		t.Fatalf("正しいTTS告知が保存できない: %d", code)
	}
	if code := put(`{"enabled":true,"templateId":"deco","message":"text"}`); code != http.StatusBadRequest {
		t.Fatalf("announce 非許可テンプレートが400にならない: %d", code)
	}
	if code := put(`{"enabled":true,"templateId":"voice-loop","message":"","speakerId":9}`); code != http.StatusBadRequest {
		t.Fatalf("TTSテキスト空が400にならない: %d", code)
	}
	if code := put(`{"enabled":true,"templateId":"voice-loop","message":"text","speakerId":0}`); code != http.StatusBadRequest {
		t.Fatalf("TTS speakerId欠落が400にならない: %d", code)
	}
	if code := put(`{"enabled":true,"itemUrl":"resrec:///manual","impulseTag":"T","message":"text","speakerId":9}`); code != http.StatusBadRequest {
		t.Fatalf("手動入力speakerIdが400にならない: %d", code)
	}
}

func TestAnnounce_TemplateResolvedAtRuntime(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{present: 2}
	rc := config.DefaultRestart()
	rc.WaitControl = config.WaitControl{QuietWaitMin: 50, AnnounceWaitMin: 50}
	rc.PreActions.Announce = config.AnnounceAction{Enabled: true, TemplateID: "tpl-x", Message: "再起動します"}
	o := newTestOrch(d, fw, rc, "night")
	o.resolveAnnounce = func(_ context.Context, a config.AnnounceAction) (config.AnnounceAction, bool) {
		a.ItemURL, a.ImpulseTag = "resrec:///resolved", "TAG.resolved"
		return a, true
	}
	if err := o.Trigger("scheduled", "day"); err != nil {
		t.Fatalf("trigger 失敗: %v", err)
	}
	waitUntil(t, func() bool { _, _, starts, _ := d.snap(); return starts == 1 }, 5*time.Second, "再起動完了")
	cmds := fw.commands()
	if !hasCmd(cmds, "resrec:///resolved") || !hasCmd(cmds, "TAG.resolved") {
		t.Fatalf("解決済みテンプレートで告知されていない: %v", cmds)
	}
}

func TestAnnounce_UnresolvableTemplateSkipsAnnounce(t *testing.T) {
	d := &fakeDriver{state: headless.StateRunning}
	fw := &fakeWorlds{present: 2}
	rc := config.DefaultRestart()
	rc.WaitControl = config.WaitControl{QuietWaitMin: 50, AnnounceWaitMin: 50}
	rc.PreActions.Announce = config.AnnounceAction{Enabled: true, TemplateID: "ghost"}
	o := newTestOrch(d, fw, rc, "night")
	o.resolveAnnounce = func(_ context.Context, a config.AnnounceAction) (config.AnnounceAction, bool) { return a, false }
	if err := o.Trigger("scheduled", "day"); err != nil {
		t.Fatalf("trigger 失敗: %v", err)
	}
	waitUntil(t, func() bool { _, _, starts, _ := d.snap(); return starts == 1 }, 5*time.Second, "再起動完了")
	cmds := fw.commands()
	if hasCmd(cmds, "spawn") || hasCmd(cmds, "dynamicimpulsestring") {
		t.Fatalf("未解決テンプレートで告知された: %v", cmds)
	}
}

func TestServer_Write_SpawnImpulseActionConstraint(t *testing.T) {
	ts, pw, srv := newTestServerFull(t)
	srv.spawnImpulseDelay = time.Millisecond
	srv.itemTpl.cache = []itemTemplate{
		{ID: "deco", URL: "resrec:///U-MarkN/R-deco", Actions: []templateAction{templateActionSpawn}},
		{ID: "voice-loop", URL: "resrec:///U-MarkN/R-voice", Tag: "MRHC.play", Actions: []templateAction{templateActionSpawnImpulse}, Input: &templateInput{Kind: templateInputTTSVoice}},
	}
	srv.itemTpl.fetched = time.Now()
	code, env := postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw, `{"itemUrl":"resrec:///U-MarkN/R-manual","impulseTag":"T.manual","message":"hello"}`)
	if code != http.StatusOK || env.Data["executed"] != true {
		t.Fatalf("手動 spawn-impulse 失敗: code=%d env=%+v", code, env)
	}
	if code, _ := postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw, `{"itemUrl":"resrec:///x"}`); code != http.StatusBadRequest {
		t.Fatalf("手動入力のタグ欠落が400にならない: %d", code)
	}
	if code, _ := postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw, `{"templateId":"ghost"}`); code != http.StatusBadRequest {
		t.Fatalf("未知テンプレートが400にならない: %d", code)
	}
	code, env = postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw, `{"templateId":"voice-loop","message":"TTS test","speakerId":7}`)
	if code != http.StatusOK || env.Data["executed"] != true {
		t.Fatalf("ttsVoice spawn-impulse 失敗: code=%d env=%+v", code, env)
	}
	if code, _ := postJSON(t, ts.URL+"/api/v1/sessions/0/spawn-impulse", pw, `{"templateId":"deco","message":"x"}`); code != http.StatusBadRequest {
		t.Fatalf("spawn専用テンプレートがspawnImpulseで400にならない: %d", code)
	}
}
