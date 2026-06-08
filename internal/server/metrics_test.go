package server

// システム使用率エンドポイントのテスト: 保持スナップショットの返却・ディスク未取得時の 0・
// 実機サンプリング（sampleCPUMem/sampleDisk）の populate。

import (
	"runtime"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/sysmetrics"
)

// TestSystemMetrics_ReturnsSnapshot は保持済みスナップショットがそのまま返ることを確認する。
func TestSystemMetrics_ReturnsSnapshot(t *testing.T) {
	ts, pw, _, srv := newCacheServer(t)
	srv.metricsMu.Lock()
	srv.cpumem = sysmetrics.CPUMem{CPUPercent: 42, MemUsedBytes: 4 << 30, MemTotalBytes: 16 << 30, MemPercent: 25}
	srv.cpumemOK = true
	srv.disk = sysmetrics.Disk{FreeBytes: 100 << 30, TotalBytes: 500 << 30}
	srv.diskOK = true
	srv.metricsMu.Unlock()

	var got okEnv[systemMetricsResp]
	authGet(t, ts.URL+"/api/v1/system/metrics", pw, &got)
	d := got.Data
	if !d.Supported || d.CPUPercent != 42 || d.MemUsedBytes != 4<<30 || d.MemTotalBytes != 16<<30 || d.MemPercent != 25 {
		t.Fatalf("cpu/mem mismatch: %+v", d)
	}
	if d.DiskFreeBytes != 100<<30 || d.DiskTotalBytes != 500<<30 {
		t.Fatalf("disk mismatch: %+v", d)
	}
}

// TestSystemMetrics_DiskUnavailable はディスク未取得時に Disk* が 0 で返ることを確認する。
func TestSystemMetrics_DiskUnavailable(t *testing.T) {
	ts, pw, _, srv := newCacheServer(t)
	srv.metricsMu.Lock()
	srv.cpumemOK = true // CPU/メモリは取得済み
	srv.diskOK = false  // ディスクは未取得
	srv.metricsMu.Unlock()

	var got okEnv[systemMetricsResp]
	authGet(t, ts.URL+"/api/v1/system/metrics", pw, &got)
	if !got.Data.Supported {
		t.Fatal("supported should be true (cpu/mem ok)")
	}
	if got.Data.DiskTotalBytes != 0 || got.Data.DiskFreeBytes != 0 {
		t.Fatalf("disk should be 0 when unavailable: %+v", got.Data)
	}
}

// TestSampleCPUMemDisk_Populates は実機サンプリングが値を埋めることを確認する（Win/Linux のみ）。
func TestSampleCPUMemDisk_Populates(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skipf("非対応OS（%s）はスキップ", runtime.GOOS)
	}
	_, _, _, srv := newCacheServer(t) // dataDir=tmp（実在）

	srv.sampleCPUMem()
	srv.sampleDisk()

	srv.metricsMu.RLock()
	defer srv.metricsMu.RUnlock()
	if !srv.cpumemOK {
		t.Fatal("cpumemOK should be true on supported OS")
	}
	if srv.cpumem.MemTotalBytes == 0 {
		t.Error("MemTotalBytes が 0")
	}
	if !srv.diskOK {
		t.Fatal("diskOK should be true (dataDir exists)")
	}
	if srv.disk.TotalBytes == 0 {
		t.Error("disk TotalBytes が 0")
	}
}
