import { Center, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { InspectorCard } from "../../components/inspector";

// DepotDownloader（Steam 経由の Resonite 更新）設定の将来枠。今は disabled プレースホルダのみ（P9-B）。
// 7-2/7-3 と同じ「枠を今から予約」手法で、将来実装してもレイアウトが変わらないようにする。
export function SteamSection() {
  const { t } = useTranslation();
  return (
    <InspectorCard title={t("settings.steamSection")}>
      <Center h={56}>
        <Text size="sm" c="dimmed">
          {t("settings.steamSoon")}
        </Text>
      </Center>
    </InspectorCard>
  );
}
