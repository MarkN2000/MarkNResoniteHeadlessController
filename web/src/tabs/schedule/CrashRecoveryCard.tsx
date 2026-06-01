import { useTranslation } from "react-i18next";
import { Stack, Switch, Text } from "@mantine/core";
import type { RestartCrashRecovery } from "../../api";
import { InspectorCard, FieldRow, InspectorNumberInput } from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConfirm } from "../../hooks/useConfirm";
import { defaultCrashRecovery } from "./scheduleModel";

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

  // マーカークリック＝その項目を既定値へ戻す（確認あり）。閾値は無効時は非クリック（入力欄 disabled と整合）。
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
    <InspectorCard title={t("schedule.crashRecovery")}>
      <Stack gap="xs">
        <FieldRow
          label={t("schedule.enabled")}
          {...resetProps(() => onChange({ ...value, enabled: defaultCrashRecovery().enabled }), t("schedule.enabled"))}
        >
          <Switch checked={value.enabled} onChange={(e) => onChange({ ...value, enabled: e.currentTarget.checked })} />
        </FieldRow>
        <FieldRow
          label={t("schedule.maxCrashes")}
          {...(value.enabled
            ? resetProps(() => onChange({ ...value, maxCrashes: defaultCrashRecovery().maxCrashes }), t("schedule.maxCrashes"))
            : {})}
        >
          <InspectorNumberInput
            value={value.maxCrashes}
            onChange={num("maxCrashes")}
            min={1}
            allowNegative={false}
            disabled={!value.enabled}
          />
        </FieldRow>
        <FieldRow
          label={t("schedule.windowMinutes")}
          {...(value.enabled
            ? resetProps(() => onChange({ ...value, windowMinutes: defaultCrashRecovery().windowMinutes }), t("schedule.windowMinutes"))
            : {})}
        >
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
      <ConfirmModal
        opened={confirm.request !== null}
        title={confirm.request?.title ?? ""}
        message={confirm.request?.message}
        danger={confirm.request?.danger}
        loading={confirm.busy}
        onConfirm={() => void confirm.confirm()}
        onClose={confirm.close}
      />
    </InspectorCard>
  );
}
