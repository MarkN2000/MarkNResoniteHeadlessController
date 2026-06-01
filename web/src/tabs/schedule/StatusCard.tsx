import { useTranslation } from "react-i18next";
import { Group, Text } from "@mantine/core";
import type { RestartStatus } from "../../api";
import { InspectorCard, FieldRow, InspectorButton } from "../../components/inspector";
import { formatDateTime, formatDuration, formatRemaining, isCancellablePhase, phaseKey, triggerKey } from "./scheduleModel";

// ①ステータスカード（§3.16(7)）。restart-status を表示し、進行中は現在フェーズ＋残り時間＋[中止]。
// status の poll は ScheduleTab が担当（ここは表示のみ）。onCancel は確認モーダルを開く。
export function StatusCard({
  status,
  running,
  onCancel,
}: {
  status: RestartStatus | null;
  running: boolean;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const s = status;
  const inProgress = !!s?.inProgress;
  const cancellable = inProgress && isCancellablePhase(s!.phase);

  // 進行状態の表示（フェーズ＋待機中は残り時間）。
  let progressText = "—";
  if (inProgress && s) {
    progressText = t(phaseKey(s.phase));
    if (s.deadlineAt && (s.phase === "waiting" || s.phase === "announcing")) {
      progressText += `（${t("schedule.remaining")} ${formatRemaining(s.deadlineAt)}）`;
    }
  }

  const dim = (text: string) => (
    <Text size="sm" c="dimmed" ta="center">
      {text}
    </Text>
  );

  return (
    <InspectorCard title={t("schedule.statusTitle")}>
      {running && s && <FieldRow label={t("schedule.uptime")}>{dim(formatDuration(s.uptimeSeconds))}</FieldRow>}

      <FieldRow label={t("schedule.nextScheduled")}>
        {s?.nextScheduledAt
          ? dim(`${formatDateTime(s.nextScheduledAt)}・${s.nextScheduledConfigName || t("schedule.usePrevious")}`)
          : dim(t("schedule.none"))}
      </FieldRow>

      <FieldRow label={t("schedule.lastRestart")}>
        {s?.lastRestartAt
          ? dim(`${formatDateTime(s.lastRestartAt)}・${t(triggerKey(s.lastRestartTrigger ?? "manual"))}`)
          : dim(t("schedule.none"))}
      </FieldRow>

      {running && (
        <FieldRow label={t("schedule.progress")}>
          {inProgress ? (
            <Group justify="space-between" wrap="nowrap" gap="xs">
              <Text size="sm" c="yellow.6" style={{ flex: 1, minWidth: 0 }}>
                {progressText}
              </Text>
              {cancellable && (
                <InspectorButton severity="danger" onClick={onCancel} style={{ flexShrink: 0 }}>
                  {t("schedule.cancel")}
                </InspectorButton>
              )}
            </Group>
          ) : (
            dim(progressText)
          )}
        </FieldRow>
      )}

      <FieldRow label={t("schedule.crashRecovery")}>
        {dim(s?.crashRecoveryEnabled ? t("schedule.enabled") : t("schedule.disabled"))}
      </FieldRow>
    </InspectorCard>
  );
}
