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
	dir := filepath.Join(t.TempDir(), "configs")
	// 空 → default.json を作る
	if err := EnsureDefault(dir); err != nil {
		t.Fatal(err)
	}
	raw, err := readRaw(dir, "default")
	if err != nil {
		t.Fatalf("default not created: %v", err)
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
	if w0["autoSleep"] != true || w0["autoRecover"] != true {
		t.Fatalf("default autoSleep/autoRecover should be true, got %v / %v", w0["autoSleep"], w0["autoRecover"])
	}
	if w0["idleRestartInterval"] != 1800.0 {
		t.Fatalf("default idleRestartInterval should be 1800, got %v", w0["idleRestartInterval"])
	}

	// 既に config がある → 何もしない（default を再作成しない）
	_ = Delete(dir, "default")
	writeRawFile(t, dir, "existing", map[string]any{"comment": "x"})
	if err := EnsureDefault(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := readRaw(dir, "default"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("default should not be created when a config exists")
	}
}
