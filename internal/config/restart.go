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
	// UpdateOnScheduledRestart は「予定再起動時に Resonite を更新する」トグル（P9-B）。
	// ON かつ Steam(A) 設定済みのとき、予定再起動の停止→起動の間に DepotDownloader で更新する
	// （手動/userZero/クラッシュ復帰は対象外）。Steam 未設定なら no-op。
	// 設計: docs/design/steam-depotdownloader.md §7
	UpdateOnScheduledRestart bool `json:"updateOnScheduledRestart"`
	// UpdateBeforeManualStart は「手動起動（トップバー）・手動『通常再起動』の前に Resonite を更新する」
	// トグル。ON かつ Steam(A) 設定済みのとき、起動/再起動の前に DepotDownloader を実行（＝最新確認＋適用）
	// してから起動する（予定再起動の UpdateOnScheduledRestart と対をなす）。Steam 未設定なら no-op。
	// クラッシュ復帰・通常停止は対象外。設計: docs/design/steam-depotdownloader.md §7
	UpdateBeforeManualStart bool `json:"updateBeforeManualStart"`
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

// WaitControl は「静かに待つ→告知→さらに待つ→締切で強制」の2区間モデル（R9）。
// 締切 = QuietWaitMin + AnnounceWaitMin。告知は QuietWaitMin 経過時点（＝締切の AnnounceWaitMin 前）に1回。
// 2区間は互いに独立（相互依存の検証が不要＝旧 force/actionTiming の「告知≦強制」制約を撤廃）。
// 在席0人化や③クラッシュ検知では締切を待たず早期に終端へ進む（orchestrator 側）。
type WaitControl struct {
	QuietWaitMin    int `json:"quietWaitMin"`    // 告知前に静かに待つ（分）
	AnnounceWaitMin int `json:"announceWaitMin"` // 告知後に待つ（分）。この後に強制実行
}

// PreActions は再起動前に実行するアクション群。
type PreActions struct {
	Announce       AnnounceAction `json:"announce"`       // dynamicImpulse 告知
	SessionChanges SessionChanges `json:"sessionChanges"` // セッション設定変更
}

// AnnounceAction は dynamicImpulse 告知（spawn したアイテムに impulse を送る）。
// TemplateID 非空＝テンプレ参照（URL/タグは告知実行時にリモートリストから解決＝
// docs/design/item-templates.md）。空＝手動入力（ItemURL/ImpulseTag を使う）。
type AnnounceAction struct {
	Enabled    bool   `json:"enabled"`
	TemplateID string `json:"templateId"` // 告知テンプレートの永続キー（空=手動入力）
	ItemURL    string `json:"itemUrl"`    // 手動: spawn するアイテム（空=spawn しない＝常設受け機構前提）
	ImpulseTag string `json:"impulseTag"` // 手動: dynamicimpulsestring のタグ（例 MRHC.play）
	Message    string `json:"message"`    // 送る文字列（固定文）
	SpeakerID  int64  `json:"speakerId"`  // ttsVoice テンプレート用の話者 style ID
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
// 告知は既定 OFF だが、ON にしたとき即使えるよう templateId に既定テンプレ
// （とらぞ閉店アナウンス）を入れておく。URL/タグは実行時に解決するため保存しない。
// セッション変更は maxusers=1 のみ ON、クラッシュ復帰は ON（10分に3回で停止）、
// 待機は静かに58分＋告知後2分（合計60分）。
// 予定再起動時の更新／手動起動・通常再起動前の更新はいずれも既定 ON（Steam 未設定なら no-op・P9-B）。
// templateId は server の builtinItemTemplates に含まれる既定ID・フロント
// web/src/tabs/schedule/scheduleModel.ts の defaultAnnounce() と同期すること。
func DefaultRestart() Restart {
	return Restart{
		Scheduled:   []ScheduledRestart{},
		WaitControl: WaitControl{QuietWaitMin: 58, AnnounceWaitMin: 2},
		PreActions: PreActions{
			Announce: AnnounceAction{
				Enabled:    false,
				TemplateID: "torazo-close",
				Message:    "",
			},
			SessionChanges: SessionChanges{SetMaxUsersOne: true},
		},
		CrashRecovery:            CrashRecovery{Enabled: true, MaxCrashes: 3, WindowMinutes: 10},
		UpdateOnScheduledRestart: true,
		UpdateBeforeManualStart:  true,
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
	// 2区間は互いに独立に検証（相互依存なし＝R9）。各 0〜1440 分。
	if wc.QuietWaitMin < 0 || wc.QuietWaitMin > 1440 {
		return fmt.Errorf("静かに待つ時間は 0〜1440 分で指定してください")
	}
	if wc.AnnounceWaitMin < 0 || wc.AnnounceWaitMin > 1440 {
		return fmt.Errorf("告知後に待つ時間は 0〜1440 分で指定してください")
	}
	cr := r.CrashRecovery
	if cr.MaxCrashes < 1 || cr.MaxCrashes > 100 {
		return fmt.Errorf("クラッシュ復帰の許容回数は 1〜100 で指定してください")
	}
	if cr.WindowMinutes < 1 || cr.WindowMinutes > 1440 {
		return fmt.Errorf("クラッシュ復帰の集計ウィンドウは 1〜1440 分で指定してください")
	}
	an := r.PreActions.Announce
	if an.Enabled && an.TemplateID == "" {
		// 手動入力時のみ: インパルスタグは dynamicimpulse の宛先指定に必須。
		// テンプレ参照時は URL/タグとも実行時にリストから解決するため検証しない
		// （templateId の実在は server 層が PUT 時に検証する）。
		// メッセージは任意（空可）＝受信アイテムが固定内容でメッセージを使わない場合があるため。
		if strings.TrimSpace(an.ImpulseTag) == "" {
			return fmt.Errorf("告知を有効にする場合はインパルスタグを入力してください")
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
