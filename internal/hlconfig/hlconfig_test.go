package hlconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	ok := []string{"default", "my-world", "event_2026", "ABC123"}
	for _, n := range ok {
		if err := SanitizeName(n); err != nil {
			t.Errorf("valid name %q rejected: %v", n, err)
		}
	}
	bad := []string{"", "a/b", "a\\b", "..", "../x", "a.b", "a b", "secret!", strings.Repeat("x", 65)}
	for _, n := range bad {
		if err := SanitizeName(n); err == nil {
			t.Errorf("invalid name %q accepted", n)
		}
	}
}

// writeJSON は raw map をファイルへ書く（テスト用ヘルパ）。
func writeRawFile(t *testing.T, dir, name string, m map[string]any) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, name+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWrite_And_ReadMasked(t *testing.T) {
	dir := t.TempDir()
	body := map[string]any{
		"loginCredential": "user@example.com",
		"loginPassword":   "secret",
		"startWorlds":     []any{map[string]any{"sessionName": "W"}},
	}
	if err := Write(dir, "cfg", body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// readRaw は password を保持
	raw, err := readRaw(dir, "cfg")
	if err != nil {
		t.Fatal(err)
	}
	if raw["loginPassword"] != "secret" {
		t.Fatalf("password not stored: %v", raw["loginPassword"])
	}
	if raw["$schema"] != schemaURL {
		t.Fatalf("$schema not injected")
	}
	// ReadMasked は password を空に
	masked, err := ReadMasked(dir, "cfg")
	if err != nil {
		t.Fatal(err)
	}
	if masked["loginPassword"] != "" {
		t.Fatalf("password not masked: %v", masked["loginPassword"])
	}
	if masked["loginCredential"] != "user@example.com" {
		t.Fatalf("username should be returned: %v", masked["loginCredential"])
	}
}

func TestWrite_StartWorldsMustBeArray(t *testing.T) {
	dir := t.TempDir()
	err := Write(dir, "cfg", map[string]any{"startWorlds": "notarray"})
	if !errors.Is(err, ErrStartWorldsType) {
		t.Fatalf("expected ErrStartWorldsType, got %v", err)
	}
}

func TestWrite_PreservesPasswordOnEmpty(t *testing.T) {
	dir := t.TempDir()
	// 初回: password あり
	if err := Write(dir, "cfg", map[string]any{"loginPassword": "secret", "comment": "v1"}); err != nil {
		t.Fatal(err)
	}
	// 2回目: password 空（=変更なし）で comment だけ変更
	if err := Write(dir, "cfg", map[string]any{"loginPassword": "", "comment": "v2"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := readRaw(dir, "cfg")
	if raw["loginPassword"] != "secret" {
		t.Fatalf("password not preserved on empty submit: %v", raw["loginPassword"])
	}
	if raw["comment"] != "v2" {
		t.Fatalf("comment not updated: %v", raw["comment"])
	}
}

func TestResolveForLaunch_InjectsCentral(t *testing.T) {
	dir := t.TempDir()
	runDir := t.TempDir()
	// creds 空の config
	writeRawFile(t, dir, "cfg", map[string]any{
		"loginCredential": "",
		"loginPassword":   "",
		"startWorlds":     []any{},
	})
	central := Credentials{Username: "central@e.com", Password: "centralpw"}
	out, err := ResolveForLaunch(dir, "cfg", central, runDir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["loginCredential"] != "central@e.com" || m["loginPassword"] != "centralpw" {
		t.Fatalf("central creds not injected: %v / %v", m["loginCredential"], m["loginPassword"])
	}
}

func TestResolveForLaunch_PerConfigOverride(t *testing.T) {
	dir := t.TempDir()
	runDir := t.TempDir()
	// config 自身が creds を持つ → 中央より優先
	writeRawFile(t, dir, "cfg", map[string]any{
		"loginCredential": "own@e.com",
		"loginPassword":   "ownpw",
	})
	central := Credentials{Username: "central@e.com", Password: "centralpw"}
	out, err := ResolveForLaunch(dir, "cfg", central, runDir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["loginCredential"] != "own@e.com" || m["loginPassword"] != "ownpw" {
		t.Fatalf("per-config creds should win: %v / %v", m["loginCredential"], m["loginPassword"])
	}
}

func TestResolveForLaunch_UsernameOnly_NoCentralPasswordMix(t *testing.T) {
	// config が username だけ持ち password 空 → all-or-nothing なので
	// 中央 password を混入させず、config の値（空）のまま（別アカウント組合せを防ぐ）。
	dir := t.TempDir()
	runDir := t.TempDir()
	writeRawFile(t, dir, "cfg", map[string]any{
		"loginCredential": "bot_account",
		"loginPassword":   "",
	})
	central := Credentials{Username: "central@e.com", Password: "centralpw"}
	out, err := ResolveForLaunch(dir, "cfg", central, runDir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["loginCredential"] != "bot_account" {
		t.Fatalf("per-config username should be kept: %v", m["loginCredential"])
	}
	if m["loginPassword"] != "" {
		t.Fatalf("central password must NOT be mixed in: %v", m["loginPassword"])
	}
}

func TestResolveForLaunch_NotFound(t *testing.T) {
	_, err := ResolveForLaunch(t.TempDir(), "missing", Credentials{}, t.TempDir())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestList_And_Delete(t *testing.T) {
	dir := t.TempDir()
	writeRawFile(t, dir, "b-conf", map[string]any{"comment": "B", "startWorlds": []any{map[string]any{}, map[string]any{}}})
	writeRawFile(t, dir, "a-conf", map[string]any{"comment": "A", "startWorlds": []any{map[string]any{}}})

	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "a-conf" || list[1].Name != "b-conf" {
		t.Fatalf("list not sorted/complete: %+v", list)
	}
	if list[0].Comment != "A" || list[0].WorldCount != 1 {
		t.Fatalf("summary wrong: %+v", list[0])
	}
	if list[1].WorldCount != 2 {
		t.Fatalf("worldCount wrong: %+v", list[1])
	}

	if err := Delete(dir, "a-conf"); err != nil {
		t.Fatal(err)
	}
	if _, err := readRaw(dir, "a-conf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted, got %v", err)
	}
	if err := Delete(dir, "a-conf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing should be ErrNotFound, got %v", err)
	}
}

func TestList_MissingDir(t *testing.T) {
	list, err := List(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty, got %+v", list)
	}
}

func TestEnsureDefault(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "configs")
	// 空 → default.json を作る
	if err := EnsureDefault(dir, tmp); err != nil {
		t.Fatal(err)
	}
	raw, err := readRaw(dir, "default")
	if err != nil {
		t.Fatalf("default not created: %v", err)
	}
	// default.json には説明文が注入されること（テンプレ側は空・EnsureDefault が jsonString で注入）。
	if raw["comment"] != defaultConfigComment {
		t.Fatalf("comment not injected: %v", raw["comment"])
	}
	// dataFolder/cacheFolder は {dataDir}/headless-data 等の絶対パスが焼き込まれること（UI改善⑤）。
	wantData, wantCache, err := DefaultFolders(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if raw["dataFolder"] != wantData {
		t.Fatalf("dataFolder = %v, want %v", raw["dataFolder"], wantData)
	}
	if raw["cacheFolder"] != wantCache {
		t.Fatalf("cacheFolder = %v, want %v", raw["cacheFolder"], wantCache)
	}
	// logsFolder は対象外＝null のまま。
	if v, ok := raw["logsFolder"]; !ok || v != nil {
		t.Fatalf("logsFolder should stay null, got %v (present=%v)", v, ok)
	}
	sw, ok := raw["startWorlds"].([]any)
	if !ok || len(sw) != 1 {
		t.Fatalf("default startWorlds wrong: %v", raw["startWorlds"])
	}
	w0 := sw[0].(map[string]any)
	if w0["accessLevel"] != "Anyone" {
		t.Fatalf("default accessLevel should be Anyone, got %v", w0["accessLevel"])
	}
	// 決定値が雛形に明示されていること（表示と保存値の一致を担保）。
	if w0["awayKickMinutes"] != 5.0 {
		t.Fatalf("default awayKickMinutes should be 5, got %v", w0["awayKickMinutes"])
	}
	if w0["autoSleep"] != true {
		t.Fatalf("default autoSleep should be true, got %v", w0["autoSleep"])
	}
	if w0["idleRestartInterval"] != 1800.0 {
		t.Fatalf("default idleRestartInterval should be 1800, got %v", w0["idleRestartInterval"])
	}
	// ニッチ化に伴い雛形から外したワールド項目は default に存在しないこと（headless 既定へ委譲・スリム化）。
	for _, k := range []string{"autoRecover", "mobileFriendly", "forcePort", "useCustomJoinVerifier"} {
		if _, ok := w0[k]; ok {
			t.Fatalf("%s should NOT be in default world template (slimmed to niche): %v", k, w0[k])
		}
	}
	// トップレベルのニッチ項目（universeId）も雛形に無いこと。
	if _, ok := raw["universeId"]; ok {
		t.Fatalf("universeId should NOT be in default config template (slimmed to niche): %v", raw["universeId"])
	}

	// 既に config がある → 何もしない（default を再作成しない）
	_ = Delete(dir, "default")
	writeRawFile(t, dir, "existing", map[string]any{"comment": "x"})
	if err := EnsureDefault(dir, tmp); err != nil {
		t.Fatal(err)
	}
	if _, err := readRaw(dir, "default"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("default should not be created when a config exists")
	}
}

// TestDefaultFolders は相対 dataDir でも絶対パスを返すこと（相対 dataFolder は headless 即クラッシュ）を検証する。
func TestDefaultFolders(t *testing.T) {
	dataFolder, cacheFolder, err := DefaultFolders("rel-data")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(dataFolder) || !filepath.IsAbs(cacheFolder) {
		t.Fatalf("folders must be absolute: %q %q", dataFolder, cacheFolder)
	}
	if filepath.Base(dataFolder) != "headless-data" || filepath.Base(cacheFolder) != "headless-cache" {
		t.Fatalf("folder names wrong: %q %q", dataFolder, cacheFolder)
	}
}

// TestEnsureFolders は起動前のフォルダ作成（絶対のみ・相対/未設定はスキップ）を検証する。
func TestEnsureFolders(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "configs")
	absData := filepath.Join(tmp, "hl-data")
	writeRawFile(t, dir, "w", map[string]any{
		"dataFolder":  absData,
		"cacheFolder": "relative-cache", // 相対 → 作らない（cwd 汚染防止）
		// logsFolder/未設定キーは対象外
	})
	if err := EnsureFolders(dir, "w"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(absData); err != nil || !fi.IsDir() {
		t.Fatalf("absolute dataFolder should be created: %v", err)
	}
	if _, err := os.Stat("relative-cache"); !os.IsNotExist(err) {
		t.Fatalf("relative cacheFolder should NOT be created")
	}
	// 既存でも no-op（エラーにならない）
	if err := EnsureFolders(dir, "w"); err != nil {
		t.Fatal(err)
	}
	// config 不在は ErrNotFound
	if err := EnsureFolders(dir, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing config should return ErrNotFound, got %v", err)
	}
	// 作成失敗（パス位置に既存ファイル）→ ErrFolderCreate（HTTP 層が 409 にマップする契約）
	blocked := filepath.Join(tmp, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeRawFile(t, dir, "bad", map[string]any{"dataFolder": filepath.Join(blocked, "sub")})
	if err := EnsureFolders(dir, "bad"); !errors.Is(err, ErrFolderCreate) {
		t.Fatalf("mkdir failure should return ErrFolderCreate, got %v", err)
	}
}

// TestCreate は即時作成（テンプレ・サーバー採番・comment 空・既定フォルダ焼き込み）を検証する。
func TestCreate(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "configs")

	name, err := Create(dir, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if name != "new-config" {
		t.Fatalf("first create should be new-config, got %q", name)
	}
	raw, err := readRaw(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	// default.json 用の説明文は引き継がない（UI 新規作成の従来挙動＝フロント defaultConfig() と一致）。
	if raw["comment"] != "" {
		t.Fatalf("comment should be blank, got %v", raw["comment"])
	}
	// dataFolder/cacheFolder は EnsureDefault と同じ焼き込み済み既定値。
	wantData, wantCache, err := DefaultFolders(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if raw["dataFolder"] != wantData || raw["cacheFolder"] != wantCache {
		t.Fatalf("folders not baked: %v / %v", raw["dataFolder"], raw["cacheFolder"])
	}
	// 2回目以降は new-config2, new-config3, … と採番される。
	if n2, _ := Create(dir, tmp); n2 != "new-config2" {
		t.Fatalf("second create should be new-config2, got %q", n2)
	}
	if n3, _ := Create(dir, tmp); n3 != "new-config3" {
		t.Fatalf("third create should be new-config3, got %q", n3)
	}
}

// TestDuplicate はサーバー側バイトコピー（password も写る・-copy 採番・不在 404）を検証する。
func TestDuplicate(t *testing.T) {
	dir := t.TempDir()
	writeRawFile(t, dir, "src", map[string]any{
		"loginCredential": "u@e.com",
		"loginPassword":   "secret", // フロント経由（GET マスク→PUT）では失われていた値
		"comment":         "c",
	})

	name, err := Duplicate(dir, "src")
	if err != nil {
		t.Fatal(err)
	}
	if name != "src-copy" {
		t.Fatalf("first duplicate should be src-copy, got %q", name)
	}
	raw, err := readRaw(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if raw["loginPassword"] != "secret" {
		t.Fatalf("password must be copied as-is, got %v", raw["loginPassword"])
	}
	if raw["comment"] != "c" {
		t.Fatalf("comment lost: %v", raw["comment"])
	}
	// 2回目は src-copy2。コピーのコピーは src-copy-copy。
	if n2, _ := Duplicate(dir, "src"); n2 != "src-copy2" {
		t.Fatalf("second duplicate should be src-copy2, got %q", n2)
	}
	if nc, _ := Duplicate(dir, "src-copy"); nc != "src-copy-copy" {
		t.Fatalf("duplicate of copy should be src-copy-copy, got %q", nc)
	}
	// 不在は ErrNotFound。
	if _, err := Duplicate(dir, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing source should return ErrNotFound, got %v", err)
	}
	// 64字（上限）の名前でも -copy 付与後に 64 字へ切り詰められ有効名で作られる。
	long := strings.Repeat("x", 64)
	writeRawFile(t, dir, long, map[string]any{"comment": "L"})
	ln, err := Duplicate(dir, long)
	if err != nil {
		t.Fatal(err)
	}
	if len(ln) > 64 || SanitizeName(ln) != nil {
		t.Fatalf("truncated name invalid: %q (len=%d)", ln, len(ln))
	}
}

// TestWriteRenamed は保存リネーム（旧削除・マスク解決は旧名側・不在 404）を検証する。
func TestWriteRenamed(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "old", map[string]any{"loginPassword": "secret", "comment": "v1"}); err != nil {
		t.Fatal(err)
	}

	// マスクされた password（空）は旧名側から解決され、旧ファイルは消える。
	if err := WriteRenamed(dir, "old", "new", map[string]any{"loginPassword": "", "comment": "v2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := readRaw(dir, "old"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old config should be removed after rename")
	}
	raw, err := readRaw(dir, "new")
	if err != nil {
		t.Fatal(err)
	}
	if raw["loginPassword"] != "secret" {
		t.Fatalf("password should be resolved from old name, got %v", raw["loginPassword"])
	}
	if raw["comment"] != "v2" {
		t.Fatalf("body should be the edited content, got %v", raw["comment"])
	}

	// 既存名へのリネーム＝上書き（UI 側で確認モーダルを挟む契約）。
	if err := Write(dir, "other", map[string]any{"comment": "target"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteRenamed(dir, "new", "other", map[string]any{"comment": "v3"}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := readRaw(dir, "other"); raw["comment"] != "v3" {
		t.Fatalf("rename onto existing should overwrite, got %v", raw["comment"])
	}
	if _, err := readRaw(dir, "new"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("renamed-from config should be removed")
	}

	// 旧名不在は ErrNotFound（書き込みもしない）。
	if err := WriteRenamed(dir, "missing", "x", map[string]any{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing oldName should return ErrNotFound, got %v", err)
	}
	if _, err := readRaw(dir, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nothing should be written when oldName is missing")
	}

	// 同名は通常保存と同じ（削除しない）。
	if err := WriteRenamed(dir, "other", "other", map[string]any{"comment": "v4"}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := readRaw(dir, "other"); raw["comment"] != "v4" {
		t.Fatalf("same-name rename should behave as normal save, got %v", raw["comment"])
	}
}
