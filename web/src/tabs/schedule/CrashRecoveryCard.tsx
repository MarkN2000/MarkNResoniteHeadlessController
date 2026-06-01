import { useTranslation } from "react-i18next";
import { Stack, Switch, Text } from "@mantine/core";
import type { RestartCrashRecovery } from "../../api";
import { InspectorCard, FieldRow, InspectorNumberInput } from "../../components/inspector";

// ⑥クラッシュ復帰カード（§3.16(4)(7)）。意図しない終了を検知し直近 config で自動再起動。
// 無効時は閾値フィールドを disabled（編集不可）にして関係を明示。
export function CrashRecoveryCard({
  value,
  onChange,
}: {
  value: RestartCrashRecovery;
  onChange: (v: RestartCrashRecovery) => void;
}) {
  const { t } = useTranslation();
  // 空入力は 1 として扱う（backend は最小 1 にクランプ）。
  const num = (key: "maxCrashes" | "windowMinutes") => (v: number | string) =>
    onChange({ ...value, [key]: v === "" ? 1 : Number(v) });

  return (
    <InspectorCard title={t("schedule.crashRecovery")}>
      <Stack gap="xs">
        <FieldRow label={t("schedule.enabled")}>
          <Switch checked={value.enabled} onChange={(e) => onChange({ ...value, enabled: e.currentTarget.checked })} />
        </FieldRow>
        <FieldRow label={t("schedule.maxCrashes")}>
          <InspectorNumberInput
            value={value.maxCrashes}
            onChange={num("maxCrashes")}
            min={1}
            allowNegative={false}
            disabled={!value.enabled}
          />
        </FieldRow>
        <FieldRow label={t("schedule.windowMinutes")}>
          <InspectorNumberInput
            value={value.windowMinutes}
            onChange={num("windowMinutes")}
            min={1}
            allowNegative={false}
            disabled={!value.enabled}
          />
        </FieldRow>
        <Text size="xs" c="dimmed">
          {t("schedule.crashNote")}
        </Text>
      </Stack>
    </InspectorCard>
  );
}
