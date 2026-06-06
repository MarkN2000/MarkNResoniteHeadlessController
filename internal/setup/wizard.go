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
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/steam"
)

// ErrAborted は入力が読み取れず（EOF 等）ウィザードを中断したことを表す。
// 中断メッセージはウィザード自身が表示済みなので、呼び出し側は終了コードだけ扱えばよい。
var ErrAborted = errors.New("setup aborted")

// errInput は stdin の読取エラー（EOF 含む）を示す内部マーカー。
// Run が「中断（abort=EOF メッセージ）」と「実エラー（SaveTo 失敗等→main が原因を表示）」を
// 区別するために、読取系ヘルパはエラーを必ずこれで包む。
var errInput = errors.New("input unavailable")

// errDLCancelled は S5b の DL がキャンセル（Ctrl+C / shutdown）で終わったことを示す内部マーカー。
// 「失敗」と区別して専用の中止文言を出すために使う。
var errDLCancelled = errors.New("download cancelled")

// Wizard は初回セットアップの対話。フィールドはテストからの注入点（NewWizard が実環境を結線）。
type Wizard struct {
	In         io.Reader
	Out        io.Writer
	TTY        bool                       // tty ならパスワードを伏せ字入力
	DetectLang func() string              // S0 言語選択の既定値の提案（"ja"/"en"）
	PortInUse  func(port int) bool        // S3 ポートの空き事前試験（true=使用中）
	CheckDeps  func() []platform.DepIssue // S4 依存検出（nil 返却=不足なし）
	// SteamUpdate は S5b の DL 実行（実環境= steam.Manager.Update への結線）。
	// イベントは onEvent へ転送される（進捗%の端末描画用）。
	SteamUpdate func(ctx context.Context, toolsDir string, p steam.UpdateParams, onEvent func(steam.Event)) error
}

// NewWizard は実環境（stdin/stdout・OS 言語検出・実依存チェック・実 Steam DL）の Wizard を返す。
func NewWizard() *Wizard {
	return &Wizard{
		In:         os.Stdin,
		Out:        os.Stdout,
		TTY:        term.IsTerminal(int(os.Stdin.Fd())),
		DetectLang: platform.DetectLang,
		PortInUse:  portInUse,
		CheckDeps: func() []platform.DepIssue {
			return platform.CheckHeadlessDeps(runtime.GOOS, runtime.GOARCH)
		},
		SteamUpdate: realSteamUpdate,
	}
}

// portInUse は S3 の空き事前試験。Listen が成功すれば空き、失敗のうち「使用中」
// （IsAddrInUse）だけを true とする。権限不足など他の失敗は S3 では黙って通し、
// 起動時の listenFailed 文言に任せる（チェックはあくまで早期警告のベストエフォート。
// 3GB の DL を完走した後に「ポート使用中」で死ぬ事故を入力時点で防ぐのが目的）。
func portInUse(port int) bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err == nil {
		_ = ln.Close()
		return false
	}
	return platform.IsAddrInUse(err)
}

