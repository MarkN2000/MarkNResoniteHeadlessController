package server

// jsonstate.go は「無ければ空・あれば読む」型の小さな状態ファイル（runtime-state / favorites 等）を
// 読み書きするジェネリックヘルパ。秘密を含み得るため 0600 で書く。設定本体(mrhc.config.json)とは別系統。

import (
	"encoding/json"
	"os"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
)

// readJSONFile は path の JSON を T に読み込む。存在しない/壊れている場合は T のゼロ値を返す
// （状態ファイルは「無ければ空で良い」ため、欠損・破損を黙ってゼロ値に倒す）。
func readJSONFile[T any](path string) T {
	var v T
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &v)
	}
	return v
}

// writeJSONFile は v を整形 JSON として path に 0600 で書く（秘密を含み得る状態ファイル用）。
// 保存失敗を呼び出し元に返し、既存ファイルは途中で切り詰めない。
func writeJSONFile[T any](path string, v T) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(path, b)
}
