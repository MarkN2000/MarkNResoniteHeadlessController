package config

import "time"

// スケジュール発火時刻の算出（Phase 8・§3.16(3)(8)・P8-2）。
// cron 不使用の独自モデル once/weekly/daily。判定はサーバーローカル時刻（now.Location()）で行い、
// 秒・ナノ秒は 0（予定は分解像度）。副作用なしの純関数＝決定的にテストできる。
// 実際に時刻到達で再起動を起動するゴルーチンは P8-3/P8-4（orchestrator）で追加する。

// NextFireAfter は予定 s が now より「厳密に未来」に発火する最小時刻を返す。
//   - once   : 指定年月日時分が now より未来ならそれ。過去なら ok=false（発火しない）。
//   - daily  : 今日のその時刻が未来ならそれ、過ぎていれば翌日。
//   - weekly : その曜日のその時刻。今日が該当曜日でも時刻を過ぎていれば翌週。
//
// 「厳密に未来」とするのは、同分ちょうどでの即時再発火を避けるため。
func (s ScheduledRestart) NextFireAfter(now time.Time) (time.Time, bool) {
	loc := now.Location()
	switch s.Type {
	case RestartTypeOnce:
		t := time.Date(s.Year, time.Month(s.Month), s.Day, s.Hour, s.Minute, 0, 0, loc)
		if t.After(now) {
			return t, true
		}
		return time.Time{}, false
	case RestartTypeDaily:
		t := time.Date(now.Year(), now.Month(), now.Day(), s.Hour, s.Minute, 0, 0, loc)
		if !t.After(now) {
			t = t.AddDate(0, 0, 1) // 今日は過ぎた→翌日
		}
		return t, true
	case RestartTypeWeekly:
		t := time.Date(now.Year(), now.Month(), now.Day(), s.Hour, s.Minute, 0, 0, loc)
		delta := (s.Weekday - int(t.Weekday()) + 7) % 7 // 今日から目的曜日までの日数（0..6）
		if delta == 0 && !t.After(now) {
			delta = 7 // 当日該当曜日だが時刻を過ぎた→翌週
		}
		return t.AddDate(0, 0, delta), true
	}
	return time.Time{}, false
}

// NextScheduled は有効な全予定のうち now より未来で最も近い発火を返す。
// 無効な予定・発火しない過去 once は除外。該当が無ければ ok=false。
func (r Restart) NextScheduled(now time.Time) (next time.Time, sched ScheduledRestart, ok bool) {
	for _, s := range r.Scheduled {
		if !s.Enabled {
			continue
		}
		t, fires := s.NextFireAfter(now)
		if !fires {
			continue
		}
		if !ok || t.Before(next) {
			next, sched, ok = t, s, true
		}
	}
	return next, sched, ok
}
