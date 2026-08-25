import { useCallback, useEffect, useState } from "react";
import * as api from "../api";
import type { SystemMetrics } from "../api";
import { useVisiblePolling } from "./useVisiblePolling";

const METRICS_POLL_INTERVAL_MS = 3_000;

// マシン全体の使用率を表示中だけ取得する。取得失敗では直前値を維持し、表示のちらつきを防ぐ。
export function useSystemMetrics(): SystemMetrics | null {
  const [metrics, setMetrics] = useState<SystemMetrics | null>(null);

  const refetch = useCallback(async () => {
    const next = await api.getSystemMetrics();
    if (next !== null) setMetrics(next);
  }, []);

  useEffect(() => {
    void refetch();
  }, [refetch]);
  useVisiblePolling(refetch, METRICS_POLL_INTERVAL_MS);

  return metrics;
}
