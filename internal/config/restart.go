package config

import (
	"fmt"
	"strings"
	"time"
)

// Restart は自動再起動（スケジュール）設定。確定仕様: docs/design/phase-7-spec.md §3.16。
// Config.Restart は *Restart（省略可）。未設定（nil）のときは DefaultRestart を使う＝
// 既定でクラッシュ自動復帰 ON 等を効かせるため（ゼロ値 false との区別を pointer で行う）。
type Restart struct {
	Scheduled     []ScheduledRestart `json:"scheduled"`     // 再起動予定（空=なし）
	WaitControl   WaitControl        `json:"waitControl"`   // 全トリガー共通の待機制御
	PreActions    PreActions         `json:"preActions"`    // 再起動前アクション
	CrashRecovery CrashRecovery      `json:"crashRecovery"` // クラッシュ自動復帰
}

// ScheduledRestart は1件の再起動予定。Type により使うフィールドが変わる（独自形式・cron不使用）。
// 時刻はサーバーのローカル時刻で解釈する。
type ScheduledRestart struct {
	ID      string `json:"id"`      // フロント生成の安定ID（編集/削除の単位）
	Enabled bool   `json:"enabled"` // この予定の有効/無効
	Type    string `json:"type"`    // "once" | "weekly" | "daily"
	// year/month/day は once 専用で必ず正値（Validate 済）。daily/weekly では未使用＝0 のため
	// omitempty で保存JSONから省く。weekday(0=日)・hour・minute は 0 が有効値なので omitempty 不可。
	Year       int    `json:"year,omitempty"`  // once のみ
	Month      int    `json:"month,omitempty"` // once のみ（1-12）
	Day        int    `json:"day,omitempty"`   // once のみ（1-31）
	Weekday    int    `json:"weekday"`         // weekly のみ（0=日..6=土）
	Hour       int    `json:"hour"`            // 全 type（0-23）
	Minute     int    `json:"minute"`          // 全 type（0-59）
	ConfigName string `json:"configName"`      // 空=前回起動と同じ config / 非空=その config 名
}

// WaitControl は「予告→空くまで待つ→締切で強制」のグローバル設定。
type WaitControl struct {
	ForceRestartTimeoutMin int `json:"forceRestartTimeoutMin"` // 自然退出を待つ最大（分）。超過で強制
	ActionTimingMin        int `json:"actionTimingMin"`        // 締切の何分前に告知するか
}

// PreActions は再起動前に実行するアクション群。
type PreActions struct {
	Announce       AnnounceAction `json:"announce"`       // dynamicImpulse 告知
	SessionChanges SessionChanges `json:"sessionChanges"` // セッション設定変更
}

// AnnounceAction は dynamicImpulse 告知（spawn したアイテムに impulse を送る）。
type AnnounceAction struct {
	Enabled    bool   `json:"enabled"`
	ItemURL    string `json:"itemUrl"`    // spawn するアイテム（空=spawn しない＝常設受け機構前提）
	ImpulseTag string `json:"impulseTag"` // dynamicimpulsestring のタグ（例 MRHC.play）
	Message    string `json:"message"`    // 送る文字列（固定文）
}

// SessionChanges は再起動前のセッション設定変更（各項目は独立トグル・全OFF可）。
type SessionChanges struct {
	SetPrivate     bool   `json:"setPrivate"`     // accesslevel Private
	SetMaxUsersOne bool   `json:"setMaxUsersOne"` // maxusers 1
	RenameEnabled  bool   `json:"renameEnabled"`  // name 変更を行うか
	RenameTo       string `json:"renameTo"`       // 変更後のセッション名
}

// CrashRecovery は意図しないプロセス終了時の自動復帰。
type CrashRecovery struct {
	Enabled       bool `json:"enabled"`       // 既定 ON
	MaxCrashes    int  `json:"maxCrashes"`    // ループ保護: ウィンドウ内の許容クラッシュ回数
	WindowMinutes int  `json:"windowMinutes"` // ループ保護: 集計ウィンドウ（分）
}

