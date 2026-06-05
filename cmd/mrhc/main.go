// Command mrhc is the MarkN Resonite Headless Controller (v2).
// On first run (no config) it launches an interactive CLI setup wizard;
// otherwise it loads the config and starts the HTTP server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/server"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/setup"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/web"
)

func main() {
	dataDir := flag.String("data", "", "データディレクトリ（config/state置き場。既定: 実行ファイルと同じ場所）")
	flag.Parse()

	dir := *dataDir
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("実行ファイルパスの取得に失敗: %v", err)
		}
		dir = filepath.Dir(exe)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("データディレクトリの作成に失敗: %v", err)
	}
	cfgPath := filepath.Join(dir, config.FileName)

	// サブコマンド: reset-password （旧PW不要・実機での復旧手段）
	if flag.Arg(0) == "reset-password" {
		if !config.FileExists(cfgPath) {
			log.Fatalf("設定ファイルがありません: %s（先に通常起動して初回セットアップを完了してください）", cfgPath)
		}
		if err := setup.ResetPassword(cfgPath); err != nil {
			log.Fatalf("パスワード再設定に失敗: %v", err)
		}
		return
	}

	if !config.FileExists(cfgPath) {
		if err := setup.RunWizard(cfgPath); err != nil {
			log.Fatalf("セットアップに失敗しました: %v", err)
		}
		fmt.Println("セットアップ完了。もう一度起動するとサーバーが立ち上がります。")
		return
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		log.Fatalf("設定の読み込みに失敗しました: %v", err)
	}

	// 依存チェック（R-C 経路②）: 不足があればログで案内するだけで続行する
	// （[Y/n] 対話は初回ウィザード末尾のみ＝起動をブロックしない）。Windows は常に no-op。
	for _, issue := range platform.CheckHeadlessDeps(runtime.GOOS, runtime.GOARCH, cfg.InstallDirOrDefault(dir)) {
		log.Printf("依存不足: %s / %s", issue.Title, issue.GuideText())
	}

	// headless config ディレクトリを確保し、空なら同梱デフォルトを用意する。
	configDir := cfg.HeadlessConfigDirOrDefault(dir)
	if err := hlconfig.EnsureDefault(configDir); err != nil {
		log.Printf("デフォルトconfigの用意に失敗（続行します）: %v", err)
	}

	driver := headless.NewDriver(platform.ConsoleEncoding(cfg.Encoding))
	srv := server.New(cfg, cfgPath, driver, resonite.NewClient(), web.FS())

	// バックグラウンドワーカー（scheduler 等・Phase 8）を起動。stop で停止する。
	stopBackground := srv.Start()

	httpServer := &http.Server{Addr: ":" + strconv.Itoa(cfg.Port), Handler: srv.Handler()}
	go func() {
		fmt.Printf("MRHC: http://localhost:%d を開いてください（管理パスワードでログイン）\n", cfg.Port)
		// LAN アクセスと FW/ポート開放の案内（R-C・Linux のみ。Windows は OS が接続時に
		// 自動でプロンプトを出す。IP は複数 NIC で混乱するため列挙しない）。
		if runtime.GOOS == "linux" {
			fmt.Printf("LAN内の別PCからは http://<このPCのIP>:%d でアクセスできます。\n", cfg.Port)
			fmt.Printf("（接続できない場合はファイアウォールで TCP %d の開放が必要です。例:\n", cfg.Port)
			fmt.Printf("  sudo firewall-cmd --permanent --add-port=%d/tcp && sudo firewall-cmd --reload\n", cfg.Port)
			fmt.Printf("  / sudo ufw allow %d/tcp）\n", cfg.Port)
		}
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("サーバー起動に失敗しました: %v", err)
		}
	}()

	// SIGINT/SIGTERM で graceful 終了。稼働中のヘッドレスを shutdown して
	// orphan（管理不能な残存プロセス）を防ぐ。もう一度シグナルが来たら即終了。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\n終了シグナル受信。ヘッドレスを停止しています...（もう一度Ctrl-Cで即終了）")

	// 先にバックグラウンドワーカーを止める（予定発火を打ち切り、進行中の再起動 ①②③ を cancel して
	// 下の driver.Stop() と競合しないようにする）。
	stopBackground()

	if driver.Status().State != headless.StateStopped {
		go func() {
			<-sigCh
			fmt.Println("強制終了します。")
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
	fmt.Println("終了しました。")
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
