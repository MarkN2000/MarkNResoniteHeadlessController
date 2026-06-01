package server

// スケジュール（Phase 8・§3.16・P8-1）のHTTPテスト: restart-config GET/PUT・restart-status。
// driver 未起動でよい（これらは driver.Status() を読むだけ）。共通ヘルパ（authGet/authReq/okEnv）
// と driver 未起動 Server は settings_test.go の newSettingsServer を再利用。

import (
	"net/http"
	"testing"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

func TestRestartConfig_GetDefault(t *testing.T) {
	ts, pw, _ := newSettingsServer(t)
	var got okEnv[config.Restart]
	if code := authGet(t, ts.URL+"/api/v1/restart-config", pw, &got); code != http.StatusOK {
		t.Fatalf("GET status=%d", code)
	}
	rc := got.Data
	if rc.WaitControl.ForceRestartTimeoutMin != 60 || rc.WaitControl.ActionTimingMin != 2 {
		t.Fatalf("既定 waitControl が想定外: %+v", rc.WaitControl)
	}
	if !rc.CrashRecovery.Enabled || rc.CrashRecovery.MaxCrashes != 3 || rc.CrashRecovery.WindowMinutes != 10 {
		t.Fatalf("既定 crashRecovery が想定外: %+v", rc.CrashRecovery)
	}
	if !rc.PreActions.SessionChanges.SetMaxUsersOne || rc.PreActions.SessionChanges.SetPrivate {
		t.Fatalf("既定 sessionChanges は maxusers=1 のみ ON のはず: %+v", rc.PreActions.SessionChanges)
	}
	if rc.PreActions.Announce.Enabled {
		t.Fatalf("既定 announce は OFF のはず: %+v", rc.PreActions.Announce)
	}
	if len(rc.Scheduled) != 0 {
		t.Fatalf("既定 scheduled は空のはず: %+v", rc.Scheduled)
	}
}

func TestRestartConfig_PutAndPersist(t *testing.T) {
	ts, pw, cfgPath := newSettingsServer(t)
	body := `{
		"scheduled":[
			{"id":"a","enabled":true,"type":"daily","hour":5,"minute":0,"configName":""},
			{"id":"b","enabled":false,"type":"weekly","weekday":1,"hour":4,"minute":30,"configName":"night"}
		],
		"waitControl":{"forceRestartTimeoutMin":90,"actionTimingMin":3},
		"preActions":{
			"announce":{"enabled":true,"itemUrl":"resrec:///x","impulseTag":"MRHC.play","message":"再起動します"},
			"sessionChanges":{"setPrivate":true,"setMaxUsersOne":false,"renameEnabled":false,"renameTo":""}
		},
		"crashRecovery":{"enabled":false,"maxCrashes":5,"windowMinutes":15}
	}`
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/restart-config", pw, "application/json", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	// GET が反映を返す
	var got okEnv[config.Restart]
	authGet(t, ts.URL+"/api/v1/restart-config", pw, &got)
	if len(got.Data.Scheduled) != 2 || got.Data.WaitControl.ForceRestartTimeoutMin != 90 {
		t.Fatalf("PUT 後の GET が想定外: %+v", got.Data)
	}
	if got.Data.CrashRecovery.Enabled {
		t.Fatalf("crashRecovery を false にしたのに true（pointer で明示 false が保持されていない）: %+v", got.Data.CrashRecovery)
	}

	// ファイルにも永続化されている
	reloaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Restart == nil || len(reloaded.Restart.Scheduled) != 2 || reloaded.Restart.Scheduled[1].ConfigName != "night" {
		t.Fatalf("ファイル未反映: %+v", reloaded.Restart)
	}
}

func TestRestartConfig_PutInvalid(t *testing.T) {
	ts, pw, _ := newSettingsServer(t)
	base := `"waitControl":{"forceRestartTimeoutMin":60,"actionTimingMin":2},"crashRecovery":{"enabled":true,"maxCrashes":3,"windowMinutes":10}`
	cases := map[string]string{
		"bad type":             `{"scheduled":[{"id":"a","enabled":true,"type":"monthly","hour":1,"minute":0}],` + base + `}`,
		"hour out of range":    `{"scheduled":[{"id":"a","enabled":true,"type":"daily","hour":24,"minute":0}],` + base + `}`,
		"timeout out of range": `{"scheduled":[],"waitControl":{"forceRestartTimeoutMin":0,"actionTimingMin":0},"crashRecovery":{"enabled":true,"maxCrashes":3,"windowMinutes":10}}`,
		"announce no tag":      `{"scheduled":[],"waitControl":{"forceRestartTimeoutMin":60,"actionTimingMin":2},"preActions":{"announce":{"enabled":true,"impulseTag":"","message":"x"}},"crashRecovery":{"enabled":true,"maxCrashes":3,"windowMinutes":10}}`,
		"duplicate id":         `{"scheduled":[{"id":"a","enabled":true,"type":"daily","hour":1,"minute":0},{"id":"a","enabled":true,"type":"daily","hour":2,"minute":0}],` + base + `}`,
		"bad config name":      `{"scheduled":[{"id":"a","enabled":true,"type":"daily","hour":1,"minute":0,"configName":"../escape"}],` + base + `}`,
		"invalid calendar":     `{"scheduled":[{"id":"a","enabled":true,"type":"once","year":2026,"month":2,"day":30,"hour":1,"minute":0}],` + base + `}`,
	}
	for name, b := range cases {
		resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/restart-config", pw, "application/json", b)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: 400 にならない: %d", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// runtime-state は複数フィールドを read-modify-write で保全する（recordLastUsed が lastRestart を消さない・P8-5）。
func TestRuntimeState_PreservesFields(t *testing.T) {
	s := &Server{dataDir: t.TempDir()}
	s.recordLastUsed("night")
	s.recordLastRestart("scheduled", "2026-06-01T05:00:00+09:00")

	if got := s.loadLastUsed(); got != "night" {
		t.Fatalf("lastUsed が想定外: %q", got)
	}
	at, trig := s.loadLastRestart()
	if at != "2026-06-01T05:00:00+09:00" || trig != "scheduled" {
		t.Fatalf("lastRestart が想定外: at=%q trig=%q", at, trig)
	}
	// recordLastUsed は lastRestart を消さない。
	s.recordLastUsed("day")
	if at2, trig2 := s.loadLastRestart(); at2 != at || trig2 != trig {
		t.Fatalf("recordLastUsed が lastRestart を消した: at=%q trig=%q", at2, trig2)
	}
	if s.loadLastUsed() != "day" {
		t.Fatal("lastUsed が更新されていない")
	}
}

func TestRestartStatus_Idle(t *testing.T) {
	ts, pw, _ := newSettingsServer(t)
	var got okEnv[restartStatus]
	if code := authGet(t, ts.URL+"/api/v1/restart-status", pw, &got); code != http.StatusOK {
		t.Fatalf("GET status=%d", code)
	}
	if got.Data.Running || got.Data.InProgress || got.Data.Phase != "idle" {
		t.Fatalf("driver 未起動の idle 状態が想定外: %+v", got.Data)
	}
	if !got.Data.CrashRecoveryEnabled {
		t.Fatalf("既定では crashRecoveryEnabled=true のはず: %+v", got.Data)
	}
	if got.Data.NextScheduledAt != nil {
		t.Fatalf("既定（予定なし）では nextScheduledAt=null のはず: %v", *got.Data.NextScheduledAt)
	}
}

// PUT で有効な daily 予定を入れると status に次回予定（未来・config 名つき）が出る（P8-2）。
func TestRestartStatus_NextScheduled(t *testing.T) {
	ts, pw, _ := newSettingsServer(t)
	body := `{
		"scheduled":[{"id":"a","enabled":true,"type":"daily","hour":5,"minute":0,"configName":"night"}],
		"waitControl":{"forceRestartTimeoutMin":60,"actionTimingMin":2},
		"crashRecovery":{"enabled":true,"maxCrashes":3,"windowMinutes":10}
	}`
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/restart-config", pw, "application/json", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	var got okEnv[restartStatus]
	authGet(t, ts.URL+"/api/v1/restart-status", pw, &got)
	d := got.Data
	if d.NextScheduledAt == nil {
		t.Fatal("daily 予定があるのに nextScheduledAt=null")
	}
	if d.NextScheduledID != "a" || d.NextScheduledType != "daily" || d.NextScheduledConfigName != "night" {
		t.Fatalf("次回予定のメタが想定外: %+v", d)
	}
	when, err := time.Parse(time.RFC3339, *d.NextScheduledAt)
	if err != nil {
		t.Fatalf("nextScheduledAt が RFC3339 でない: %q (%v)", *d.NextScheduledAt, err)
	}
	if !when.After(time.Now()) {
		t.Fatalf("次回予定が未来でない: %v", when)
	}
}
