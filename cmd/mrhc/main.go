// Command mrhc is the MarkN Resonite Headless Controller (v2).
// On first run (no config) it launches an interactive CLI setup wizard;
// otherwise it loads the config and starts the HTTP server.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
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

	driver := headless.NewDriver()
	srv := server.New(cfg, cfgPath, driver, web.FS())

	fmt.Printf("MRHC: http://localhost:%d を開いてください（管理パスワードでログイン）\n", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("サーバー起動に失敗しました: %v", err)
	}
}
