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
// セッション秘密を自動生成して cfgPath に保存する。
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

	secret, err := config.RandomSecret(32)
	if err != nil {
		return err
	}

	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     secret,
		SessionTTLHours:   config.DefaultSessionTTLHours,
		Port:              port,
		ResoniteHeadless:  headlessPath,
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		return err
	}

	fmt.Printf("\n設定を保存しました: %s\n", cfgPath)
	return nil
}

// ResetPassword は既存設定の管理パスワードを再設定する（旧パスワード不要）。
// 実機のコマンドラインから `mrhc reset-password` で呼ばれる想定。
// 物理/SSHアクセス＝認可とみなし、パスワード忘れ時の復旧手段とする。
// adminPasswordHash が変わるため、署名鍵が変わり既存の全セッションが自動的に無効化される。
func ResetPassword(cfgPath string) error {
	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		return fmt.Errorf("設定の読み込みに失敗: %w", err)
	}
	in := bufio.NewReader(os.Stdin)
	tty := term.IsTerminal(int(os.Stdin.Fd()))

	fmt.Println("=== MRHC パスワード再設定 ===")
	fmt.Println("新しい管理パスワードを設定します。")

	pw, err := readPasswordTwice(in, tty)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	cfg.AdminPasswordHash = string(hash)
	if cfg.SessionSecret == "" { // 旧config救済: 署名に必須なので無ければ生成
		if s, e := config.RandomSecret(32); e == nil {
			cfg.SessionSecret = s
		}
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		return err
	}
	fmt.Println("\nパスワードを再設定しました。既存のログインセッションは全て無効になりました。")
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
