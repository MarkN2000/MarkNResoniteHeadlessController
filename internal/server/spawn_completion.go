package server

import (
	"fmt"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
)

const (
	// spawnReadyDelay は Resonite の完了行を確認してから受信アイテムへ impulse を送るまでの猶予。
	spawnReadyDelay = 500 * time.Millisecond
	// spawnCommandTimeout はアセット取得を含む spawn コマンドの最大待機時間。
	// 通常はプロンプト復帰で即終了し、ハングした場合だけこの上限まで待つ。
	spawnCommandTimeout = 60 * time.Second
)

// execTemporarySpawn は一時アイテムをspawnし、実機の完了行まで確認する。
// セッションのスポーン＆パルスとスケジュール告知で同じ成功条件を使う。
func execTemporarySpawn(exec headless.Tx, itemURL string) error {
	lines, err := exec.Exec(headless.SpawnCmd(itemURL, true, false), headless.WithTimeout(spawnCommandTimeout))
	if err != nil {
		return err
	}
	if !headless.SpawnCompleted(lines, itemURL) {
		return fmt.Errorf("アイテムのスポーン完了を確認できませんでした: %s", itemURL)
	}
	return nil
}
