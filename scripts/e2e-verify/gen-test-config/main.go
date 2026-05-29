// gen-test-config は Phase 6 e2e 検証用の mrhc.config.json を生成する小道具。
// usage:
//   gen-test-config <password> <resonitePath> <outpath>
//
// 出力: outpath に 0600 で書き込む。SessionSecret はランダム生成する。
// 認証は Bearer パスワード（呼び出し元が <password> を保持して使う）。
// 標準出力に CONFIG=<path> を印字する（呼び出し元スクリプトが拾う）。
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// cfg は internal/config.Config の e2e 用サブセット（SchemaVersion=1 / 既定TTL=720h=30日）。
type cfg struct {
	Version           int    `json:"version"`
	AdminPasswordHash string `json:"adminPasswordHash"`
	SessionSecret     string `json:"sessionSecret"`
	SessionTTLHours   int    `json:"sessionTtlHours,omitempty"`
	Port              int    `json:"port"`
	ResoniteHeadless  string `json:"resoniteHeadlessPath,omitempty"`
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: gen-test-config <password> <resonitePath> <outpath>")
		os.Exit(2)
	}
	pw := os.Args[1]
	resPath := os.Args[2]
	out := os.Args[3]

	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		fail("bcrypt: %v", err)
	}
	secret := randomB64(32)

	c := cfg{
		Version:           1,
		AdminPasswordHash: string(hash),
		SessionSecret:     secret,
		SessionTTLHours:   720,
		Port:              8080,
		ResoniteHeadless:  resPath,
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		fail("marshal: %v", err)
	}
	if err := os.WriteFile(out, b, 0o600); err != nil {
		fail("write %s: %v", out, err)
	}
	// 呼び出し元が拾うため標準出力に key=value 形式で印字
	fmt.Printf("CONFIG=%s\n", out)
}

func randomB64(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		fail("rand: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
