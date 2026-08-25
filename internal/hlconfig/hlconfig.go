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
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

const (
	schemaURL               = "https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json"
	legacyTTSAllowedURLHost = "https://ttsapi.markn2000.com/"
	ttsAllowedURLHost       = "https://tts.markn2000.com/"
)

var (
	ErrInvalidName     = errors.New("不正な config 名（文字・数字・_・- のみ、1〜64文字）")
	ErrReservedName    = errors.New("この名前は予約語のため使用できません")
	ErrNotFound        = errors.New("config が見つかりません")
	ErrStartWorldsType = errors.New("startWorlds は配列である必要があります")
	ErrInvalidJSON     = errors.New("不正な JSON")
	ErrUDPPortConflict = errors.New("起動対象の LNL/QUIC 固定ポートが重複しています")
	// ErrFolderCreate は EnsureFolders（起動前の dataFolder/cacheFolder 作成）の失敗。
	// config に書かれたパス起因＝ユーザーが直せるエラーのため、HTTP 層は 409 にマップする
	// （500 だと「サーバー内部エラー」に見えて原因に辿り着けない）。
	ErrFolderCreate = errors.New("フォルダを作成できません")
)

// nameRe は config 名の許可文字。文字(\p{L})・数字(\p{N})・結合文字(\p{M})・`_`・`-` のみ許可し、
// `/` `\` `.` や空白・記号を含まないためパストラバーサル不可。日本語（かな・漢字・長音符ー等）は
// \p{L}、濁点等の結合文字（NFD 入力で分解された分）は \p{M} で受理する。{1,64} はルーン数。
var nameRe = regexp.MustCompile(`^[\p{L}\p{N}\p{M}_\-]{1,64}$`)

