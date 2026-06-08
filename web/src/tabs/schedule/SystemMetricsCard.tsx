import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Text } from "@mantine/core";
import * as api from "../../api";
import type { SystemMetrics } from "../../api";
import { InspectorCard, FieldRow } from "../../components/inspector";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { formatBytes } from "../../lib/format";

// システム使用率カード（マシン全体の CPU/メモリ/ディスク・数値のみ表示）。
// 自己完結型: 自身で取得＋ポーリング（CacheSection と同型）。ScheduleTab は配置するだけ。
// マシン全体の値なのでヘッドレス稼働状態に依存せず常に表示（停止中も負荷が見える）。
export function SystemMetricsCard() {
  const { t } = useTranslation();
  const [m, setM] = useState<SystemMetrics | null>(null);

  const refetch = useCallback(async () => {
    setM(await api.getSystemMetrics());
  }, []);
  useEffect(() => {
    void refetch(); // マウント直後の即時取得（以降は useVisiblePolling が担当）
  }, [refetch]);
  useVisiblePolling(refetch, 3000); // 表示中のみ3秒ポーリング（非表示で停止）

  const dim = (text: string) => (
    <Text size="sm" c="dimmed" ta="center">
      {text}
    </Text>
  );

  const ready = m !== null && m.supported;
  const diskReady = m !== null && m.diskTotalBytes > 0;

  return (
    <InspectorCard title={t("schedule.systemTitle")}>
      <FieldRow label={t("schedule.cpu")}>{dim(ready ? `${Math.round(m.cpuPercent)}%` : "—")}</FieldRow>
      <FieldRow label={t("schedule.memory")}>
        {dim(ready ? `${formatBytes(m.memUsedBytes)} / ${formatBytes(m.memTotalBytes)} (${Math.round(m.memPercent)}%)` : "—")}
      </FieldRow>
      <FieldRow label={t("schedule.disk")}>
        {dim(diskReady ? t("schedule.diskFree", { free: formatBytes(m.diskFreeBytes), total: formatBytes(m.diskTotalBytes) }) : "—")}
      </FieldRow>
      {m !== null && !m.supported && (
        <Text size="xs" c="dimmed" ta="center">
          {t("schedule.metricsUnsupported")}
        </Text>
      )}
    </InspectorCard>
  );
}