// realSteamUpdate は steam.Manager で実 DL を行い、進捗イベントを onEvent へ転送する。
// サーバー未起動のウィザード段階なので Web UI（single-flight 共有）との衝突は構造的に無い。
// 成否判定は Update の戻り値で行う（result イベントではない・pubsub は満杯時ドロップのため）。
func realSteamUpdate(ctx context.Context, toolsDir string, p steam.UpdateParams, onEvent func(steam.Event)) error {
	m := steam.NewManager(toolsDir)
	ch, _ := m.Subscribe(64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range ch {
			onEvent(e)
		}
	}()
	err := m.Update(ctx, p)
	m.Unsubscribe(ch) // close され転送 goroutine が抜ける
	<-done
	return err
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
		return nil, false, w.abortOr(lang, err)
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
		return nil, false, w.abortOr(lang, err)
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
		return nil, false, w.abortOr(lang, err)
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
	w.offerDeps(in, lang)

	// S5: Resonite セットアップ（任意・Steam 資格入力→DL 実行）
	if err := w.offerResonite(cfgPath, cfg, in, lang); err != nil {
		return nil, false, w.abortOr(lang, err) // 読取エラー=中断 / SaveTo 失敗等=実エラーのまま
	}

	// S6: 起動確認
	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, i18n.T(lang, "wizard.saved", cfgPath))
	fmt.Fprintln(w.Out)
	start, err := promptYN(in, w.Out, lang, i18n.T(lang, "wizard.start.prompt"))
	if err != nil {
		return nil, false, w.abortOr(lang, err)
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

// readLine は 1 行読み取って前後空白を落とす。EOF 等で行が得られなければ errInput を返す。
// 「改行なしの最終行 + EOF」は有効な回答として扱う（次の読みで EOF が返る）。
func readLine(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	s := strings.TrimSpace(line)
	if err != nil && s == "" {
		return "", fmt.Errorf("%w: %v", errInput, err)
	}
	return s, nil
}

// abortOr は読取エラー（errInput）なら中断メッセージ付きの ErrAborted に、
// それ以外（SaveTo 失敗等）はそのまま返す（M1: 実エラーを EOF と誤報告しない）。
func (w *Wizard) abortOr(lang i18n.Lang, err error) error {
	if errors.Is(err, errInput) {
		return w.abort(lang)
	}
	return err
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
		// 注意: tty では bufio をバイパスして端末から直読みする（伏せ字のため）。
		// 先読みバッファ内のバイトは ReadPassword に届かない（対話入力では実害なし）。
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(w.Out)
		if err != nil {
			return "", fmt.Errorf("%w: %v", errInput, err)
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
		p := def
		if s != "" {
			v, convErr := strconv.Atoi(s)
			if convErr != nil || v <= 0 || v >= 65536 {
				fmt.Fprintln(w.Out, i18n.T(lang, "wizard.port.invalid"))
				continue
			}
			p = v
		}
		// 使用中なら警告して確認する。ここだけ既定 N（空 Enter=ポート再入力に戻る）＝
		// 使用中と分かっているポートをうっかり確定しない安全側。
		if w.PortInUse != nil && w.PortInUse(p) {
			useAnyway, err := promptYNDef(in, w.Out, lang, i18n.T(lang, "wizard.port.inUse", p), false)
			if err != nil {
				return 0, err
			}
			if !useAnyway {
				continue
			}
		}
		return p, nil
	}
}

// promptYN は [Y/n] を尋ねる（空 Enter = Y）。不正は再入力・EOF はエラー。
// ウィザード各所（S5/S6）と OfferDepInstall（S4）で共用する。
func promptYN(in *bufio.Reader, out io.Writer, lang i18n.Lang, prompt string) (bool, error) {
	return promptYNDef(in, out, lang, prompt, true)
}

// promptYNDef は空 Enter の既定値を指定できる y/n プロンプト。既定 N は S3 の
// 「使用中ポートをこのまま使うか」だけ（全プロンプト既定 Y の原則の唯一の例外＝安全側）。
func promptYNDef(in *bufio.Reader, out io.Writer, lang i18n.Lang, prompt string, defYes bool) (bool, error) {
	for {
		fmt.Fprint(out, prompt)
		s, err := readLine(in)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(s) {
		case "":
			return defYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, i18n.T(lang, "common.yn.invalid"))
		}
	}
}

// offerDeps はウィザード保存成功後の依存チェック＋導入提案（R-C 経路①・freetype2 のみ）。
func (w *Wizard) offerDeps(in *bufio.Reader, lang i18n.Lang) {
	OfferDepInstall(w.CheckDeps(), in, w.Out, lang, w.TTY, nil, func(kind string) bool {
		for _, i := range w.CheckDeps() {
			if i.Kind == kind {
				return false // まだ不足のまま
			}
		}
		return true
	})
}

