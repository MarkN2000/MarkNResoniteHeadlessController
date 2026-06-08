//go:build !linux && !windows

package sysmetrics

// 非対応プラットフォーム（mac 等）。ビルドは通しつつ取得は ErrUnsupported を返す。
// 本番ターゲットは Windows/Linux のみ。dev/CI が他OSでも compile できるための保険。

func readCPUTimes() (busy, total uint64, err error) { return 0, 0, ErrUnsupported }

func readMem() (used, total uint64, percent float64, err error) { return 0, 0, 0, ErrUnsupported }

func readDisk(path string) (free, total uint64, err error) { return 0, 0, ErrUnsupported }
