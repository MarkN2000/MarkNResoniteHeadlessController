import { useTranslation } from "react-i18next";
import { Box, Group, Stack, Text } from "@mantine/core";
import { FieldRow, InspectorButton, InspectorCard, InspectorTextInput } from "../../components/inspector";

// ワールド検索 → 検索結果から起動する枠（将来対応）。今はレイアウト予約のための
// disabled プレースホルダ（ロジック無し）。有効化には DESIGN §Won't のワールド検索
// （go.resonite.com スクレイピング or 代替検索ソース）の判断が必要。起動自体は将来も
// 既存 URL モード（startWorldURL に resrec:// を渡す）を流用できる。phase-7-spec §3.12。
export function WorldSearchPanel() {
  const { t } = useTranslation();
  return (
    <InspectorCard title={t("newSession.searchTitle")}>
      <Stack gap={10}>
        <FieldRow label={t("newSession.keyword")}>
          <Group gap="xs" wrap="nowrap">
            <InspectorTextInput
              disabled
              placeholder={t("newSession.keywordPlaceholder")}
              style={{ flex: 1, minWidth: 0 }}
            />
            <InspectorButton disabled>{t("newSession.search")}</InspectorButton>
          </Group>
        </FieldRow>

        <Text size="xs" c="dimmed" ta="center">
          {t("newSession.searchSoon")}
        </Text>

        {/* 将来の結果グリッド（サムネ＋名前）の形を示すグレーのスケルトン2枚。 */}
        <Group gap="sm" grow align="flex-start">
          {[0, 1].map((i) => (
            <Stack key={i} gap={6} style={{ opacity: 0.4 }}>
              <Box h={80} style={{ backgroundColor: "var(--mantine-color-dark-5)", borderRadius: 8 }} />
              <Box h={10} w="70%" style={{ backgroundColor: "var(--mantine-color-dark-5)", borderRadius: 4 }} />
              <Box h={8} w="45%" style={{ backgroundColor: "var(--mantine-color-dark-6)", borderRadius: 4 }} />
            </Stack>
          ))}
        </Group>
      </Stack>
    </InspectorCard>
  );
}