// offerResonite は S5: Resonite 本体のセットアップ（任意）。
// Y なら Steam 資格を読み（S5a）、config へ保存して DL を実行する（S5b）。
// 戻り値の error は読取エラー（errInput）と SaveTo 失敗のみ（DL の失敗・中止は
// メッセージを出して nil ＝続行。あとから Web UI で再試行できるため、ウィザードはブロックしない）。
func (w *Wizard) offerResonite(cfgPath string, cfg *config.Config, in *bufio.Reader, lang i18n.Lang) error {
	dataDir := filepath.Dir(cfgPath)

	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, i18n.T(lang, "wizard.resonite.header"))
	yes, err := promptYN(in, w.Out, lang, i18n.T(lang, "wizard.resonite.prompt"))
	if err != nil {
		return err
	}
	if !yes {
		return nil // ヘッダで「あとから Web UI で実行できます」と案内済み
	}

	for { // 認証失敗時は資格入力からやり直せる（S5b → S5a のループ）
		creds, ok, err := w.readSteamCreds(in, lang, cfg.InstallDirOrDefault(dataDir))
		if err != nil {
			return err
		}
		if !ok {
			return nil // 空 Enter で中止（案内表示済み）
		}

		// 資格を config へ保存（DL の成否に関わらず Web UI から再利用できるように先に保存）。
		// インストール先の空 Enter は「表示した既定値」の維持＝認証失敗→再入力の 2 周目では
		// 前回の明示値を引き継ぐ（M2: 黙って既定導出に巻き戻さない）。
		installDir := creds.installDir
		if installDir == "" && cfg.Steam != nil {
			installDir = cfg.Steam.InstallDir
		}
		cfg.Steam = &config.Steam{
			Username:   creds.user,
			Password:   creds.pw,
			BranchCode: creds.code,
			InstallDir: installDir, // 空=既定導出（InstallDirOrDefault）
		}
		if err := cfg.SaveTo(cfgPath); err != nil {
			return err
		}

		err = w.runSteamUpdate(dataDir, cfg, lang)
		switch {
		case err == nil:
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.done"))
			return nil
		case errors.Is(err, errDLCancelled):
			// ユーザー自身の中止は「失敗」と区別する（DD は差分再開できる）
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.cancelled"))
			return nil
		case errors.Is(err, steam.ErrTwoFactorRequired):
			// Guard オンは再入力しても絶対に成功しないため、再入力ループに入れない（M3）
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.twoFactor"))
			return nil
		case errors.Is(err, steam.ErrAuthFailed):
			retry, err2 := promptYN(in, w.Out, lang, i18n.T(lang, "wizard.dl.authRetry"))
			if err2 != nil {
				return err2
			}
			if !retry {
				fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.retryLater"))
				return nil
			}
			continue
		case errors.Is(err, steam.ErrVerifyMissing):
			// ヘッドレス（branch）コード誤り＝DD は public へフォールバックして exit 0 でも
			// headless 実体が取れない（H2）。資格の入力ミスなので認証失敗と同じく再入力へ
			// 誘導する（DD は差分再開するため再実行コストは低い）。
			retry, err2 := promptYN(in, w.Out, lang, i18n.T(lang, "wizard.dl.verifyRetry"))
			if err2 != nil {
				return err2
			}
			if !retry {
				fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.retryLater"))
				return nil
			}
			continue
		case errors.Is(err, steam.ErrStalled):
			// 停滞打切りは資格ミスではない（回線・サーバー側要因）ので再入力には誘導しない。
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.stalled"))
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.retryLater"))
			return nil
		case errors.Is(err, steam.ErrDotnetInstallFailed):
			// Resonite 本体の DL は完了している（資格ミスではない）。.NET ランタイムは
			// ヘッドレス起動時のガードが自動で再試行するため、再入力には誘導しない。
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.dotnetFailed", err))
			return nil
		default:
			// ネットワーク断・Ctrl+C 中断・検証失敗など。DD は差分再開できるので
			// あとから Web UI の「今すぐ更新」でやり直せる。
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.failed", err))
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.retryLater"))
			return nil
		}
	}
}

// steamCreds は S5a で読み取る入力一式。installDir は空=既定導出。
type steamCreds struct {
	user, pw, code, installDir string
}

