import { useTranslation } from "react-i18next";
import { Stack, Text } from "@mantine/core";
import type { RestartWaitControl } from "../../api";
import { InspectorCard, FieldRow, InspectorNumberInput } from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { defaultWaitControl } from "./scheduleModel";

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
  // 空入力は 0 にフォールバック（2区間とも最小0＝相互依存なし・R9）。
  const num = (key: keyof RestartWaitControl, fallback: number) => (v: number | string) =>
    onChange({ ...value, [key]: v === "" ? fallback : Number(v) });

  // マーカークリック＝その項目を既定値へ戻す（確認あり）。
  const confirm = useConfirm();
  const resetProps = (apply: () => void, fieldLabel: string) => ({
    markerLabel: t("common.resetToDefault"),
    onMarkerClick: () =>
      confirm.ask({
        title: t("common.resetConfirmTitle"),
        message: t("common.resetConfirmMsg", { field: fieldLabel }),
        onConfirm: apply,
      }),
  });

  return (
    <InspectorCard title={t("schedule.waitTitle")}>
      <Stack gap="xs">
        <FieldRow
          label={t("schedule.quietWait")}
          {...resetProps(
            () => onChange({ ...value, quietWaitMin: defaultWaitControl().quietWaitMin }),
            t("schedule.quietWait"),
          )}
        >
          <InspectorNumberInput
            value={value.quietWaitMin}
            onChange={num("quietWaitMin", 0)}
            min={0}
            allowNegative={false}
          />
        </FieldRow>
        <FieldRow
          label={t("schedule.announceWait")}
          {...resetProps(
            () => onChange({ ...value, announceWaitMin: defaultWaitControl().announceWaitMin }),
            t("schedule.announceWait"),
          )}
        >
          <InspectorNumberInput
            value={value.announceWaitMin}
            onChange={num("announceWaitMin", 0)}
            min={0}
            allowNegative={false}
          />
        </FieldRow>
        <Text size="xs" c="dimmed">
          {t("schedule.waitNote")}
        </Text>
      </Stack>
      <ConfirmHost confirm={confirm} />
    </InspectorCard>
  );
}
