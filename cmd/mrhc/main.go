// Command mrhc is the MarkN Resonite Headless Controller (v2).
// On first run (no config) it launches an interactive CLI setup wizard;
// otherwise it loads the config and starts the HTTP server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/hlconfig"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/server"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/setup"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/web"
)

// version はリリースビルド時に -ldflags "-X main.version=<タグ名>" で焼き込まれる
// （release.yml）。未指定のローカルビルドは "dev" のまま。
var version = "dev"

func main() {
	// config が読めない場面（フラグ説明・起動前の fatal）は OS 検出言語で表示する
	// （日本語 OS 以外はすべて英語）。config が読めたら以降は config の言語。
	osLang := i18n.LangOf(platform.DetectLang())

	dataDir := flag.String("data", "", i18n.T(osLang, "main.flag.data"))
	showVersion := flag.Bool("version", false, i18n.T(osLang, "main.flag.version"))
	flag.Parse()

	if *showVersion {
		fmt.Println("mrhc " + version)
		return
	}

	dir := *dataDir
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatal(i18n.T(osLang, "main.exePathFailed", err))
		}
		dir = filepath.Dir(exe)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(i18n.T(osLang, "main.dataDirFailed", err))
	}
	cfgPath := filepath.Join(dir, config.FileName)

	// サブコマンド: reset-password （旧PW不要・実機での復旧手段）
	if flag.Arg(0) == "reset-password" {
		if !config.FileExists(cfgPath) {
			log.Fatal(i18n.T(osLang, "main.resetNoConfig", cfgPath))
		}
		if err := setup.ResetPassword(cfgPath); err != nil {
			log.Fatal(i18n.T(osLang, "main.resetFailed", err))
		}
		return
	}

	// 初回（config 無し）はウィザード。S6 で「今すぐ起動」を選んだらそのままサーバーへ
	// 合流する（2回起動の廃止・docs/design/cli-onboarding.md）。
	if !config.FileExists(cfgPath) {
		cfg, startNow, err := setup.NewWizard().Run(cfgPath)
		if err != nil {
			if errors.Is(err, setup.ErrAborted) {
				os.Exit(1) // 中断メッセージはウィザードが表示済み
			}
			log.Fatal(i18n.T(osLang, "main.setupFailed", err))
		}
		if !startNow {
			return
		}
		fmt.Println()
		runServer(cfg, cfgPath, dir, true)
		return
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		log.Fatal(i18n.T(osLang, "main.configLoadFailed", err))
	}
	runServer(cfg, cfgPath, dir, false)
}

// runServer はサーバー本体を起動し、SIGINT/SIGTERM で graceful 終了するまでブロックする。
// fromWizard=true はウィザード直後の継続起動: S4 で依存チェック済みのため経路②
// （毎起動の依存不足ログ）の重複実行をスキップし、バナーをログイン案内付きにする。
func runServer(cfg *config.Config, cfgPath, dir string, fromWizard bool) {
	lang := cfg.LangOrDefault()

	// 依存チェック（R-C 経路②）: 不足があればログで案内するだけで続行する
	// （[Y/n] 対話は初回ウィザード末尾のみ＝起動をブロックしない）。Windows は常に no-op。
	if !fromWizard {
		for _, issue := range platform.CheckHeadlessDeps(runtime.GOOS, runtime.GOARCH, cfg.InstallDirOrDefault(dir)) {
			log.Print(i18n.T(lang, "deps.missingLog", issue.Title(lang), issue.GuideText(lang)))
		}
	}

	// headless config ディレクトリを確保し、空なら同梱デフォルトを用意する。
	configDir := cfg.HeadlessConfigDirOrDefault(dir)
	if err := hlconfig.EnsureDefault(configDir); err != nil {
		log.Print(i18n.T(lang, "main.defaultConfigFailed", err))
	}

	driver := headless.NewDriver(platform.ConsoleEncoding(cfg.Encoding))
	srv := server.New(cfg, cfgPath, driver, resonite.NewClient(), web.FS())

	// ポートを同期で確保してからバナーを出す（bind 失敗を「起動しました」の後に出さない）。
	// Windows の「使用中」は WSAEADDRINUSE(10048) で syscall.EADDRINUSE と一致しないため、
	// 判定は platform.IsAddrInUse に集約している。
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.Port))
	if err != nil {
		if platform.IsAddrInUse(err) {
			log.Fatal(i18n.T(lang, "main.portInUse", cfg.Port))
		}
		log.Fatal(i18n.T(lang, "main.listenFailed", err))
	}

	// バックグラウンドワーカー（scheduler 等・Phase 8）を起動。stop で停止する。
	stopBackground := srv.Start()

	printBanner(lang, cfg.Port, fromWizard)

	httpServer := &http.Server{Handler: srv.Handler()}
	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatal(i18n.T(lang, "main.listenFailed", err))
		}
	}()

	// SIGINT/SIGTERM で graceful 終了。稼働中のヘッドレスを shutdown して
	// orphan（管理不能な残存プロセス）を防ぐ。もう一度シグナルが来たら即終了。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println()
	fmt.Println(i18n.T(lang, "main.shutdown.received"))

	// 先にバックグラウンドワーカーを止める（予定発火を打ち切り、進行中の再起動 ①②③ を cancel して
	// 下の driver.Stop() と競合しないようにする）。
	stopBackground()

	if driver.Status().State != headless.StateStopped {
		go func() {
			<-sigCh
			fmt.Println(i18n.T(lang, "main.shutdown.force"))
			os.Exit(1)
		}()
		_ = driver.Stop()
		// driver.Stop() の force-kill 猶予(180s)より少し長く待つ（先に MRHC が
		// 終了して子プロセスが残るのを防ぐ）。
		waitForStopped(driver, 185*time.Second)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	fmt.Println(i18n.T(lang, "main.shutdown.done"))
}

// printBanner は起動バナー（S7/S9）を表示する。LAN URL を主役にする
// （ブラウザで開くのは別 PC が前提）。IP は複数 NIC で混乱するため列挙せず固定文言。
// FW 案内は両 OS（Windows=初回ダイアログと誤キャンセル時の復旧 / Linux=開放コマンド例）。
func printBanner(lang i18n.Lang, port int, fromWizard bool) {
	fmt.Println(i18n.T(lang, "banner.running", version))
	if fromWizard {
		fmt.Println(i18n.T(lang, "banner.openLanLogin", port))
	} else {
		fmt.Println(i18n.T(lang, "banner.openLan", port))
	}
	fmt.Println(i18n.T(lang, "banner.localhost", port))
	if runtime.GOOS == "windows" {
		fmt.Println(i18n.T(lang, "banner.fw.windows"))
	} else {
		fmt.Println(i18n.T(lang, "banner.fw.linux", port, port, port))
	}
	fmt.Println(i18n.T(lang, "banner.stop"))
}

// waitForStopped はヘッドレスが停止状態になるまで（または timeout まで）待つ。
func waitForStopped(d *headless.Driver, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if d.Status().State == headless.StateStopped {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}