// reservedNameRe は Windows のデバイス予約名（CON.json のように拡張子付きでも予約扱い）。
// Linux で作った config 名を Windows へ移しても壊れないよう、全 OS で拒否する（大小無視）。
var reservedNameRe = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])$`)

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

// SanitizeName は config 名を検証する（パストラバーサル防止）。許可文字・長さを正規表現で確認し、
// 続けて Windows 予約名（CON/NUL/COM1 等・大小無視）を拒否する。NFC 正規化はファイル名生成側
// （pathFor）が一手に担うため、ここでは形を問わず検証する（NFC/NFD どちらも \p{L}+\p{M} で受理）。
func SanitizeName(name string) error {
	if !nameRe.MatchString(name) {
		return ErrInvalidName
	}
	if reservedNameRe.MatchString(name) {
		return ErrReservedName
	}
	return nil
}

// NormalizeName は config 名を Unicode NFC へ正規化する。pathFor を通さない経路（表示・last-used
// 記録・起動ラベル・スケジュール保存）で表記ゆれ（NFC/NFD）をそろえ、ディスク上の正準名と一致させる。
// ファイル操作側の正規化は pathFor が担うため、ファイル名の生成にこの関数を使う必要はない。
func NormalizeName(name string) string { return norm.NFC.String(name) }

// pathFor は config 名から保存パスを組み立てる唯一の関所。名前を NFC へ正規化してから連結するため、
// 入力が NFC/NFD どちらでもディスク上の実体は常に正準形になり、作成・取得・削除・起動の往復が安定する。
func pathFor(dir, name string) string { return filepath.Join(dir, norm.NFC.String(name)+".json") }

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
	normalizeAllowedURLHosts(m)
	m["loginPassword"] = ""
	return m, nil
}

// Write は config を保存する。
//   - startWorlds が（存在し）配列でなければエラー
//   - loginPassword が空なら既存ファイルの値を保持（GET でマスクされるため空送信＝変更なし）
//   - $schema を付与し 0600 で保存
func Write(dir, name string, body map[string]any) error {
	return writeMasked(dir, name, name, body)
}

// WriteRenamed は body を newName で保存し、成功後に oldName を削除する（保存＝リネーム）。
// マスクされた loginPassword（空）は oldName 側のファイルから解決する（Write の解決先は
// 保存先名のため、リネームでは元 config の password が失われる）。oldName 不在は ErrNotFound。
// 書き込み成功 → 削除の順なので、途中で失敗してもデータは失われない（両方残る＝リトライ可能）。
func WriteRenamed(dir, oldName, newName string, body map[string]any) error {
	if err := SanitizeName(oldName); err != nil {
		return err
	}
	// 同名判定は NFC 正準形で行う。生バイトのままだと NFC/NFD 違いで「別名＝改名」と誤判定し、
	// 「新名で書き込み → pathFor の正規化で同一ファイルに上書き → 旧名（=同一ファイル）を削除」で
	// 保存内容を失う。pathFor が NFC 化するので、等価判定もそれに合わせる。
	if norm.NFC.String(oldName) == norm.NFC.String(newName) {
		return Write(dir, newName, body)
	}
	if _, err := os.Stat(pathFor(dir, oldName)); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	if err := writeMasked(dir, newName, oldName, body); err != nil {
		return err
	}
	return os.Remove(pathFor(dir, oldName))
}

// writeMasked は Write の本体。マスクされた loginPassword（空）の解決先ファイルを
// maskSource で指定する（通常保存=保存先自身・リネーム=旧名）。
func writeMasked(dir, name, maskSource string, body map[string]any) error {
	if err := SanitizeName(name); err != nil {
		return err
	}
	if sw, ok := body["startWorlds"]; ok && sw != nil {
		if _, isArr := sw.([]any); !isArr {
			return ErrStartWorldsType
		}
	}
	normalizeForcePorts(body)
	normalizeAllowedURLHosts(body)
	if err := validateUDPPortConflicts(body); err != nil {
		return err
	}
	if pw, _ := body["loginPassword"].(string); pw == "" {
		if existing, err := readRaw(dir, maskSource); err == nil {
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

// normalizeAllowedURLHosts は旧TTSホストと完全一致する配列要素だけを現行ホストへ置換する。
// 現行ホストが既に含まれる場合は1件へまとめ、その他の要素や不正な型はそのまま保持する。
func normalizeAllowedURLHosts(m map[string]any) {
	hosts, ok := m["allowedUrlHosts"].([]any)
	if !ok {
		return
	}

	out := make([]any, 0, len(hosts))
	seenCurrent := false
	changed := false
	for _, host := range hosts {
		s, isString := host.(string)
		if !isString {
			out = append(out, host)
			continue
		}
		if s == legacyTTSAllowedURLHost {
			s = ttsAllowedURLHost
			changed = true
		}
		if s == ttsAllowedURLHost {
			if seenCurrent {
				changed = true
				continue
			}
			seenCurrent = true
		}
		out = append(out, s)
	}
	if changed {
		m["allowedUrlHosts"] = out
	}
}

// normalizeForcePorts は旧 forcePort を新しいプロトコル別辞書へ正規化する。
// 旧値は LNL にだけ対応する。新旧が併存して forcePorts.lnl が既にある場合は新形式を優先し、
// 辞書内の未知プロトコルはそのまま温存する。forcePorts が辞書でない不正な形式なら、
// 既存値を壊さないよう旧 forcePort も含めて変更しない。
func normalizeForcePorts(m map[string]any) {
	worlds, ok := m["startWorlds"].([]any)
	if !ok {
		return
	}
	for _, item := range worlds {
		world, ok := item.(map[string]any)
		if !ok {
			continue
		}

		legacy, hasLegacy := world["forcePort"]
		if !hasLegacy {
			continue
		}

		if legacy == nil {
			delete(world, "forcePort")
			continue
		}

		portsValue, hasPorts := world["forcePorts"]
		if !hasPorts || portsValue == nil {
			world["forcePorts"] = map[string]any{"lnl": legacy}
			delete(world, "forcePort")
			continue
		}
		ports, ok := portsValue.(map[string]any)
		if !ok {
			continue
		}
		if _, hasLNL := ports["lnl"]; !hasLNL {
			ports["lnl"] = legacy
		}
		delete(world, "forcePort")
	}
}

// validateUDPPortConflicts は、起動対象ワールドの LNL/QUIC 固定 UDP ポートが
// 同一ワールド内またはワールド間で重複していないことを確認する。
func validateUDPPortConflicts(m map[string]any) error {
	worlds, ok := m["startWorlds"].([]any)
	if !ok {
		return nil
	}

	used := make(map[uint16]struct{})
	for _, item := range worlds {
		world, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if enabled, exists := world["isEnabled"].(bool); exists && !enabled {
			continue
		}
		ports, ok := world["forcePorts"].(map[string]any)
		if !ok {
			continue
		}
		for _, protocol := range []string{"lnl", "quic"} {
			port, ok := fixedPortNumber(ports[protocol])
			if !ok {
				continue
			}
			if _, exists := used[port]; exists {
				return fmt.Errorf("%w: %d", ErrUDPPortConflict, port)
			}
			used[port] = struct{}{}
		}
	}
	return nil
}

func fixedPortNumber(value any) (uint16, bool) {
	switch port := value.(type) {
	case float64:
		if port < 1 || port > 65535 || port != float64(uint16(port)) {
			return 0, false
		}
		return uint16(port), true
	case int:
		if port < 1 || port > 65535 {
			return 0, false
		}
		return uint16(port), true
	default:
		return 0, false
	}
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
	normalizeForcePorts(m)
	normalizeAllowedURLHosts(m)
	if err := validateUDPPortConflicts(m); err != nil {
		return "", err
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

// DefaultFolders は新規 config 雛形に焼き込む dataFolder/cacheFolder の既定値を返す。
// {dataDir}/headless-data・{dataDir}/headless-cache を必ず絶対パスで返す
// （相対 dataFolder は headless が即クラッシュ＝ログ無しで落ちるため Abs 必須）。
// dataDir 既定（-data 未指定）は mrhc 実行ファイルと同じフォルダ＝「mrhc と同じ階層」になる。
// EnsureDefault（default.json）と /api/v1/headless-config-defaults（UI 新規作成）の単一情報源。
func DefaultFolders(dataDir string) (dataFolder, cacheFolder string, err error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(abs, "headless-data"), filepath.Join(abs, "headless-cache"), nil
}

// jsonString は Go 文字列を JSON 文字列リテラルにする（Windows パスの \ を確実にエスケープ）。
func jsonString(s string) string {
	b, _ := json.Marshal(s) // string の Marshal は失敗しない
	return string(b)
}

// bakedDefaultJSON は同梱テンプレに DefaultFolders(dataDir) の絶対パスを焼き込んで返す
// （EnsureDefault と Create の共用＝テンプレの単一情報源）。
// 雛形は手書き整形 JSON のため map 経由（キー順が壊れる）でなく文字列置換で値だけ差し込む。
// DefaultFolders が失敗（Abs 不能・稀）した場合は null のまま＝headless 既定に委譲する。
func bakedDefaultJSON(dataDir string) string {
	body := defaultConfigJSON
	if dataFolder, cacheFolder, err := DefaultFolders(dataDir); err == nil {
		body = strings.Replace(body, `"dataFolder": null`, `"dataFolder": `+jsonString(dataFolder), 1)
		body = strings.Replace(body, `"cacheFolder": null`, `"cacheFolder": `+jsonString(cacheFolder), 1)
	}
	return body
}

// EnsureDefault は dir に config が1つも無ければ同梱デフォルト(default.json)を作る。
// dataFolder/cacheFolder には DefaultFolders(dataDir) の絶対パスを焼き込む（表示=保存値一致）。
// comment はテンプレでは空で、default.json にだけ説明文を注入する（jsonString＝エスケープ安全）。
func EnsureDefault(dir, dataDir string) error {
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
	body := strings.Replace(bakedDefaultJSON(dataDir),
		`"comment": ""`, `"comment": `+jsonString(defaultConfigComment), 1)
	return os.WriteFile(filepath.Join(dir, "default.json"), []byte(body), 0o600)
}

// pickUniqueName は base, base2, base3, … の最初の未使用名を返す（即時作成の自動命名）。
// 64文字（nameRe 上限・ルーン単位）を超える場合は base 側をルーン単位で切り詰める。
// 日本語名の複製 {name}-copy 等で base がマルチバイトでも、文字の途中で切れて壊れない
// （suffix は ASCII 数字＝バイト長＝ルーン数なので len(suffix) をそのまま使える）。
func pickUniqueName(dir, base string) (string, error) {
	for n := 1; n <= 9999; n++ {
		suffix := ""
		if n >= 2 {
			suffix = strconv.Itoa(n)
		}
		b := []rune(base)
		if len(b)+len(suffix) > 64 {
			b = b[:64-len(suffix)]
		}
		cand := string(b) + suffix
		if _, err := os.Stat(pathFor(dir, cand)); os.IsNotExist(err) {
			return cand, nil
		}
	}
	return "", errors.New("空き名を採番できません")
}

// Create は同梱テンプレ（dataFolder/cacheFolder 焼き込み済み・comment 空）から新しい config を
// 即時作成し、作成した名前（new-config, new-config2, …）を返す。default.json 用の説明文は
// EnsureDefault 側で注入するため、こちらはテンプレそのまま＝フロント defaultConfig() と一致。
func Create(dir, dataDir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name, err := pickUniqueName(dir, "new-config")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(pathFor(dir, name), []byte(bakedDefaultJSON(dataDir)), 0o600); err != nil {
		return "", err
	}
	return name, nil
}

// Duplicate は config をサーバー側でバイトコピーし、コピーの名前（{name}-copy, {name}-copy2, …）
// を返す。フロント経由（GET マスク → PUT）と違い loginPassword・未知フィールド・整形が
// そのまま写る（マスク済み password の喪失が構造的に起きない）。
func Duplicate(dir, name string) (string, error) {
	if err := SanitizeName(name); err != nil {
		return "", err
	}
	b, err := os.ReadFile(pathFor(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	newName, err := pickUniqueName(dir, name+"-copy")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(pathFor(dir, newName), b, 0o600); err != nil {
		return "", err
	}
	return newName, nil
}

// EnsureFolders は config の dataFolder/cacheFolder（設定されている場合のみ）を起動前に作成する。
// headless 側がフォルダを自作するかは実機未確認のため安全側で MkdirAll する（存在すれば no-op）。
// 絶対パスのみ対象（相対値はカレントディレクトリ汚染を避けて作らない＝headless の挙動は従来どおり）。
// 作成失敗はエラーで返して起動を止める（headless 側の分かりにくいクラッシュより明示的な 409 で見せる）。
func EnsureFolders(dir, name string) error {
	m, err := readRaw(dir, name)
	if err != nil {
		return err
	}
	for _, key := range []string{"dataFolder", "cacheFolder"} {
		v, _ := m[key].(string)
		if v == "" || !filepath.IsAbs(v) {
			continue
		}
		if err := os.MkdirAll(v, 0o755); err != nil {
			return fmt.Errorf("%w: %s（%s）: %v", ErrFolderCreate, key, v, err)
		}
	}
	return nil
}

// defaultConfigJSON は同梱デフォルト config。専用フォーム（①一般＋②上級）のキーのみ明示し、
// UI 表示と保存値を一致させる（フロント defaultWorld()/defaultConfig() と同方針＝スリム化）。
// 任意項目（universeId/forcePorts/各クラウド変数/mobileFriendly/autoRecover 等）は未設定時に
// 雛形へ入れない。決定値: accessLevel=Anyone・awayKickMinutes=5・autoSleep=true・
// idleRestartInterval=1800・強制再起動/自動保存=-1(無効)・creds 空（中央注入）。
// 注: autoRecover は雛形から外し headless 既定へ委譲（既定値 true は明示しない）。
//
// defaultConfigComment は default.json（同梱デフォルト）にだけ入れる説明文。雛形の comment は
// 空で持ち、EnsureDefault が jsonString（json.Marshal）でエスケープして注入する＝説明文に
// 使える文字の制約は無い。Create（UI 新規作成）は雛形そのまま＝説明文を引き継がない。
const defaultConfigComment = "MRHC デフォルト設定（Settings で Resonite アカウントを登録し、必要に応じて編集してください）"

const defaultConfigJSON = `{
  "$schema": "https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json",
  "comment": "",
  "tickRate": 60.0,
  "maxConcurrentAssetTransfers": 128,
  "usernameOverride": null,
  "loginCredential": "",
  "loginPassword": "",
  "startWorlds": [
    {
      "isEnabled": true,
      "sessionName": null,
      "description": null,
      "accessLevel": "Anyone",
      "maxUsers": 16,
      "loadWorldPresetName": "Grid",
      "loadWorldURL": null,
      "customSessionId": null,
      "tags": null,
      "awayKickMinutes": 5.0,
      "idleRestartInterval": 1800,
      "defaultUserRoles": null,
      "autoInviteUsernames": null,
      "autoInviteMessage": null,
      "forcedRestartInterval": -1.0,
      "autosaveInterval": -1.0,
      "saveOnExit": false,
      "autoSleep": true,
      "hideFromPublicListing": false,
      "inviteRequestHandlerUsernames": null,
      "saveAsOwner": null,
      "enableResoniteLink": false,
      "forceResoniteLinkPort": null
    }
  ],
  "dataFolder": null,
  "cacheFolder": null,
  "logsFolder": null,
  "allowedUrlHosts": [ "https://tts.markn2000.com/" ],
  "autoSpawnItems": null
}
`
