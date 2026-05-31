import { useTranslation } from "react-i18next";
import { Box, Divider, Group, Stack, Text } from "@mantine/core";
import { InspectorButton, InspectorCard, InspectorTextInput } from "../../components/inspector";
import type { FriendSource } from "./FriendsTab";

interface Props {
  active: FriendSource | null; // 現在表示中のソース（ボタンのハイライト用）
  loading: boolean;
  onLoad: (src: FriendSource) => void;
}

// ① ソース選択パネル。結果は②（ResultList）に集約されるため、ここは「何を取得/検索するか」だけを持つ。
// ユーザー名/ID 検索とフォーカスセッション内は Resonite クラウドAPI 等が要るため P9（今は disabled の枠）。
export function SourcePanel({ active, loading, onLoad }: Props) {
  const { t } = useTranslation();
  return (
    <InspectorCard title={t("friends.source")}>
      <Stack gap="sm">
        {/* 検索（準備中・P9）。器だけ用意して将来 ②に検索結果を出す。 */}
        <Box>
          <Text size="xs" c="dimmed" mb={4}>
            {t("friends.searchByName")}
          </Text>
          <Group gap="xs" wrap="nowrap">
            <Box style={{ flex: 1 }}>
              <InspectorTextInput disabled placeholder={t("friends.searchPlaceholder")} />
            </Box>
            <InspectorButton disabled>{t("friends.search")}</InspectorButton>
          </Group>
        </Box>
        <Box>
          <Text size="xs" c="dimmed" mb={4}>
            {t("friends.searchById")}
          </Text>
          <Group gap="xs" wrap="nowrap" align="center">
            <Text size="sm" c="dimmed">
              U-
            </Text>
            <Box style={{ flex: 1 }}>
              <InspectorTextInput disabled placeholder={t("friends.searchPlaceholder")} />
            </Box>
            <InspectorButton disabled>{t("friends.search")}</InspectorButton>
          </Group>
        </Box>
        <Text size="xs" c="dimmed" ta="center">
          {t("friends.searchSoon")}
        </Text>

        <Divider color="dark.4" />

        {/* 取得（有効）。押したソースだけ②に取得する。現ソースは filled でハイライト。 */}
        <InspectorButton
          fullWidth
          variant={active === "requests" ? "filled" : "light"}
          loading={loading && active === "requests"}
          onClick={() => onLoad("requests")}
        >
          {t("friends.loadRequests")}
        </InspectorButton>
        <InspectorButton
          fullWidth
          variant={active === "bans" ? "filled" : "light"}
          loading={loading && active === "bans"}
          onClick={() => onLoad("bans")}
        >
          {t("friends.loadBans")}
        </InspectorButton>
        {/* フォーカスセッション内ソースは P9（検索と同じく今は枠のみ）。 */}
        <InspectorButton fullWidth disabled>
          {t("friends.loadFocused")}
        </InspectorButton>
      </Stack>
    </InspectorCard>
  );
}
