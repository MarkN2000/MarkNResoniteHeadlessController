package config

import "testing"

func TestDefaultRestart(t *testing.T) {
	d := DefaultRestart()
	if d.WaitControl.QuietWaitMin != 58 || d.WaitControl.AnnounceWaitMin != 2 {
		t.Fatalf("waitControl 既定が想定外: %+v", d.WaitControl)
	}
	if !d.CrashRecovery.Enabled || d.CrashRecovery.MaxCrashes != 3 || d.CrashRecovery.WindowMinutes != 10 {
		t.Fatalf("crashRecovery 既定が想定外: %+v", d.CrashRecovery)
	}
	if !d.PreActions.SessionChanges.SetMaxUsersOne || d.PreActions.SessionChanges.SetPrivate {
		t.Fatalf("sessionChanges 既定は maxusers=1 のみ ON のはず: %+v", d.PreActions.SessionChanges)
	}
	if d.Scheduled == nil {
		t.Fatal("scheduled は非nilの空スライスであるべき（JSONで [] になる）")
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("既定値が検証を通らない: %v", err)
	}
}

func TestRestartOrDefault(t *testing.T) {
	c := &Config{} // Restart 未設定
	if c.RestartOrDefault().WaitControl.QuietWaitMin != 58 {
		t.Fatal("未設定時に既定が返らない")
	}
	custom := DefaultRestart()
	custom.CrashRecovery.Enabled = false // 明示 false
	c.Restart = &custom
	if c.RestartOrDefault().CrashRecovery.Enabled {
		t.Fatal("設定済みの明示 false が返らない（pointer の意味がない）")
	}
}

func TestRestartValidate(t *testing.T) {
	valid := DefaultRestart()
	valid.Scheduled = []ScheduledRestart{
		{ID: "a", Enabled: true, Type: RestartTypeDaily, Hour: 5, Minute: 0},
		{ID: "b", Enabled: true, Type: RestartTypeWeekly, Weekday: 3, Hour: 4, Minute: 30},
		{ID: "c", Enabled: false, Type: RestartTypeOnce, Year: 2026, Month: 6, Day: 10, Hour: 3, Minute: 0},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("妥当な設定が弾かれた: %v", err)
	}

	// 告知ON・タグあり・メッセージ空は妥当（メッセージは任意＝受信アイテム依存）。
	msgOptional := DefaultRestart()
	msgOptional.PreActions.Announce = AnnounceAction{Enabled: true, ImpulseTag: "MRHC.play", Message: ""}
	if err := msgOptional.Validate(); err != nil {
		t.Fatalf("メッセージ空の告知が弾かれた（任意のはず）: %v", err)
	}

	// テンプレ参照（templateId 非空）は URL/タグを実行時解決するため、タグ空でも妥当。
	tplRef := DefaultRestart()
	tplRef.PreActions.Announce = AnnounceAction{Enabled: true, TemplateID: "torazo-close"}
	if err := tplRef.Validate(); err != nil {
		t.Fatalf("テンプレ参照の告知が弾かれた（タグは実行時解決のはず）: %v", err)
	}

	bad := func(mut func(r *Restart)) {
		t.Helper()
		r := DefaultRestart()
		mut(&r)
		if err := r.Validate(); err == nil {
			t.Fatal("不正な設定が検証を通ってしまった")
		}
	}
	bad(func(r *Restart) { r.WaitControl.QuietWaitMin = 1441 })    // > 1440（範囲外）
	bad(func(r *Restart) { r.WaitControl.AnnounceWaitMin = -1 })   // < 0（範囲外）
	bad(func(r *Restart) { r.WaitControl.AnnounceWaitMin = 1441 }) // > 1440（範囲外）
	bad(func(r *Restart) { r.CrashRecovery.MaxCrashes = 0 })
	bad(func(r *Restart) { r.PreActions.Announce = AnnounceAction{Enabled: true, Message: "x"} }) // tag 空
	bad(func(r *Restart) { r.PreActions.SessionChanges = SessionChanges{RenameEnabled: true} })   // 名前空
	bad(func(r *Restart) {
		r.Scheduled = []ScheduledRestart{{ID: "a", Type: "monthly", Hour: 1}}
	})
	bad(func(r *Restart) {
		r.Scheduled = []ScheduledRestart{{ID: "x", Type: RestartTypeDaily, Hour: 1}, {ID: "x", Type: RestartTypeDaily, Hour: 2}}
	})
	bad(func(r *Restart) { // 2月30日（実在しない暦日）は弾く
		r.Scheduled = []ScheduledRestart{{ID: "a", Type: RestartTypeOnce, Year: 2026, Month: 2, Day: 30, Hour: 1, Minute: 0}}
	})
}
