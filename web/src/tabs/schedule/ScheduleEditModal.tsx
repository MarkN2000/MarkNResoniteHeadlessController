import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Group, Modal, Stack, Text } from "@mantine/core";
import type { ConfigSummary, RestartType, ScheduledRestart } from "../../api";
import { FieldRow, InspectorNumberInput, InspectorSelect } from "../../components/inspector";
import { MIN_YEAR, WEEKDAY_KEYS, isValidOnceDate } from "./scheduleModel";

// config Select は空値を扱えないため番兵を使い、送信時に "" (=前回config) へ変換する（ManualCard と同方式）。
const PREV = "#prev";

// 予定の編集モーダル（§3.16(7)③）。ドラフトを編集し [OK] で working 配列へ反映、[キャンセル] で破棄。
// 種別で日時欄を出し分け。時/分の範囲と once の暦実在をインライン検証し、不正なら [OK] を無効化。
export function ScheduleEditModal({
  initial,
  isNew,
  configs,
  onApply,
  onClose,
}: {
  initial: ScheduledRestart;
  isNew: boolean;
  configs: ConfigSummary[];
  onApply: (s: ScheduledRestart) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<ScheduledRestart>(initial);

  const setField = <K extends keyof ScheduledRestart>(key: K, value: ScheduledRestart[K]) =>
    setDraft((d) => ({ ...d, [key]: value }));
  // 空入力は 0 に丸める（NumberInput の min/max が範囲を拘束・最終判定は valid で行う）。
  const num = (key: "year" | "month" | "day" | "hour" | "minute") => (v: number | string) =>
    setField(key, v === "" ? 0 : Number(v));

  // 種別変更。once へ切替時に年月日が未設定なら今日で初期化。
  // daily/weekly へ切替時は once 専用の年月日をクリアし、保存JSONに残らないようにする。
  const changeType = (v: string | null) => {
    if (!v) return;
    const type = v as RestartType;
    if (type === "once") {
      if (draft.year) {
        setField("type", type);
      } else {
        const now = new Date();
        setDraft((d) => ({ ...d, type, year: now.getFullYear(), month: now.getMonth() + 1, day: now.getDate() }));
      }
    } else {
      setDraft((d) => ({ ...d, type, year: undefined, month: undefined, day: undefined }));
    }
  };

  const typeData = [
    { value: "once", label: t("schedule.typeOnce") },
    { value: "weekly", label: t("schedule.typeWeekly") },
    { value: "daily", label: t("schedule.typeDaily") },
  ];
  const weekdayData = WEEKDAY_KEYS.map((k, i) => ({ value: String(i), label: t(k) }));
  const configData = [
    { value: PREV, label: t("schedule.usePrevious") },
    ...configs.map((c) => ({ value: c.name, label: c.name })),
  ];
  // 既存 configName が一覧に無い（削除済み）場合も表示できるよう補う。
  if (draft.configName && !configs.some((c) => c.name === draft.configName)) {
    configData.push({ value: draft.configName, label: draft.configName });
  }

  const timeValid = draft.hour >= 0 && draft.hour <= 23 && draft.minute >= 0 && draft.minute <= 59;
  const dateValid = draft.type !== "once" || isValidOnceDate(draft.year ?? 0, draft.month ?? 0, draft.day ?? 0);
  const valid = timeValid && dateValid;

  const numStyle = { width: 84 };
  // 縦並び日付の単位ラベル（年/月/日）。固定幅で3段の入力欄の右端を揃える。
  const dateUnitStyle = { width: 44, flexShrink: 0 } as const;

  return (
    <Modal opened onClose={onClose} title={t(isNew ? "schedule.newScheduleTitle" : "schedule.editScheduleTitle")} centered>
      <Stack gap="xs">
        <FieldRow label={t("schedule.scheduleType")}>
          <InspectorSelect data={typeData} value={draft.type} onChange={changeType} />
        </FieldRow>

        {draft.type === "once" && (
          // スマホ向けに年/月/日を縦並び（横一列は狭幅ではみ出すため・R8）。各入力は full-width＋右に単位ラベル。
          <FieldRow label={t("schedule.scheduleDate")} align="start">
            <Stack gap={4}>
              <Group gap={6} wrap="nowrap" align="center">
                <InspectorNumberInput
                  value={draft.year ?? 0}
                  onChange={num("year")}
                  min={MIN_YEAR}
                  max={2100}
                  allowNegative={false}
                  style={{ flex: 1 }}
                />
                <Text size="sm" c="dimmed" style={dateUnitStyle}>
                  {t("schedule.unitYear")}
                </Text>
              </Group>
              <Group gap={6} wrap="nowrap" align="center">
                <InspectorNumberInput value={draft.month ?? 0} onChange={num("month")} min={1} max={12} allowNegative={false} style={{ flex: 1 }} />
                <Text size="sm" c="dimmed" style={dateUnitStyle}>
                  {t("schedule.unitMonth")}
                </Text>
              </Group>
              <Group gap={6} wrap="nowrap" align="center">
                <InspectorNumberInput value={draft.day ?? 0} onChange={num("day")} min={1} max={31} allowNegative={false} style={{ flex: 1 }} />
                <Text size="sm" c="dimmed" style={dateUnitStyle}>
                  {t("schedule.unitDay")}
                </Text>
              </Group>
            </Stack>
          </FieldRow>
        )}

        {draft.type === "weekly" && (
          <FieldRow label={t("schedule.scheduleWeekday")}>
            <InspectorSelect
              data={weekdayData}
              value={String(draft.weekday)}
              onChange={(v) => v && setField("weekday", Number(v))}
            />
          </FieldRow>
        )}

        <FieldRow label={t("schedule.scheduleTime")}>
          <Group gap={4} wrap="nowrap" align="center">
            <InspectorNumberInput value={draft.hour} onChange={num("hour")} min={0} max={23} allowNegative={false} style={numStyle} />
            <Text size="sm">:</Text>
            <InspectorNumberInput value={draft.minute} onChange={num("minute")} min={0} max={59} allowNegative={false} style={numStyle} />
          </Group>
        </FieldRow>

        <FieldRow label={t("schedule.restartConfig")}>
          <InspectorSelect
            data={configData}
            value={draft.configName === "" ? PREV : draft.configName}
            onChange={(v) => setField("configName", v === PREV || !v ? "" : v)}
          />
        </FieldRow>

        {!dateValid && (
          <Text size="xs" c="red.5">
            {t("schedule.invalidDate")}
          </Text>
        )}

        <Group justify="flex-end" gap="xs" mt="sm">
          <Button variant="default" size="xs" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button color="brand" size="xs" disabled={!valid} onClick={() => onApply(draft)}>
            {t("schedule.modalOk")}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
