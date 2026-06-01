package config

import (
	"testing"
	"time"
)

// at は time.Local の指定時分（秒=0）を作る。テストの now/expected を同一TZで構築するため。
func at(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.Local)
}

// 基準時刻: 2026-06-01 10:00（月曜・weekday=1）。weekly の境界を見るため曜日が確定した日を選ぶ。
func TestNextFireAfter(t *testing.T) {
	now := at(2026, time.June, 1, 10, 0)
	if now.Weekday() != time.Monday {
		t.Fatalf("前提崩れ: 2026-06-01 は月曜のはず: %v", now.Weekday())
	}
	cases := []struct {
		name string
		s    ScheduledRestart
		want time.Time
		ok   bool
	}{
		{"daily 既に過ぎた→翌日", ScheduledRestart{Type: RestartTypeDaily, Hour: 5, Minute: 0}, at(2026, time.June, 2, 5, 0), true},
		{"daily まだ未来→今日", ScheduledRestart{Type: RestartTypeDaily, Hour: 15, Minute: 0}, at(2026, time.June, 1, 15, 0), true},
		{"daily 同分ちょうど→翌日", ScheduledRestart{Type: RestartTypeDaily, Hour: 10, Minute: 0}, at(2026, time.June, 2, 10, 0), true},
		{"weekly 当日まだ未来→今日", ScheduledRestart{Type: RestartTypeWeekly, Weekday: 1, Hour: 15, Minute: 0}, at(2026, time.June, 1, 15, 0), true},
		{"weekly 当日過ぎた→翌週", ScheduledRestart{Type: RestartTypeWeekly, Weekday: 1, Hour: 5, Minute: 0}, at(2026, time.June, 8, 5, 0), true},
		{"weekly 別曜日(水)", ScheduledRestart{Type: RestartTypeWeekly, Weekday: 3, Hour: 9, Minute: 0}, at(2026, time.June, 3, 9, 0), true},
		{"weekly 跨ぎ(日)", ScheduledRestart{Type: RestartTypeWeekly, Weekday: 0, Hour: 12, Minute: 0}, at(2026, time.June, 7, 12, 0), true},
		{"once 未来", ScheduledRestart{Type: RestartTypeOnce, Year: 2026, Month: 6, Day: 10, Hour: 3, Minute: 0}, at(2026, time.June, 10, 3, 0), true},
		{"once 当日まだ未来", ScheduledRestart{Type: RestartTypeOnce, Year: 2026, Month: 6, Day: 1, Hour: 23, Minute: 0}, at(2026, time.June, 1, 23, 0), true},
		{"once 過去→発火しない", ScheduledRestart{Type: RestartTypeOnce, Year: 2026, Month: 5, Day: 1, Hour: 3, Minute: 0}, time.Time{}, false},
		{"once 当日過ぎた→発火しない", ScheduledRestart{Type: RestartTypeOnce, Year: 2026, Month: 6, Day: 1, Hour: 5, Minute: 0}, time.Time{}, false},
	}
	for _, c := range cases {
		got, ok := c.s.NextFireAfter(now)
		if ok != c.ok {
			t.Errorf("%s: ok=%v want %v", c.name, ok, c.ok)
			continue
		}
		if ok && !got.Equal(c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestNextScheduled(t *testing.T) {
	now := at(2026, time.June, 1, 10, 0)

	// 有効な最小を選ぶ（無効は除外）: 無効 11:00 が最も近いが、有効は 15:00 と翌週なので 15:00。
	r := Restart{Scheduled: []ScheduledRestart{
		{ID: "disabled-soon", Enabled: false, Type: RestartTypeDaily, Hour: 11, Minute: 0},
		{ID: "today15", Enabled: true, Type: RestartTypeDaily, Hour: 15, Minute: 0},
		{ID: "nextweek", Enabled: true, Type: RestartTypeWeekly, Weekday: 1, Hour: 5, Minute: 0},
	}}
	next, sc, ok := r.NextScheduled(now)
	if !ok {
		t.Fatal("有効予定があるのに ok=false")
	}
	if sc.ID != "today15" || !next.Equal(at(2026, time.June, 1, 15, 0)) {
		t.Fatalf("最小選択が想定外: id=%s next=%v", sc.ID, next)
	}

	// 全て無効/過去 once → ok=false。
	r2 := Restart{Scheduled: []ScheduledRestart{
		{ID: "off", Enabled: false, Type: RestartTypeDaily, Hour: 1, Minute: 0},
		{ID: "past", Enabled: true, Type: RestartTypeOnce, Year: 2026, Month: 5, Day: 1, Hour: 3, Minute: 0},
	}}
	if _, _, ok := r2.NextScheduled(now); ok {
		t.Fatal("発火対象が無いのに ok=true")
	}

	// 空 → ok=false。
	if _, _, ok := (Restart{Scheduled: []ScheduledRestart{}}).NextScheduled(now); ok {
		t.Fatal("空予定で ok=true")
	}
}
