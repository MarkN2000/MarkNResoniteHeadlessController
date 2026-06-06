// Package setup は初回起動時の対話型CLIセットアップウィザードを提供する。
// Windows / Linux 共通（同一バイナリ）。画面構成・文言は docs/design/cli-onboarding.md
// （確定仕様）に従う。
//
// 入力の契約（全プロンプト共通）:
//   - 不正な値 → 理由を表示して再入力（黙って既定値に倒さない）
//   - EOF・読取エラー → ErrAborted で中断（パイプ実行で回答が尽きたとき無限ループしない）
//   - 空 Enter → 各プロンプトの既定値（[Y/n] は Y）
package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

// ErrAborted は入力が読み取れず（EOF 等）ウィザードを中断したことを表す。
// 中断メッセージはウィザード自身が表示済みなので、呼び出し側は終了コードだけ扱えばよい。
var ErrAborted = errors.New("setup aborted")

// Wizard は初回セットアップの対話。フィールドはテストからの注入点（NewWizard が実環境を結線）。
type Wizard struct {
	In         io.Reader
	Out        io.Writer
	TTY        bool                                        // tty ならパスワードを伏せ字入力
	DetectLang func() string                               // S0 言語選択の既定値の提案（"ja"/"en"）
	CheckDeps  func(installDir string) []platform.DepIssue // S4 依存検出（nil 返却=不足なし）
}

// NewWizard は実環境（stdin/stdout・OS 言語検出・実依存チェック）の Wizard を返す。
func NewWizard() *Wizard {
	return &Wizard{
		In:         os.Stdin,
		Out:        os.Stdout,
		TTY:        term.IsTerminal(int(os.Stdin.Fd())),
		DetectLang: platform.DetectLang,
		CheckDeps: func(installDir string) []platform.DepIssue {
			return platform.CheckHeadlessDeps(runtime.GOOS, runtime.GOARCH, installDir)
		},
	}
}

// Run はウィザードを実行し、保存済み config と「今すぐサーバーを起動するか」を返す。
// 流れ: S0 言語 → S1 導入 → S2 管理PW → S3 ポート → config 保存 → S4 依存（Linuxのみ）
// → S6 起動確認。S3 直後に保存するため、以降で中断しても基本設定は残る
// （次回は通常起動になり、Resonite の準備は Web UI・依存導入は起動時ログで回収できる）。
func (w *Wizard) Run(cfgPath string) (cfg *config.Config, startNow bool, err error) {
	in := bufio.NewReader(w.In)

	// S0: 言語選択（確定前なので中断メッセージも既定言語で出す）
	lang := i18n.Ja
	if w.DetectLang() == "en" {
		lang = i18n.En
	}
	chosen, err := w.promptLang(in, lang)
	if err != nil {
		return nil, false, w.abort(lang)
	}
	lang = chosen

	// S1: 導入（これから決める 3 つ）
	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, i18n.T(lang, "wizard.intro"))

	// S2: 管理パスワード
	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, i18n.T(lang, "wizard.pw.header"))
	pw, err := w.readPasswordTwice(in, lang)
	if err != nil {
		return nil, false, w.abort(lang)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return nil, false, err
	}

	// S3: Web ポート
	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, i18n.T(lang, "wizard.port.header"))
	port, err := w.promptPort(in, lang, 8080)
	if err != nil {
		return nil, false, w.abort(lang)
	}

	secret, err := config.RandomSecret(32)
	if err != nil {
		return nil, false, err
	}
	cfg = &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     secret,
		SessionTTLHours:   config.DefaultSessionTTLHours,
		Port:              port,
		Language:          string(lang),
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		return nil, false, err
	}

	// S4: 依存チェック＋[Y/n] 導入提案（Linux のみ・不足時のみ表示）
	w.offerDeps(cfgPath, cfg, in)

	// S5（Resonite セットアップ）は後続コミットで追加。それまでは Web UI への案内のみ。
	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, "Resonite 本体はログイン後の Web UI（設定 → Steam）からダウンロードできます。")

	// S6: 起動確認
	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, i18n.T(lang, "wizard.saved", cfgPath))
	fmt.Fprintln(w.Out)
	start, err := w.promptYN(in, lang, i18n.T(lang, "wizard.start.prompt"))
	if err != nil {
		return nil, false, w.abort(lang)
	}
	if !start {
		exe := "./mrhc"
		if runtime.GOOS == "windows" {
			exe = "mrhc.exe"
		}
		fmt.Fprintln(w.Out, i18n.T(lang, "wizard.start.later", exe))
	}
	return cfg, start, nil
}

