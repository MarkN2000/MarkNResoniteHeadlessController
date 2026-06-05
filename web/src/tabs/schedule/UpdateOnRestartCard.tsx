import { useTranslation } from "react-i18next";
import { Stack, Switch, Text } from "@mantine/core";
import { InspectorCard, FieldRow } from "../../components/inspector";

// 予定再起動時の Resonite 自動更新トグル（P9-B）。ON かつ Steam 設定済みのとき、
// 予定再起動の停止→起動の間に DepotDownloader で更新する（手動/クラッシュ復帰は対象外）。
// Steam 未設定なら no-op。既定 ON。
export function UpdateOnRestartCard({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  const { t } = useTranslation();
  return (
    <InspectorCard title={t("schedule.updateCard")}>
      <Stack gap="xs">
        <FieldRow label={t("schedule.updateOnRestart")}>
          <Switch checked={value} onChange={(e) => onChange(e.currentTarget.checked)} />
        </FieldRow>
        <Text size="xs" c="dimmed">
          {t("schedule.updateOnRestartNote")}
        </Text>
      </Stack>
    </InspectorCard>
  );
}
