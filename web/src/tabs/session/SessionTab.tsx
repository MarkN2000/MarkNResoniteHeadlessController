import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Button, Center, Loader, ScrollArea, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { UserInfo, WorldStatus } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { SessionSettings } from "./SessionSettings";
import { SessionUsers } from "./SessionUsers";

// セッションタブ（フォーカス中 idx）。docs §3.3 #1 / §3.4。
// 取得 = status + users（各エンドポイントが内部で focus idx → cmd）。
// イベント駆動（マウント時 / focusedIdx 変更 / 操作後 / 手動更新）
// ＋ 表示中のみ 10 秒ごとの背景 poll（ユーザーの参加/退出を追従・Page Visibility 連動・§3.4）。
const POLL_INTERVAL_MS = 10_000;

export function SessionTab({ idx, selfUserId }: { idx: number; selfUserId: string | null }) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<WorldStatus | null>(null);
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [loading, setLoading] = useState(true);

  // silent=true（背景 poll）のときは loading を触らない＝⟳ スピナーを回さず画面をチラつかせない。
  // 手動⟳ / マウント / focus 変更 / 操作後は silent なし＝従来どおりスピナー表示。
  const refetch = useCallback(
    async (opts?: { silent?: boolean }) => {
      if (!opts?.silent) setLoading(true);
      // B1: status + users を1回の取得（focus 1回）で。一貫スナップショット。
      const d = await api.getSessionDetail(idx);
      if (d) {
        setStatus(d.status);
        setUsers(d.users);
      } else {
        // M3: 取得失敗でも表示中データは消さない（初回 status=null のときだけエラー画面）。
        // 背景 poll(silent) の失敗もトーストを出さず黙って維持（10 秒ごとの赤通知を防ぐ）。
        console.warn("session detail refetch failed");
      }
      if (!opts?.silent) setLoading(false);
    },
    [idx],
  );

  useEffect(() => {
    void refetch();
  }, [refetch]);

  // 表示中のみ背景で 10 秒ごとに再取得（非表示で停止/再表示で即取得・unmount で停止）。
  useVisiblePolling(() => refetch({ silent: true }), POLL_INTERVAL_MS);

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
          right={<SessionUsers idx={idx} users={users} onChanged={refetch} selfUserId={selfUserId} />}
        />
      </Box>
    </ScrollArea>
  );
}
