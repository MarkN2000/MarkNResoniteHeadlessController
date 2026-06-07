// Command mrhc is the MarkN Resonite Headless Controller (v2).
// On first run (no config) it launches an interactive CLI setup wizard;
// otherwise it loads the config and starts the HTTP server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
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
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/selfupdate"
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

	// サブコマンド: update （自己更新・docs/design/self-update.md）。
	// ウィザード分岐より前に置き、config 不要で動かす（MRHC が起動できないほど
	// 壊れた環境での復旧経路を兼ねるため。新規環境でウィザードが起動しても困る）。
	if flag.Arg(0) == "update" {
		runUpdate(cfgPath, osLang)
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

	// 前回起動以降に自己更新が適用されていれば、残骸（退避された旧バイナリ等）を掃除して
	// その旨をログする（版が変わった痕跡は .old の存在だけなので、ここで一度だけ可視化する）。
	if selfupdate.CleanupStale() {
		log.Print(i18n.T(lang, "main.updated", version))
	}

	// 依存チェック（R-C 経路②）: 不足があればログで案内するだけで続行する
	// （[Y/n] 対話は初回ウィザード末尾のみ＝起動をブロックしない）。Windows は常に no-op。
	// .NET ランタイムはここでは見ない（DL 後フック＋起動時ガードの自動設置が担う）。
	if !fromWizard {
		for _, issue := range platform.CheckHeadlessDeps(runtime.GOOS, runtime.GOARCH) {
			log.Print(i18n.T(lang, "deps.missingLog", issue.Title(lang), issue.GuideText(lang)))
		}
	}

	// headless config ディレクトリを確保し、空なら同梱デフォルトを用意する
	// （dataFolder/cacheFolder には {dataDir}/headless-data 等の絶対パスを焼き込む）。
	configDir := cfg.HeadlessConfigDirOrDefault(dir)
	if err := hlconfig.EnsureDefault(configDir, dir); err != nil {
		log.Print(i18n.T(lang, "main.defaultConfigFailed", err))
	}

	driver := headless.NewDriver(platform.ConsoleEncoding(cfg.Encoding))
	srv := server.New(cfg, cfgPath, driver, resonite.NewClient(), web.FS())

	// 自己更新（Web UI 経路）と終了依頼の注入（Listen 前に設定する・docs/design/self-update.md）。
	if u, err := newUpdater(); err == nil {
		srv.SetUpdater(u)
	} else {
		log.Print(i18n.T(lang, "main.update.failed", err)) // update API は 503 になるが起動は続行
	}
	shutdownReq := make(chan struct{}, 1)
	srv.SetShutdownRequest(func() {
		select {
		case shutdownReq <- struct{}{}:
		default: // 既に依頼済み
		}
	})

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

	// SIGINT/SIGTERM または Web UI の終了依頼（自己更新後の「今すぐ終了」）で graceful 終了。
	// 稼働中のヘッドレスを shutdown して orphan（管理不能な残存プロセス）を防ぐ。
	// もう一度シグナルが来たら即終了。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sigCh:
		fmt.Println()
		fmt.Println(i18n.T(lang, "main.shutdown.received"))
	case <-shutdownReq:
		fmt.Println(i18n.T(lang, "main.shutdown.requestedWeb"))
		// select 以前に届いていたシグナルが buffer に残っていると、下の force-quit goroutine が
		// それを「2発目」として即終了に使ってしまう。1発目は graceful 扱い＝ここで読み捨てる
		//（旧コードの「最初のシグナルは必ず graceful」を web 終了経路でも保つ）。
		select {
		case <-sigCh:
		default:
		}
	}

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

// newUpdater は自己更新の実行器を返す（CLI の update サブコマンドと Web UI 経路で共用）。
// 検証・ローカルテスト用の取得元差し替え MRHC_UPDATE_BASE はここで注入する
// （install.sh の MRHC_DOWNLOAD_BASE と同じ流儀。selfupdate 内では環境変数を読まない）。
func newUpdater() (*selfupdate.Updater, error) {
	u, err := selfupdate.New(version)
	if err != nil {
		return nil, err
	}
	if base := os.Getenv("MRHC_UPDATE_BASE"); base != "" {
		u.BaseURL = base
	}
	return u, nil
}

// runUpdate は `mrhc update` サブコマンド本体。自分自身を最新リリースへ入れ替える。
// 表示言語は config があればその言語・無ければ OS 検出言語（config は必須にしない）。
// 適用後の有効化は次回起動時（実行中の MRHC があってもファイル入れ替えは安全・無影響）。
func runUpdate(cfgPath string, osLang i18n.Lang) {
	lang := osLang
	if config.FileExists(cfgPath) {
		if cfg, err := config.LoadFrom(cfgPath); err == nil {
			lang = cfg.LangOrDefault()
		}
	}
	u, err := newUpdater()
	if err != nil {
		log.Fatal(i18n.T(lang, "main.update.failed", err))
	}
	// Ctrl+C で中断可能にする（既存バイナリに触るのは全検証後の一瞬だけなので、
	// どの時点で中断しても現状維持のまま）。
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Println(i18n.T(lang, "main.update.checking"))
	info, err := u.Check(ctx)
	if err != nil {
		fatalUpdateErr(lang, err)
	}
	if !info.CurrentIsRelease {
		log.Fatal(i18n.T(lang, "main.update.devBuild", info.Current))
	}
	if !info.UpdateAvailable {
		fmt.Println(i18n.T(lang, "main.update.upToDate", info.Current))
		return
	}
	fmt.Println(i18n.T(lang, "main.update.downloading", info.Current, info.Latest))
	staged, err := u.Apply(ctx)
	if err != nil {
		fatalUpdateErr(lang, err)
	}
	fmt.Println(i18n.T(lang, "main.update.done", staged))
}

// fatalUpdateErr は selfupdate の sentinel を利用者向け文言に変換して終了する。
func fatalUpdateErr(lang i18n.Lang, err error) {
	switch {
	case errors.Is(err, selfupdate.ErrNoRelease):
		log.Fatal(i18n.T(lang, "main.update.noRelease", selfupdate.ReleasesURL))
	case errors.Is(err, selfupdate.ErrBusy):
		log.Fatal(i18n.T(lang, "main.update.busy"))
	case errors.Is(err, selfupdate.ErrUpToDate):
		// Check と Apply の間に latest が動いた稀なケース。更新不要＝正常終了。
		fmt.Println(i18n.T(lang, "main.update.upToDate", version))
		os.Exit(0)
	case errors.Is(err, fs.ErrPermission):
		log.Fatal(i18n.T(lang, "main.update.permission", err))
	default:
		log.Fatal(i18n.T(lang, "main.update.failed", err))
	}
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
