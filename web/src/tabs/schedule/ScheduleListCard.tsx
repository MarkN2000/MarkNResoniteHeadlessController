import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Group, Stack, Switch, Text } from "@mantine/core";
import type { ConfigSummary, ScheduledRestart } from "../../api";
import { InspectorButton, InspectorCard, RowIconButton } from "../../components/inspector";
import { ScheduleEditModal } from "./ScheduleEditModal";
import { WEEKDAY_KEYS, defaultScheduled, formatScheduleTime, typeKey } from "./scheduleModel";

// ③予定リストカード（§3.16(7)）。再起動予定の CRUD。各行=有効トグル/種別・時刻/config/編集/削除。
// 編集はモーダル（ドラフト→[OK]で working 配列へ反映）。削除は直接（未保存なので保存前なら取り消し可）。
// 永続化は親（ScheduleTab）の一括保存バー。
export function ScheduleListCard({
  schedules,
  configs,
  onChange,
}: {
  schedules: ScheduledRestart[];
  configs: ConfigSummary[];
  onChange: (s: ScheduledRestart[]) => void;
}) {
  const { t } = useTranslation();
  // 編集対象（コピー）。null=モーダル閉。新規/既存は id が一覧にあるかで判定。
  const [editing, setEditing] = useState<ScheduledRestart | null>(null);
  const isNew = editing !== null && !schedules.some((s) => s.id === editing.id);

  const toggle = (id: string, enabled: boolean) =>
    onChange(schedules.map((s) => (s.id === id ? { ...s, enabled } : s)));
  const remove = (id: string) => onChange(schedules.filter((s) => s.id !== id));
  const apply = (s: ScheduledRestart) => {
    const exists = schedules.some((x) => x.id === s.id);
    onChange(exists ? schedules.map((x) => (x.id === s.id ? s : x)) : [...schedules, s]);
    setEditing(null);
  };

  return (
    <InspectorCard
      title={t("schedule.scheduleListTitle")}
      actions={
        <InspectorButton severity="neutral" onClick={() => setEditing(defaultScheduled())}>
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
              <RowIconButton color="red" label={t("schedule.deleteSchedule")} onClick={() => remove(s.id)}>
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
    </InspectorCard>
  );
}
