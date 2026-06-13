import { useTranslation } from "react-i18next";
import { Stack, Switch, Text } from "@mantine/core";
import { InspectorCard, FieldRow } from "../../components/inspector";

// Resonite 自動更新トグル（P9-B）。ON かつ Steam 設定済みのとき、対象の停止→起動の間に
// DepotDownloader で更新する（＝最新確認＋適用）。Steam 未設定なら no-op。いずれも既定 ON。
//   - scheduled: 予定再起動の前に更新
//   - manual:    手動起動（トップバー）・手動「通常再起動」の前に更新
// クラッシュ復帰・通常停止は対象外。
export function UpdateOnRestartCard({
  scheduled,
  manual,
  onScheduledChange,
  onManualChange,
}: {
  scheduled: boolean;
  manual: boolean;
  onScheduledChange: (v: boolean) => void;
  onManualChange: (v: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <InspectorCard title={t("schedule.updateCard")}>
      <Stack gap="xs">
        <FieldRow label={t("schedule.updateOnRestart")}>
          <Switch checked={scheduled} onChange={(e) => onScheduledChange(e.currentTarget.checked)} />
        </FieldRow>
        <FieldRow label={t("schedule.updateBeforeManualStart")}>
          <Switch checked={manual} onChange={(e) => onManualChange(e.currentTarget.checked)} />
        </FieldRow>
        <Text size="xs" c="dimmed">
          {t("schedule.updateOnRestartNote")}
        </Text>
      </Stack>
    </InspectorCard>
  );
}
