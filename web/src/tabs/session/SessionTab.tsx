import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Button, Center, Loader, ScrollArea, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { UserInfo, WorldStatus } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { SessionSettings } from "./SessionSettings";
import { SessionUsers } from "./SessionUsers";

// セッションタブ（フォーカス中 idx）。docs §3.3 #1 / §3.4。
// 取得 = status + users（各エンドポイントが内部で focus idx → cmd）。
// イベント駆動（マウント時 / focusedIdx 変更 / 操作後 / 手動更新）。
// 自動 poll・Page Visibility・トーストは 7-7（仕上げ）。
export function SessionTab({ idx }: { idx: number }) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<WorldStatus | null>(null);
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [loading, setLoading] = useState(true);

  const refetch = useCallback(async () => {
    setLoading(true);
    // B1: status + users を1回の取得（focus 1回）で。一貫スナップショット。
    const d = await api.getSessionDetail(idx);
    if (d) {
      setStatus(d.status);
      setUsers(d.users);
    } else {
      // M3: 取得失敗でも表示中データは消さない（初回 status=null のときだけエラー画面）。
      // ユーザー向けの失敗通知は 7-7（トースト）で扱う。
      console.warn("session detail refetch failed");
    }
    setLoading(false);
  }, [idx]);

  useEffect(() => {
    void refetch();
  }, [refetch]);

  // データが無いのは初回（または初回失敗）だけ。データがあれば refetch 失敗時も内容を表示し続ける。
  if (!status) {
    return (
      <Center h="100%">
        {loading ? (
          <Loader />
        ) : (
          <Stack align="center" gap="sm">
            <Text c="dimmed">{t("session.loadError")}</Text>
            <Button onClick={() => void refetch()} loading={loading}>
              {t("session.refresh")}
            </Button>
          </Stack>
        )}
      </Center>
    );
  }

  return (
    <ScrollArea h="100%" type="hover">
      {/* xl 未満=1カラム / xl 以上=設定(左)・ユーザー(右)の2カラム。両パネル560固定。 */}
      <Box pb="md">
        <SplitColumns
          left={<SessionSettings idx={idx} status={status} onChanged={refetch} refreshing={loading} />}
          right={<SessionUsers idx={idx} users={users} onChanged={refetch} />}
        />
      </Box>
    </ScrollArea>
  );
}