// readSteamCreds は S5a: Steam 資格の対話入力。ユーザー名・パスワード・コードの空 Enter は
// セクション中止（ok=false・案内を表示）。インストール先だけは空=既定（確定仕様）。
func (w *Wizard) readSteamCreds(in *bufio.Reader, lang i18n.Lang, defaultInstall string) (steamCreds, bool, error) {
	var c steamCreds

	cancel := func() (steamCreds, bool, error) {
		fmt.Fprintln(w.Out, i18n.T(lang, "wizard.steam.cancelled"))
		return c, false, nil
	}

	fmt.Fprint(w.Out, i18n.T(lang, "wizard.steam.user"))
	user, err := readLine(in)
	if err != nil {
		return c, false, err
	}
	if user == "" {
		return cancel()
	}
	c.user = user

	for { // パスワードは ASCII 64 文字以内（DepotDownloader の制約）を満たすまで再入力
		pw, err := w.readSecret(in, i18n.T(lang, "wizard.steam.pw"))
		if err != nil {
			return c, false, err
		}
		if pw == "" {
			return cancel()
		}
		if steam.ValidatePassword(pw) != nil {
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.steam.pwInvalid"))
			continue
		}
		c.pw = pw
		break
	}

	fmt.Fprint(w.Out, i18n.T(lang, "wizard.steam.code"))
	code, err := readLine(in)
	if err != nil {
		return c, false, err
	}
	if code == "" {
		return cancel()
	}
	c.code = code

	fmt.Fprint(w.Out, i18n.T(lang, "wizard.steam.installDir", defaultInstall))
	dir, err := readLine(in)
	if err != nil {
		return c, false, err
	}
	c.installDir = dir // 空=既定導出のまま（config に保存しない）
	return c, true, nil
}

// runSteamUpdate は S5b: DL を実行し進捗を表示する。
// Ctrl+C は DL 実行区間だけ捕捉して cancel する（プロンプト読取中に張ると Linux で
// Ctrl+C が「効いていない」ように見えるため・M2）。中断しても DD は差分再開できる。
func (w *Wizard) runSteamUpdate(dataDir string, cfg *config.Config, lang i18n.Lang) error {
	params, err := steam.BuildUpdateParams(
		cfg.Steam.Username, cfg.Steam.Password, cfg.Steam.BranchCode,
		platform.ExpandHome(cfg.InstallDirOrDefault(dataDir)), platform.HeadlessBinaryName())
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.start"))
	fmt.Fprint(w.Out, i18n.T(lang, "wizard.dl.preparing"))

	// 進捗表示: DD 本体（ダウンロードツール）の準備完了は最初の progress/milestone で
	// 判定する（acquire 中は log のみ）。% は 10% 刻みの行表示（tty/パイプ両対応・
	// \r の上書き描画はパイプのログを汚すため使わない）。
	preparing := true
	lastStep := -10
	onEvent := func(e steam.Event) {
		if e.Kind != "progress" && e.Kind != "milestone" {
			return
		}
		if preparing {
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.preparingDone"))
			preparing = false
		}
		// DD 100% の後に走る .NET ランタイム設置区間は % が 0 から再カウントするため、
		// 単調ガード（lastStep）のままだと完全に無言になる。区間ラベルを 1 行出して
		// lastStep をリセットし、設置 DL の進捗も 10% 刻みで見えるようにする。
		if e.Kind == "milestone" && e.Text == steam.MilestoneDotnetInstalling {
			fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.dotnetInstalling"))
			lastStep = -10
			return
		}
		if e.Kind == "progress" {
			step := int(e.Percent) / 10 * 10
			if step > lastStep {
				lastStep = step
				fmt.Fprintln(w.Out, i18n.T(lang, "wizard.dl.downloading", step))
			}
		}
	}
	err = w.SteamUpdate(ctx, filepath.Join(dataDir, "tools"), params, onEvent)
	if preparing {
		fmt.Fprintln(w.Out) // 1 度も進捗が来ずに失敗した場合の改行（"準備中..." の行を閉じる）
	}
	if err != nil && (errors.Is(err, steam.ErrCancelled) || ctx.Err() != nil) {
		// manager の中止 sentinel に加え、acquire 段階の中断（ctx 起因の生エラーで返る）も
		// ctx.Err() で拾って「中止」に正規化する。
		return errDLCancelled
	}
	return err
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

	fmt.Fprintln(w.Out, i18n.T(lang, "reset.header"))

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
	fmt.Fprintln(w.Out)
	fmt.Fprintln(w.Out, i18n.T(lang, "reset.done"))
	return nil
}
