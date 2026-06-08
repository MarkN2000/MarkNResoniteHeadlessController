package sysmetrics

import (
	"os"
	"runtime"
	"testing"
)

// TestCPUPercent は累積カウンタ差分から使用率を導く純関数を検証する（OS 非依存）。
func TestCPUPercent(t *testing.T) {
	cases := []struct {
		name                                 string
		prevBusy, prevTotal, busy, total     uint64
		want                                 float64
	}{
		{"half busy", 0, 0, 50, 100, 50},
		{"quarter busy", 100, 200, 125, 300, 25}, // db=25 dt=100 → 25%
		{"idle", 10, 100, 10, 200, 0},            // busy 変化なし
		{"fully busy", 0, 0, 100, 100, 100},
		{"zero interval", 100, 200, 100, 200, 0}, // dt=0
		{"counter reset", 100, 200, 10, 50, 0},   // total < prev（巻き戻し）
		{"busy regress", 100, 200, 50, 300, 0},   // busy < prev
		{"clamp over 100", 0, 0, 150, 100, 100},  // 異常値はクランプ
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := cpuPercent(c.prevBusy, c.prevTotal, c.busy, c.total)
			if got != c.want {
				t.Errorf("cpuPercent(%d,%d,%d,%d)=%v want %v",
					c.prevBusy, c.prevTotal, c.busy, c.total, got, c.want)
			}
		})
	}
}

// TestSampleSmoke は実機で Sample() が妥当な値域を返すことを確認する（Win/Linux のみ）。
func TestSampleSmoke(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skipf("非対応OS（%s）はスキップ", runtime.GOOS)
	}
	s := NewSampler()
	cm, err := s.Sample()
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if cm.MemTotalBytes == 0 {
		t.Error("MemTotalBytes が 0")
	}
	if cm.MemUsedBytes > cm.MemTotalBytes {
		t.Errorf("MemUsedBytes(%d) > MemTotalBytes(%d)", cm.MemUsedBytes, cm.MemTotalBytes)
	}
	if cm.MemPercent < 0 || cm.MemPercent > 100 {
		t.Errorf("MemPercent 値域外: %v", cm.MemPercent)
	}
	// 2回目は CPU 使用率が算出される（0..100）。
	cm2, err := s.Sample()
	if err != nil {
		t.Fatalf("Sample(2): %v", err)
	}
	if cm2.CPUPercent < 0 || cm2.CPUPercent > 100 {
		t.Errorf("CPUPercent 値域外: %v", cm2.CPUPercent)
	}
}

// TestReadDiskSmoke は実機でディスク容量取得が妥当な値を返すことを確認する（Win/Linux のみ）。
func TestReadDiskSmoke(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skipf("非対応OS（%s）はスキップ", runtime.GOOS)
	}
	d, err := ReadDisk(os.TempDir())
	if err != nil {
		t.Fatalf("ReadDisk: %v", err)
	}
	if d.TotalBytes == 0 {
		t.Error("TotalBytes が 0")
	}
	if d.FreeBytes > d.TotalBytes {
		t.Errorf("FreeBytes(%d) > TotalBytes(%d)", d.FreeBytes, d.TotalBytes)
	}
}
