//go:build !windows

package main

import (
	"os"
	"syscall"
)

// relaunch は exePath（自己更新の swap 済み＝新版が載った元の設置パス）で自分自身を起動し直す（Unix）。
// syscall.Exec で現在のプロセスイメージを新バイナリへ置換する（PID・端末・cwd・env を保持）。
// 呼び出し元は事前に HTTP リスナーを閉じ・ヘッドレスを停止しているため、ポートは解放済み・
// 子プロセスの取り残しも無い。成功時は戻らない（プロセスが置き換わる）。
//
// exePath は起動時（swap 前）に捕捉した元名のパスを渡すこと。os.Executable() を swap 後に
// 呼ぶと <exe>.old（旧版）を指すため、ここで取り直してはならない。
func relaunch(exePath string, args []string) error {
	// argv[0] には exePath を入れる（絶対パス。元の argv[0] が相対でも確実に新版を指す）。
	return syscall.Exec(exePath, append([]string{exePath}, args...), os.Environ())
}
