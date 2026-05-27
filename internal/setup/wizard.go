// Package setup は初回起動時の対話型CLIセットアップウィザードを提供する。
// Windows / Linux 共通（同一バイナリ）。tty では入力を伏せ字にし、
// 非tty（パイプ等）では平文行読みにフォールバックする。
package setup

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

// RunWizard は最小限の設定（管理パスワード・ポート）を対話で受け取り、
// APIキー・セッション秘密を自動生成して cfgPath に保存する。
// 残りの設定（Resoniteパス・セッション定義など）はログイン後のWeb UIで行う。
func RunWizard(cfgPath string) error {
	in := bufio.NewReader(os.Stdin)
	tty := term.IsTerminal(int(os.Stdin.Fd()))

	fmt.Println("=== MRHC 初回セットアップ ===")
	fmt.Println("管理パスワードを設定します（Web UIへのログインに使用）。")

	pw, err := readPasswordTwice(in, tty)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	port := promptPort(in, 8080)
	headlessPath := promptHeadlessPath(in)

	apiKey, err := config.RandomSecret(24)
	if err != nil {
		return err
	}
	secret, err := config.RandomSecret(32)
	if err != nil {
		return err
	}

	cfg := &config.Config{
		AdminPasswordHash: string(hash),
		APIKey:            apiKey,
		SessionSecret:     secret,
		Port:              port,
		ResoniteHeadless:  headlessPath,
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		return err
	}

	fmt.Printf("\n設定を保存しました: %s\n", cfgPath)
	fmt.Printf("APIキー（スクリプト/ワールド内操作用・後でUIから再生成可）:\n  %s\n", apiKey)
	return nil
}

func readPasswordTwice(in *bufio.Reader, tty bool) (string, error) {
	for {
		p1, err := readSecret(in, tty, "管理パスワード: ")
		if err != nil {
			return "", err
		}
		if p1 == "" {
			fmt.Println("空のパスワードは設定できません。")
			continue
		}
		p2, err := readSecret(in, tty, "管理パスワード（確認）: ")
		if err != nil {
			return "", err
		}
		if p1 != p2 {
			fmt.Println("一致しません。もう一度入力してください。")
			continue
		}
		return p1, nil
	}
}

func readSecret(in *bufio.Reader, tty bool, prompt string) (string, error) {
	fmt.Print(prompt)
	if tty {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		return strings.TrimSpace(string(b)), err
	}
	line, err := in.ReadString('\n')
	return strings.TrimSpace(line), err
}

func promptPort(in *bufio.Reader, def int) int {
	fmt.Printf("HTTPポート [%d]: ", def)
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	if p, err := strconv.Atoi(line); err == nil && p > 0 && p < 65536 {
		return p
	}
	fmt.Printf("無効な入力のため %d を使用します。\n", def)
	return def
}

// promptHeadlessPath はResoniteヘッドレスのパスを任意で受け取る（空でスキップ可。
// 後でWeb UI/設定からも変更できる）。
func promptHeadlessPath(in *bufio.Reader) string {
	fmt.Println("\nResoniteヘッドレスのパス（任意・空でスキップ、後で設定可）")
	fmt.Println("  Windows例: C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe")
	fmt.Println("  Linux例:   ~/.local/share/Steam/steamapps/common/Resonite/Headless/Resonite.dll")
	fmt.Print("パス: ")
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}
