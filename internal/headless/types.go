package headless

// 構造化コマンド応答のドメインモデル。
// パース後の戻り値型として、SSE/REST のJSON応答にもそのまま使う想定。
// 詳細設計: docs/design/structured-driver.md, ドメイン事実: docs/resonite-domain-facts.md。

// World は worlds コマンドが返す1ワールドの要約。
type World struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Users       int    `json:"users"`
	Present     int    `json:"present"`
	AccessLevel string `json:"accessLevel"`
	MaxUsers    int    `json:"maxUsers"`
}

// WorldStatus は status コマンドが返すフォーカス中ワールドの詳細。
// 命名: Driver.Status（プロセス状態）と衝突回避のため WorldStatus。
// ResoniteLink は 2026-05-28 実機採取で確認された新Key（旧コード非対応）。
type WorldStatus struct {
	Name              string   `json:"name"`
	SessionID         string   `json:"sessionId"`
	CurrentUsers      int      `json:"currentUsers"`
	PresentUsers      int      `json:"presentUsers"`
	MaxUsers          int      `json:"maxUsers"`
	Uptime            string   `json:"uptime"` // "HH:MM:SS.fff..." 生文字列で保持
	AccessLevel       string   `json:"accessLevel"`
	HiddenFromListing bool     `json:"hiddenFromListing"`
	MobileFriendly    bool     `json:"mobileFriendly"`
	Description       string   `json:"description"`
	Tags              []string `json:"tags"`
	Users             []string `json:"users"`
	ResoniteLink      string   `json:"resoniteLink"` // "on"/"off" 等
}

// UserInfo は users コマンドが返す1ユーザーの情報。
// ID は無アカウントユーザー（ヘッドレス自身など）では空文字。
type UserInfo struct {
	Name     string  `json:"name"`
	ID       string  `json:"id"`
	Role     string  `json:"role"`
	Present  bool    `json:"present"`
	PingMs   int     `json:"pingMs"`
	FPS      float64 `json:"fps"`
	Silenced bool    `json:"silenced"`
}

// BanEntry は listbans コマンドが返す1件のBAN情報。
type BanEntry struct {
	Index      int      `json:"index"`
	Username   string   `json:"username"`
	UserID     string   `json:"userId"`
	MachineIDs []string `json:"machineIds"`
}
