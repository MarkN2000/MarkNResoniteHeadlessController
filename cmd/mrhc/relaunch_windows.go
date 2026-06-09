//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// relaunch は新バイナリで自分自身を起動し直す（Windows）。
// Windows には exec 置換が無いため、新バイナリを子プロセスとして起動し、本プロセスは終了する。
// 呼び出し元は事前に HTTP リスナーを閉じ・ヘッドレスを停止しているため、ポートは解放済み。
// CREATE_NEW_PROCESS_GROUP で、親終了時の Ctrl+C 等が子へ伝播しないようにする。
// 標準入出力は引き継ぎ、ログ出力を同じコンソールへ継続させる（コンソールが残る運用を想定）。
func relaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = os.Environ()
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
	return cmd.Start()
}
