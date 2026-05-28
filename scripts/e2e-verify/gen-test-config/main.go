// gen-test-config は Phase 6 e2e 検証用の mrhc.config.json を生成する小道具。
// usage:
//   gen-test-config <password> <resonitePath> <outpath>
//
// 出力: outpath に 0600 で書き込む。APIキー/SessionSecret はランダム生成し
// 標準出力に key=value で印字する（呼び出し元スクリプトが拾う）。
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

type cfg struct {
	AdminPasswordHash string `json:"adminPasswordHash"`
	APIKey            string `json:"apiKey"`
	SessionSecret     string `json:"sessionSecret"`
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
	apiKey := randomB64(32)
	secret := randomB64(32)

	c := cfg{
		AdminPasswordHash: string(hash),
		APIKey:            apiKey,
		SessionSecret:     secret,
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
	fmt.Printf("APIKEY=%s\n", apiKey)
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
