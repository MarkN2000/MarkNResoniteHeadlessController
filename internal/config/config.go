// Package config は MRHC の単一設定ファイル (mrhc.config.json) の
// 読み書きと、秘密情報の生成を担う。
package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
)

// FileName は設定ファイル名。データディレクトリ直下に置く。
const FileName = "mrhc.config.json"

// SchemaVersion は mrhc.config.json のスキーマ版。新規生成時に書き込む。
// 将来フィールド構造が変わった際のマイグレーション判定に使う。
const SchemaVersion = 1

// DefaultSessionTTLHours はセッション（Cookie）の既定有効期間（30日）。
// stateless HMAC トークンの絶対失効時刻に使う。設計: internal/server/auth.go
const DefaultSessionTTLHours = 720

// Config はアプリ全体の設定。秘密情報を含むため保存時はパーミッション 0600。
type Config struct {
	Version             int                 `json:"version"`                       // スキーマ版（SchemaVersion）
	AdminPasswordHash   string              `json:"adminPasswordHash"`             // bcryptハッシュ
	SessionSecret       string              `json:"sessionSecret"`                 // セッションCookie署名用（自動生成）
	SessionTTLHours     int                 `json:"sessionTtlHours,omitempty"`     // セッション有効期間（時間。空/0=既定720h=30日）
	Port                int                 `json:"port"`                          // HTTP待受ポート
	HeadlessConfigDir   string              `json:"headlessConfigDir,omitempty"`   // ヘッドレスconfig格納先（空=既定 {dataDir}/headless-configs）
	HeadlessCredentials HeadlessCredentials `json:"headlessCredentials,omitempty"` // 既定の Resonite アカウント（起動時に各 config へ注入）
	Encoding            string              `json:"encoding,omitempty"`            // コンソール文字コード上書き（空=OS既定。"utf-8"/"shift_jis"等）
	Language            string              `json:"language,omitempty"`            // CLI/起動メッセージ/sys案内の表示言語（"ja"/"en"。空=ja・ウィザードS0が保存）
	Restart             *Restart            `json:"restart,omitempty"`             // 自動再起動設定（未設定=DefaultRestart・§3.16）
	Steam               *Steam              `json:"steam,omitempty"`               // Resonite入手/更新用 Steam アカウント(A)（DepotDownloader・P9-B）
}

// Steam は DepotDownloader による Resonite 入手/更新に使う「DL 用 Steam アカウント(A)」。
// ヘッドレスの bot 身元である Resonite アカウント(B)＝HeadlessCredentials とは別物。
// 秘密（Password/BranchCode）を含むため保存は 0600。公開 API では値を返さず hasXxx(bool) で表す。
// 設計: docs/design/steam-depotdownloader.md
type Steam struct {
	Username   string `json:"username,omitempty"`   // Steam ユーザー名
	Password   string `json:"password,omitempty"`   // Steam パスワード（ASCII・最大64文字・復元可能保存）
	BranchCode string `json:"branchCode,omitempty"` // headless branch password（Patreon 配布・変動しうる）
	InstallDir string `json:"installDir,omitempty"` // DL/更新先（空=既定 {dataDir}/resonite を導出・InstallDirOrDefault）
}

// HeadlessCredentials は生成/起動する headless config に注入する既定の Resonite アカウント。
// 起動時に config の loginCredential/loginPassword が空ならこれを注入する（per-config 指定があればそちらを優先）。
type HeadlessCredentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// UserID は username から解決した Resonite UserID（U-xxx）。保存時に解決して保持し、
	// customSessionId の prefix 自動入力など UserID が要る箇所で再利用する（R12）。空=未解決。
	UserID string `json:"userId,omitempty"`
}

// HeadlessConfigDirOrDefault は headless config 保存ディレクトリを解決する。
// 明示設定があればそれを、無ければ {dataDir}/headless-configs を返す。
func (c *Config) HeadlessConfigDirOrDefault(dataDir string) string {
	if strings.TrimSpace(c.HeadlessConfigDir) != "" {
		return c.HeadlessConfigDir
	}
	return filepath.Join(dataDir, "headless-configs")
}

// InstallDirOrDefault は Resonite の入手/更新先（DepotDownloader の -dir 対象＝.../Resonite）を解決する。
// 優先順: ①明示 Steam.InstallDir → ②既定 {dataDir}/resonite。
// バンドル既定方針(R-A): パス未指定なら既存 Steam インストールの有無に関わらず {dataDir}/resonite を使う
// （自己完結1フォルダ・二重管理の衝突回避・DL 前はパスが無い鶏卵問題の解消）。
// 既存インストールを使いたい上級者は Steam.InstallDir にそのフォルダを明示してオプトアウトする。
//
// 起動先（HeadlessPathOrDefault）も DL 先（本関数）もこの値から導出されるため、両者は常に同一フォルダに収束する。
// 純関数（OS 非依存・ファイルアクセスなし）。"~" 展開は利用側で行う。
func (c *Config) InstallDirOrDefault(dataDir string) string {
	if c.Steam != nil {
		if d := strings.TrimSpace(c.Steam.InstallDir); d != "" {
			return d
		}
	}
	return filepath.Join(dataDir, "resonite")
}

// HeadlessPathOrDefault はヘッドレス実行ファイルのパスを解決する。
// 常に InstallDirOrDefault/Headless/<binaryName> を導出する（Resonite 正規レイアウト前提）。
// binaryName は OS 別（Windows=Resonite.exe / 他=Resonite.dll）を呼び出し側が注入する
// （config を OS 非依存に保つための依存性注入）。純関数。"~" 展開は利用側で行う。
func (c *Config) HeadlessPathOrDefault(dataDir, binaryName string) string {
	return filepath.Join(c.InstallDirOrDefault(dataDir), "Headless", binaryName)
}

// LangOrDefault は CLI・起動メッセージ・sys 案内の表示言語を返す。
// "en" 以外（空・未知の値）はすべて ja に倒す——language フィールドが無い既存 config の
// 利用者は全員日本語話者のため（2026-06-06 ユーザー裁定）。新規はウィザード S0 が必ず明示保存する。
func (c *Config) LangOrDefault() i18n.Lang {
	if c.Language == string(i18n.En) {
		return i18n.En
	}
	return i18n.Ja
}

// SessionTTL はセッション有効期間を返す。未設定（0以下）なら既定30日。
func (c *Config) SessionTTL() time.Duration {
	h := c.SessionTTLHours
	if h <= 0 {
		h = DefaultSessionTTLHours
	}
	return time.Duration(h) * time.Hour
}

// FileExists は設定ファイルの有無を返す。
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// LoadFrom は設定ファイルを読み込む。
func LoadFrom(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// SaveTo は設定ファイルを 0600 で保存する（秘密情報を含むため）。
func (c *Config) SaveTo(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// RandomSecret は URL-safe な乱数文字列を生成する（APIキー・セッション秘密用）。
func RandomSecret(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
