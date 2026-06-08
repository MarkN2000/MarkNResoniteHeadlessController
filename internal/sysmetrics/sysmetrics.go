// Package sysmetrics はマシン全体のリソース使用率（CPU/メモリ/ディスク）を
// OS 非依存のインターフェースで取得する。実体は OS 別ファイル（build tag）に置く:
//   - sysmetrics_linux.go   : /proc/stat・/proc/meminfo・syscall.Statfs
//   - sysmetrics_windows.go : kernel32 GetSystemTimes / GlobalMemoryStatusEx / GetDiskFreeSpaceEx
//   - sysmetrics_other.go   : 非対応OS（compile 保険・ErrUnsupported）
//
// 依存は標準ライブラリ＋既存の golang.org/x/sys のみ（新規依存なし＝単一バイナリ同梱のまま）。
// CPU 使用率は累積カウンタの「2時点の差分」が必要なため、Sampler が前回値を保持する。
// メモリ/ディスクは瞬時値でよい（差分不要）。
package sysmetrics

import "errors"

// ErrUnsupported はこのプラットフォームで取得手段が無いことを表す（mac 等の _other.go）。
var ErrUnsupported = errors.New("system metrics not supported on this platform")

// CPUMem はマシン全体の CPU/メモリ使用状況の瞬時スナップショット。
type CPUMem struct {
	CPUPercent    float64 // 0..100（直近サンプル区間の平均・マシン全体）
	MemUsedBytes  uint64
	MemTotalBytes uint64
	MemPercent    float64 // 0..100（OS 権威値: Win=dwMemoryLoad / Linux=used/total）
}

// Disk はあるパスが属するファイルシステムの空き/総容量。
type Disk struct {
	FreeBytes  uint64
	TotalBytes uint64
}

// Sampler は CPU 使用率算出のため前回の累積 CPU 時間を保持する。
// 単一 goroutine からの逐次 Sample() 呼び出しを前提とする（内部ロックは持たない）。
type Sampler struct {
	prevBusy  uint64
	prevTotal uint64
	primed    bool // 初回サンプルで baseline を確立したか
}

// NewSampler は新しい Sampler を返す。
func NewSampler() *Sampler { return &Sampler{} }

// Reset は CPU 使用率の baseline を破棄する（次回 Sample() は再 prime ＝CPUPercent=0）。
// アイドル明けに古い累積値との巨大な差分（誤った長区間平均）を出さないために使う。
// Sample() と同じ goroutine から呼ぶ前提（内部ロックなし）。
func (s *Sampler) Reset() { s.primed = false }

// Sample は現在の CPU/メモリを読み、前回値との差分から CPU 使用率を算出する。
// 初回呼び出しは baseline 確立のみで CPUPercent=0（メモリは初回から正確）。
func (s *Sampler) Sample() (CPUMem, error) {
	busy, total, err := readCPUTimes()
	if err != nil {
		return CPUMem{}, err
	}
	memUsed, memTotal, memPct, err := readMem()
	if err != nil {
		return CPUMem{}, err
	}
	var cpuPct float64
	if s.primed {
		cpuPct = cpuPercent(s.prevBusy, s.prevTotal, busy, total)
	}
	s.prevBusy, s.prevTotal, s.primed = busy, total, true
	return CPUMem{
		CPUPercent:    cpuPct,
		MemUsedBytes:  memUsed,
		MemTotalBytes: memTotal,
		MemPercent:    memPct,
	}, nil
}

// ReadDisk は path が属するファイルシステムの空き/総容量を返す（瞬時値・差分不要）。
func ReadDisk(path string) (Disk, error) {
	free, total, err := readDisk(path)
	if err != nil {
		return Disk{}, err
	}
	return Disk{FreeBytes: free, TotalBytes: total}, nil
}

// cpuPercent は2つの累積サンプル間の busy 比率（0..100）を返す（純関数・テスト対象）。
// カウンタのリセット/巻き戻し（再起動・wrap）や区間ゼロは 0 にフォールバックする。
func cpuPercent(prevBusy, prevTotal, busy, total uint64) float64 {
	if total <= prevTotal || busy < prevBusy {
		return 0
	}
	dt := total - prevTotal
	db := busy - prevBusy
	p := float64(db) / float64(dt) * 100
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	default:
		return p
	}
}
