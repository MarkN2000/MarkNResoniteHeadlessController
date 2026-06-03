// Package hlconfig は Resonite ヘッドレス用 config ファイル(*.json)の CRUD と、
// 起動時の認証情報注入を担う。HTTP 層から独立し単体テスト可能（SOLID: 単一責務）。
//
// 保存型モデル: config の中身は不透明な JSON（map）として扱い、こちらが触るのは
//   - name のサニタイズ（パストラバーサル防止）
//   - loginCredential/loginPassword（GET でマスク、保存で保持、起動で注入）
//   - $schema 付与、startWorlds が配列かの最小検証
//
// のみ。未知フィールドは保持する（公式スキーマ ~40 フィールドを全模写しない）。
package hlconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const schemaURL = "https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json"

var (
	ErrInvalidName     = errors.New("不正な config 名（英数・_・- のみ、1〜64文字）")
	ErrNotFound        = errors.New("config が見つかりません")
	ErrStartWorldsType = errors.New("startWorlds は配列である必要があります")
	ErrInvalidJSON     = errors.New("不正な JSON")
)

// nameRe は config 名の許可文字。`/` `\` `.` を含まないためパストラバーサル不可。
var nameRe = regexp.MustCompile(`^[A-Za-z0-9_\-]{1,64}$`)

// Credentials は起動時に注入する Resonite アカウント。
type Credentials struct {
	Username string
	Password string
}

// Summary は一覧表示用の config 要約。
type Summary struct {
	Name       string `json:"name"`
	Comment    string `json:"comment"`
	WorldCount int    `json:"worldCount"`
}

// SanitizeName は config 名を検証する（パストラバーサル防止）。
func SanitizeName(name string) error {
	if !nameRe.MatchString(name) {
		return ErrInvalidName
	}
	return nil
}

func pathFor(dir, name string) string { return filepath.Join(dir, name+".json") }

// readRaw は config を map として読む（内部用・password を含む）。
func readRaw(dir, name string) (map[string]any, error) {
	b, err := os.ReadFile(pathFor(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// List は dir 内の *.json を要約して返す（name 昇順）。dir 不在は空リスト。
func List(dir string) ([]Summary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Summary{}, nil
		}
		return nil, err
	}
	out := make([]Summary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		s := Summary{Name: name}
		if m, err := readRaw(dir, name); err == nil {
			if c, ok := m["comment"].(string); ok {
				s.Comment = c
			}
			if sw, ok := m["startWorlds"].([]any); ok {
				s.WorldCount = len(sw)
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ReadMasked は GET 用に loginPassword をマスク("")した config を返す。
func ReadMasked(dir, name string) (map[string]any, error) {
	if err := SanitizeName(name); err != nil {
		return nil, err
	}
	m, err := readRaw(dir, name)
	if err != nil {
		return nil, err
	}
	m["loginPassword"] = ""
	return m, nil
}

// Write は config を保存する。
//   - startWorlds が（存在し）配列でなければエラー
//   - loginPassword が空なら既存ファイルの値を保持（GET でマスクされるため空送信＝変更なし）
//   - $schema を付与し 0600 で保存
func Write(dir, name string, body map[string]any) error {
	if err := SanitizeName(name); err != nil {
		return err
	}
	if sw, ok := body["startWorlds"]; ok && sw != nil {
		if _, isArr := sw.([]any); !isArr {
			return ErrStartWorldsType
		}
	}
	if pw, _ := body["loginPassword"].(string); pw == "" {
		if existing, err := readRaw(dir, name); err == nil {
			if oldPw, ok := existing["loginPassword"].(string); ok && oldPw != "" {
				body["loginPassword"] = oldPw
			}
		}
	}
	body["$schema"] = schemaURL
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pathFor(dir, name), b, 0o600)
}

// Delete は config を削除する。
func Delete(dir, name string) error {
	if err := SanitizeName(name); err != nil {
		return err
	}
	if err := os.Remove(pathFor(dir, name)); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// ResolveForLaunch は起動用に認証情報を注入した config を runDir に書き、そのパスを返す。
// config 自身の loginCredential/loginPassword が空なら central を注入（per-config 指定を優先）。
// 保存済みファイルには password を焼き込まず、平文は中央設定と起動用一時ファイルのみに存在する。
func ResolveForLaunch(dir, name string, central Credentials, runDir string) (string, error) {
	if err := SanitizeName(name); err != nil {
		return "", err
	}
	m, err := readRaw(dir, name)
	if err != nil {
		return "", err
	}
	// all-or-nothing: config がアカウント未指定(loginCredential 空)のときだけ
	// 中央アカウントを username+password まとめて注入する。loginCredential が
	// 非空なら per-config アカウントとして config 自身の値をそのまま使う
	// （password 空でも中央 password を混入させない＝別アカウントの組合せを防ぐ）。
	if u, _ := m["loginCredential"].(string); u == "" {
		m["loginCredential"] = central.Username
		m["loginPassword"] = central.Password
	}
	m["$schema"] = schemaURL
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", err
	}
	out := pathFor(runDir, name)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(out, b, 0o600); err != nil {
		return "", err
	}
	return out, nil
}

// EnsureDefault は dir に config が1つも無ければ同梱デフォルト(default.json)を作る。
func EnsureDefault(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			return nil // 既に config がある
		}
	}
	return os.WriteFile(filepath.Join(dir, "default.json"), []byte(defaultConfigJSON), 0o600)
}

// defaultConfigJSON は同梱デフォルト config。公式スキーマ全項目を明示し、UI 表示と保存値を一致させる
// （フロント defaultWorld()/defaultConfig() と同方針）。accessLevel=Anyone・awayKickMinutes=5・
// autoSleep/autoRecover=true・idleRestartInterval=1800・強制再起動/自動保存=-1(無効)・creds 空（中央注入）。
const defaultConfigJSON = `{
  "$schema": "https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json",
  "comment": "MRHC デフォルト設定（Settings で Resonite アカウントを登録し、必要に応じて編集してください）",
  "universeId": null,
  "tickRate": 60.0,
  "maxConcurrentAssetTransfers": 128,
  "usernameOverride": null,
  "loginCredential": "",
  "loginPassword": "",
  "startWorlds": [
    {
      "isEnabled": true,
      "sessionName": null,
      "customSessionId": null,
      "description": null,
      "maxUsers": 16,
      "accessLevel": "Anyone",
      "useCustomJoinVerifier": false,
      "hideFromPublicListing": false,
      "tags": null,
      "mobileFriendly": false,
      "loadWorldURL": null,
      "loadWorldPresetName": "Grid",
      "overrideCorrespondingWorldId": null,
      "forcePort": null,
      "enableResoniteLink": false,
      "forceResoniteLinkPort": null,
      "keepOriginalRoles": false,
      "defaultUserRoles": null,
      "roleCloudVariable": null,
      "allowUserCloudVariable": null,
      "denyUserCloudVariable": null,
      "requiredUserJoinCloudVariable": null,
      "requiredUserJoinCloudVariableDenyMessage": null,
      "awayKickMinutes": 5.0,
      "parentSessionIds": null,
      "autoInviteUsernames": null,
      "autoInviteMessage": null,
      "saveAsOwner": null,
      "autoRecover": true,
      "idleRestartInterval": 1800,
      "forcedRestartInterval": -1.0,
      "saveOnExit": false,
      "autosaveInterval": -1.0,
      "autoSleep": true
    }
  ],
  "dataFolder": null,
  "cacheFolder": null,
  "logsFolder": null,
  "allowedUrlHosts": null,
  "autoSpawnItems": null
}
`
