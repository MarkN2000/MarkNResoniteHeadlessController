// Package config は MRHC の単一設定ファイル (mrhc.config.json) の
// 読み書きと、秘密情報の生成を担う。
package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
)

// FileName は設定ファイル名。データディレクトリ直下に置く。
const FileName = "mrhc.config.json"

// Config はアプリ全体の設定。秘密情報を含むため保存時はパーミッション 0600。
type Config struct {
	AdminPasswordHash string `json:"adminPasswordHash"`          // bcryptハッシュ
	APIKey            string `json:"apiKey"`                     // スクリプト/ワールド内操作用（再生成可）
	SessionSecret     string `json:"sessionSecret"`              // セッションCookie署名用（自動生成）
	Port              int    `json:"port"`                       // HTTP待受ポート
	ResoniteHeadless  string `json:"resoniteHeadlessPath,omitempty"`  // Resonite.exe / Resonite.dll
	HeadlessConfigDir string `json:"headlessConfigDir,omitempty"`     // ヘッドレスconfig格納先
	Encoding          string `json:"encoding,omitempty"`              // コンソール文字コード上書き（空=OS既定。"utf-8"/"shift_jis"等）
	// 後続でHeadlessCredentials / Restart / Steam / AllowedCidrs などを追加
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
