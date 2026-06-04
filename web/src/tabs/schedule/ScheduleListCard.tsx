import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Group, Stack, Switch, Text } from "@mantine/core";
import type { ConfigSummary, ScheduledRestart, WriteResult } from "../../api";
import { InspectorButton, InspectorCard, RowIconButton } from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { ScheduleEditModal } from "./ScheduleEditModal";
import { WEEKDAY_KEYS, defaultScheduled, formatScheduleTime, typeKey } from "./scheduleModel";

// ③予定リストカード（§3.16(7)）。予定の CRUD は「設定の一括保存」から分離し、その場で即保存する。
//   追加/編集/有効切替 … その場で PUT（成功トースト・useAsyncAction）
//   削除 … 確認ダイアログ→PUT（即時のため取り消しが効かないので確認を挟む・useConfirm）
// 永続化（PUT＋state更新）は親の onPersist が担い、トーストはここで1回だけ出す（二重発火を避ける）。
// 待機設定/事前アクション/クラッシュ復帰は従来どおり下部の一括「保存」で永続化する。
export function ScheduleListCard({
  schedules,
  configs,
  onPersist,
}: {
  schedules: ScheduledRestart[];
  configs: ConfigSummary[];
  onPersist: (scheduled: ScheduledRestart[]) => Promise<WriteResult>;
}) {
  const { t } = useTranslation();
  // 編集対象（コピー）。null=モーダル閉。新規/既存は id が一覧にあるかで判定。
  const [editing, setEditing] = useState<ScheduledRestart | null>(null);
  const isNew = editing !== null && !schedules.some((s) => s.id === editing.id);
  const confirm = useConfirm();
  const act = useAsyncAction();

  const toggle = (id: string, enabled: boolean) =>
    void act.run(
      () => onPersist(schedules.map((s) => (s.id === id ? { ...s, enabled } : s))),
      t("schedule.toastScheduleSaved"),
    );
  const askRemove = (id: string) =>
    confirm.ask({
      title: t("schedule.deleteScheduleTitle"),
      message: t("schedule.confirmDeleteSchedule"),
      danger: true,
      success: t("schedule.toastScheduleDeleted"),
      onConfirm: () => onPersist(schedules.filter((s) => s.id !== id)),
    });
  const apply = (s: ScheduledRestart) => {
    const exists = schedules.some((x) => x.id === s.id);
    const next = exists ? schedules.map((x) => (x.id === s.id ? s : x)) : [...schedules, s];
    void act.run(() => onPersist(next), t("schedule.toastScheduleSaved"));
    setEditing(null);
  };

  return (
    <InspectorCard
      title={t("schedule.scheduleListTitle")}
      actions={
        <InspectorButton severity="neutral" disabled={act.busy} onClick={() => setEditing(defaultScheduled())}>
          ＋ {t("schedule.addSchedule")}
        </InspectorButton>
      }
    >
      <Stack gap={6}>
        {schedules.map((s) => {
          const time = formatScheduleTime(s, t(WEEKDAY_KEYS[s.weekday] ?? WEEKDAY_KEYS[0]));
          return (
            <Group key={s.id} gap={4} wrap="nowrap" align="center">
              <Switch size="xs" checked={s.enabled} onChange={(e) => toggle(s.id, e.currentTarget.checked)} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <Text size="sm" style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {t(typeKey(s.type))}・{time}
                </Text>
                <Text size="xs" c="dimmed" style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {s.configName || t("schedule.usePrevious")}
                </Text>
              </div>
              <RowIconButton color="gray" label={t("schedule.editSchedule")} onClick={() => setEditing({ ...s })}>
                ✎
              </RowIconButton>
              <RowIconButton color="red" label={t("schedule.deleteSchedule")} onClick={() => askRemove(s.id)}>
                ×
              </RowIconButton>
            </Group>
          );
        })}
        {schedules.length === 0 && (
          <Text size="xs" c="dimmed" ta="center" mt="xs">
            {t("schedule.noSchedules")}
          </Text>
        )}
      </Stack>

      {editing && (
        <ScheduleEditModal
          initial={editing}
          isNew={isNew}
          configs={configs}
          onApply={apply}
          onClose={() => setEditing(null)}
        />
      )}

      <ConfirmHost confirm={confirm} />
    </InspectorCard>
  );
}