// abort は中断メッセージを表示して ErrAborted を返す（EOF・読取エラー時の共通処理）。
func (w *Wizard) abort(lang i18n.Lang) error {
	fmt.Fprintln(w.Out, i18n.T(lang, "wizard.aborted"))
	return ErrAborted
}

// readLine は 1 行読み取って前後空白を落とす。EOF 等で行が得られなければ error。
// 「改行なしの最終行 + EOF」は有効な回答として扱う（次の読みで EOF が返る）。
func readLine(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	s := strings.TrimSpace(line)
	if err != nil && s == "" {
		return "", err
	}
	return s, nil
}

// promptLang は S0 言語選択。言語確定前のため文言は両言語併記の固定文（カタログ外）。
func (w *Wizard) promptLang(in *bufio.Reader, def i18n.Lang) (i18n.Lang, error) {
	defNum := "2"
	if def == i18n.En {
		defNum = "1"
	}
	for {
		fmt.Fprintf(w.Out, "Language / 言語 [1=English 2=日本語] (%s): ", defNum)
		s, err := readLine(in)
		if err != nil {
			return def, err
		}
		switch s {
		case "":
			return def, nil
		case "1":
			return i18n.En, nil
		case "2":
			return i18n.Ja, nil
		default:
			fmt.Fprintln(w.Out, "Please enter 1 or 2. / 1 か 2 を入力してください。")
		}
	}
}

func (w *Wizard) readPasswordTwice(in *bufio.Reader, lang i18n.Lang) (string, error) {
	for {
		p1, err := w.readSecret(in, i18n.T(lang, "wizard.pw.prompt"))
		if err != nil {
			return "", err
		}
		if p1 == "" {
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.pw.empty"))
			continue
		}
		p2, err := w.readSecret(in, i18n.T(lang, "wizard.pw.confirm"))
		if err != nil {
			return "", err
		}
		if p1 != p2 {
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.pw.mismatch"))
			continue
		}
		return p1, nil
	}
}

// readSecret は秘密入力を 1 つ読む。tty では伏せ字（端末から直接）、
// 非 tty（パイプ）では行読みにフォールバックする。
func (w *Wizard) readSecret(in *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(w.Out, prompt)
	if w.TTY {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(w.Out)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return readLine(in)
}

func (w *Wizard) promptPort(in *bufio.Reader, lang i18n.Lang, def int) (int, error) {
	for {
		fmt.Fprint(w.Out, i18n.T(lang, "wizard.port.prompt", def))
		s, err := readLine(in)
		if err != nil {
			return 0, err
		}
		if s == "" {
			return def, nil
		}
		if p, convErr := strconv.Atoi(s); convErr == nil && p > 0 && p < 65536 {
			return p, nil
		}
		fmt.Fprintln(w.Out, i18n.T(lang, "wizard.port.invalid"))
	}
}

// promptYN は [Y/n] を尋ねる（空 Enter = Y）。不正は再入力・EOF はエラー。
func (w *Wizard) promptYN(in *bufio.Reader, lang i18n.Lang, prompt string) (bool, error) {
	for {
		fmt.Fprint(w.Out, prompt)
		s, err := readLine(in)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(s) {
		case "", "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(w.Out, i18n.T(lang, "common.yn.invalid"))
		}
	}
}

// offerDeps はウィザード保存成功後の依存チェック＋導入提案（R-C 経路①）。
// ウィザード生成 cfg は Steam=nil のため installDir は常に既定（{dataDir}/resonite）。
func (w *Wizard) offerDeps(cfgPath string, cfg *config.Config, in *bufio.Reader) {
	dataDir := filepath.Dir(cfgPath)
	installDir := cfg.InstallDirOrDefault(dataDir)
	check := func() []platform.DepIssue { return w.CheckDeps(installDir) }
	OfferDepInstall(check(), in, w.TTY, nil, func(kind string) bool {
		for _, i := range check() {
			if i.Kind == kind {
				return false // まだ不足のまま
			}
		}
		return true
	})
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
	w := NewWizard()
	in := bufio.NewReader(w.In)
	lang := cfg.LangOrDefault()

	fmt.Println("=== MRHC パスワード再設定 ===")
	fmt.Println("新しい管理パスワードを設定します。")

	pw, err := w.readPasswordTwice(in, lang)
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
