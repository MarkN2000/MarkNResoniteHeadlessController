//go:build !windows

package main

import (
	"os"
	"syscall"
)

// relaunch は新バイナリで自分自身を起動し直す（Unix）。
// syscall.Exec で現在のプロセスイメージを新バイナリへ置換する（PID・端末・cwd・env を保持）。
// 呼び出し元は事前に HTTP リスナーを閉じ・ヘッドレスを停止しているため、ポートは解放済み・
// 子プロセスの取り残しも無い。成功時は戻らない（プロセスが置き換わる）。
func relaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// argv[0] を含めて元の引数をそのまま渡す（os.Args[0] が argv[0]）。
	return syscall.Exec(exe, os.Args, os.Environ())
}
