import { useTranslation } from "react-i18next";
import { Stack, Text } from "@mantine/core";
import type { RestartWaitControl } from "../../api";
import { InspectorCard, FieldRow, InspectorNumberInput } from "../../components/inspector";

// ④待機制御カード（§3.16(7)）。安全再起動フロー共通のグローバル待機/告知タイミング（分）。
// 予定ごとの override は持たない（YAGNI）。
export function WaitControlCard({
  value,
  onChange,
}: {
  value: RestartWaitControl;
  onChange: (v: RestartWaitControl) => void;
}) {
  const { t } = useTranslation();
  // 空入力は 0 として扱う（保存時に backend がクランプ）。
  const num = (key: keyof RestartWaitControl) => (v: number | string) =>
    onChange({ ...value, [key]: v === "" ? 0 : Number(v) });

  return (
    <InspectorCard title={t("schedule.waitTitle")}>
      <Stack gap="xs">
        <FieldRow label={t("schedule.forceRestartTimeout")}>
          <InspectorNumberInput
            value={value.forceRestartTimeoutMin}
            onChange={num("forceRestartTimeoutMin")}
            min={1}
            allowNegative={false}
          />
        </FieldRow>
        <FieldRow label={t("schedule.actionTiming")}>
          <InspectorNumberInput
            value={value.actionTimingMin}
            onChange={num("actionTimingMin")}
            min={0}
            allowNegative={false}
          />
        </FieldRow>
        <Text size="xs" c="dimmed">
          {t("schedule.waitNote")}
        </Text>
      </Stack>
    </InspectorCard>
  );
}