// DefaultRestart は restart 未設定時の既定値（§3.16）。
// 告知は既定 OFF（itemUrl/tag はワールド依存で空のため）、セッション変更は maxusers=1 のみ ON、
// クラッシュ復帰は ON（10分に3回で停止）、待機は最大60分・告知2分前。
func DefaultRestart() Restart {
	return Restart{
		Scheduled:   []ScheduledRestart{},
		WaitControl: WaitControl{ForceRestartTimeoutMin: 60, ActionTimingMin: 2},
		PreActions: PreActions{
			Announce:       AnnounceAction{Enabled: false, Message: "まもなく再起動します"},
			SessionChanges: SessionChanges{SetMaxUsersOne: true},
		},
		CrashRecovery: CrashRecovery{Enabled: true, MaxCrashes: 3, WindowMinutes: 10},
	}
}

// RestartOrDefault は設定済みの restart を返す。未設定（nil）なら既定を返す。
func (c *Config) RestartOrDefault() Restart {
	if c.Restart != nil {
		return *c.Restart
	}
	return DefaultRestart()
}

// 予定の種別。
const (
	RestartTypeOnce   = "once"
	RestartTypeWeekly = "weekly"
	RestartTypeDaily  = "daily"
)

// Validate は restart 設定の数値範囲・enum・条件付き必須を検証する（PUT 時）。
// 文字列は存在のみ見る軽い検証。configName の実在/フォーマットは呼び出し側（server 層）で扱う。
func (r Restart) Validate() error {
	wc := r.WaitControl
	if wc.ForceRestartTimeoutMin < 1 || wc.ForceRestartTimeoutMin > 1440 {
		return fmt.Errorf("最大待機時間は 1〜1440 分で指定してください")
	}
	if wc.ActionTimingMin < 0 || wc.ActionTimingMin > wc.ForceRestartTimeoutMin {
		return fmt.Errorf("告知タイミングは 0〜最大待機時間（分）で指定してください")
	}
	cr := r.CrashRecovery
	if cr.MaxCrashes < 1 || cr.MaxCrashes > 100 {
		return fmt.Errorf("クラッシュ復帰の許容回数は 1〜100 で指定してください")
	}
	if cr.WindowMinutes < 1 || cr.WindowMinutes > 1440 {
		return fmt.Errorf("クラッシュ復帰の集計ウィンドウは 1〜1440 分で指定してください")
	}
	an := r.PreActions.Announce
	if an.Enabled {
		if strings.TrimSpace(an.ImpulseTag) == "" {
			return fmt.Errorf("告知を有効にする場合はインパルスタグを入力してください")
		}
		if strings.TrimSpace(an.Message) == "" {
			return fmt.Errorf("告知を有効にする場合はメッセージを入力してください")
		}
	}
	sc := r.PreActions.SessionChanges
	if sc.RenameEnabled && strings.TrimSpace(sc.RenameTo) == "" {
		return fmt.Errorf("セッション名変更を有効にする場合は名前を入力してください")
	}
	seen := map[string]bool{}
	for i, s := range r.Scheduled {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("予定[%d]: ID が空です", i)
		}
		if seen[s.ID] {
			return fmt.Errorf("予定の ID が重複しています: %s", s.ID)
		}
		seen[s.ID] = true
		switch s.Type {
		case RestartTypeOnce, RestartTypeWeekly, RestartTypeDaily:
		default:
			return fmt.Errorf("予定[%d]: 種別は once/weekly/daily のいずれかです", i)
		}
		if s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 {
			return fmt.Errorf("予定[%d]: 時刻が不正です（時 0-23 / 分 0-59）", i)
		}
		if s.Type == RestartTypeWeekly && (s.Weekday < 0 || s.Weekday > 6) {
			return fmt.Errorf("予定[%d]: 曜日が不正です（0=日..6=土）", i)
		}
		if s.Type == RestartTypeOnce {
			if s.Year < 2000 || s.Year > 9999 || s.Month < 1 || s.Month > 12 || s.Day < 1 || s.Day > 31 {
				return fmt.Errorf("予定[%d]: 日付が不正です", i)
			}
			// 実在チェック: time.Date は不正日付を正規化する（2/30→3/2）ため、
			// 往復で年月日が変わらないことを確認して 2/30 等を弾く。時刻は別途範囲検証済み。
			d := time.Date(s.Year, time.Month(s.Month), s.Day, 0, 0, 0, 0, time.Local)
			if d.Year() != s.Year || int(d.Month()) != s.Month || d.Day() != s.Day {
				return fmt.Errorf("予定[%d]: 実在しない日付です", i)
			}
		}
	}
	return nil
}
