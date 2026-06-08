package server

// システム使用率（マシン全体の CPU/メモリ/ディスク）。スケジュールタブの「システム使用率」カードへ。
// バックグラウンド・サンプラーが最新スナップショットを保持し、エンドポイントは即返す（HTTP 応答内で待たない）。
// 取得頻度: CPU/メモリ=2秒（CPU は累積差分が必要）、ディスク=30秒（残量は急変しない・statfs を無駄に呼ばない）。
// 取得実体は internal/sysmetrics（OS 別・新規依存なし）。

import (
	"context"
	"net/http"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/sysmetrics"
)

const (
	metricsCPUInterval  = 2 * time.Second
	metricsDiskInterval = 30 * time.Second
	// metricsActiveWindow: 最後のメトリクス要求からこの間だけ実サンプリングする。
	// 3秒ポーリング中は毎回延長されてアクティブ維持。UI が離れる/閉じると約この時間で停止する。
	metricsActiveWindow = 10 * time.Second
)

// systemMetricsResp は GET /api/v1/system/metrics の応答。
type systemMetricsResp struct {
	Supported      bool    `json:"supported"` // false=非対応OS/未サンプル（CPU/メモリ取得不可）
	CPUPercent     float64 `json:"cpuPercent"`
	MemUsedBytes   uint64  `json:"memUsedBytes"`
	MemTotalBytes  uint64  `json:"memTotalBytes"`
	MemPercent     float64 `json:"memPercent"`
	DiskFreeBytes  uint64  `json:"diskFreeBytes"`
	DiskTotalBytes uint64  `json:"diskTotalBytes"` // 0=ディスク取得不可（UI は「—」表示）
}

// runMetricsSampler は CPU/メモリ（2秒）とディスク（30秒）を別ティッカーでサンプリングし、
// 最新値を s.metrics* に保持する。ctx（bg ctx）の cancel で停止する。Start() から1回起動する。
// アクセス駆動の遅延ゲート: 直近に /system/metrics が要求されていないアイドル時は実取得せず、
// ティック毎の atomic 比較だけで syscall を回避する（誰も見ていない間は実質無処理）。
func (s *Server) runMetricsSampler(ctx context.Context) {
	cpuTick := time.NewTicker(metricsCPUInterval)
	diskTick := time.NewTicker(metricsDiskInterval)
	defer cpuTick.Stop()
	defer diskTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cpuTick.C:
			if s.metricsActive() {
				s.sampleCPUMem()
			} else {
				s.sysSampler.Reset() // アイドル明けに巨大区間の誤った CPU% を出さないため baseline を破棄
			}
		case <-diskTick.C:
			if s.metricsActive() {
				s.sampleDisk()
			}
		}
	}
}

// metricsActive は直近の /system/metrics 要求が有効窓内かを返す（遅延ゲートの判定）。
func (s *Server) metricsActive() bool {
	return time.Now().UnixNano() < s.metricsActiveUntil.Load()
}

// sampleCPUMem は CPU/メモリを1回サンプリングして保持する（runMetricsSampler 専用）。
func (s *Server) sampleCPUMem() {
	cm, err := s.sysSampler.Sample()
	s.metricsMu.Lock()
	if err != nil {
		s.cpumemOK = false
	} else {
		s.cpumem = cm
		s.cpumemOK = true
	}
	s.metricsMu.Unlock()
}

// sampleDisk はデータ領域のディスク容量を1回サンプリングして保持する（runMetricsSampler 専用）。
func (s *Server) sampleDisk() {
	if s.dataDir == "" {
		return
	}
	d, err := sysmetrics.ReadDisk(s.dataDir)
	s.metricsMu.Lock()
	if err != nil {
		s.diskOK = false
	} else {
		s.disk = d
		s.diskOK = true
	}
	s.metricsMu.Unlock()
}

// handleSystemMetrics: GET /api/v1/system/metrics → 保持済みスナップショットを即返す。
func (s *Server) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	// アクセス駆動ゲートの有効窓を延長（このリクエストでサンプラを起こし続ける）。
	s.metricsActiveUntil.Store(time.Now().Add(metricsActiveWindow).UnixNano())
	s.metricsMu.RLock()
	resp := systemMetricsResp{
		Supported:     s.cpumemOK,
		CPUPercent:    s.cpumem.CPUPercent,
		MemUsedBytes:  s.cpumem.MemUsedBytes,
		MemTotalBytes: s.cpumem.MemTotalBytes,
		MemPercent:    s.cpumem.MemPercent,
	}
	if s.diskOK {
		resp.DiskFreeBytes = s.disk.FreeBytes
		resp.DiskTotalBytes = s.disk.TotalBytes
	}
	s.metricsMu.RUnlock()
	writeOK(w, resp)
}
