import { useTranslation } from "react-i18next";
import { Text } from "@mantine/core";
import { InspectorCard, FieldRow } from "../../components/inspector";
import { useSystemMetrics } from "../../hooks/useSystemMetrics";
import { formatBytes } from "../../lib/format";

// システム使用率カード（マシン全体の CPU/メモリ/ディスク・数値のみ表示）。
// 共通 useSystemMetrics で取得＋ポーリングする。ScheduleTab は配置するだけ。
// マシン全体の値なのでヘッドレス稼働状態に依存せず常に表示（停止中も負荷が見える）。
export function SystemMetricsCard() {
  const { t } = useTranslation();
  const m = useSystemMetrics();

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
