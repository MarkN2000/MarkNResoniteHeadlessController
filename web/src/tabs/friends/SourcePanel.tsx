import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Divider, Group, Stack, Text } from "@mantine/core";
import { InspectorButton, InspectorCard, InspectorTextInput } from "../../components/inspector";
import type { FriendSource, LoadSource } from "./FriendsTab";

interface Props {
  active: FriendSource | null; // 現在表示中のソース（ボタンのハイライト用）
  loading: boolean;
  onLoad: (src: LoadSource) => void;
  onSearch: (term: string) => void;
}

// ① ソース選択パネル。結果は②（ResultList）に集約。ここは「何を取得/検索するか」だけを持つ。
// ユーザー検索は Resonite 公開API（無認証）。フォーカス内は現セッションの在席者。
export function SourcePanel({ active, loading, onLoad, onSearch }: Props) {
  const { t } = useTranslation();
  const [nameQ, setNameQ] = useState("");
  const [idQ, setIdQ] = useState("");

  const searchByName = () => {
    const q = nameQ.trim();
    if (q) onSearch(q);
  };
  const searchById = () => {
    // 表示済みの "U-" 接頭辞を二重化しないよう、入力側の U- は剥がしてから付け直す。
    const q = idQ.trim().replace(/^U-/i, "");
    if (q) onSearch("U-" + q);
  };

  return (
    <InspectorCard title={t("friends.source")}>
      <Stack gap="sm">
        {/* ユーザー検索（Resonite 公開API・無認証） */}
        <Box>
          <Text size="xs" c="dimmed" mb={4}>
            {t("friends.searchByName")}
          </Text>
          <Group gap="xs" wrap="nowrap">
            <Box style={{ flex: 1 }}>
              <InspectorTextInput
                value={nameQ}
                onChange={(e) => setNameQ(e.currentTarget.value)}
                onKeyDown={(e) => e.key === "Enter" && searchByName()}
                placeholder={t("friends.searchNamePlaceholder")}
              />
            </Box>
            <InspectorButton onClick={searchByName}>{t("friends.search")}</InspectorButton>
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
              <InspectorTextInput
                value={idQ}
                onChange={(e) => setIdQ(e.currentTarget.value)}
                onKeyDown={(e) => e.key === "Enter" && searchById()}
                placeholder={t("friends.searchIdPlaceholder")}
              />
            </Box>
            <InspectorButton onClick={searchById}>{t("friends.search")}</InspectorButton>
          </Group>
        </Box>

        <Divider color="dark.4" />

        {/* 取得系ソース。押したソースだけ②に取得。現ソースは filled でハイライト。 */}
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
        <InspectorButton
          fullWidth
          variant={active === "focused" ? "filled" : "light"}
          loading={loading && active === "focused"}
          onClick={() => onLoad("focused")}
        >
          {t("friends.loadFocused")}
        </InspectorButton>
      </Stack>
    </InspectorCard>
  );
}
