import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Button, Center, Loader, ScrollArea, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { ItemTemplate, UserInfo, World, WorldStatus } from "../../api";
import { PANEL_WIDTH, SplitColumns } from "../../components/SplitColumns";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useConfirm } from "../../hooks/useConfirm";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { SessionList } from "./SessionList";
import { SessionSettings } from "./SessionSettings";
import { SessionUsers } from "./SessionUsers";
import { SpawnImpulseCard } from "./SpawnImpulseCard";

// 詳細はフォーカス中の1件だけを10秒間隔、一覧は軽量な worlds を15秒間隔で更新する。
// どちらも Page Visibility 連動で、画面外では停止する。
const DETAIL_POLL_INTERVAL_MS = 10_000;
const LIST_POLL_INTERVAL_MS = 15_000;
const CONTENT_MAX_WIDTH = PANEL_WIDTH * 2 + 24;

interface Props {
  idx: number;
  sessions: World[];
  selfUserId: string | null;
  templates: ItemTemplate[];
  onFocus: (idx: number) => void;
  onRefreshSessions: () => void | Promise<void>;
  onSessionClosed: () => void | Promise<void>;
  onOpenNewSession: () => void;
}

interface DetailState {
  idx: number;
  status: WorldStatus;
  users: UserInfo[];
}

export function SessionTab({
  idx,
  sessions,
  selfUserId,
  templates,
  onFocus,
  onRefreshSessions,
  onSessionClosed,
  onOpenNewSession,
}: Props) {
  const { t } = useTranslation();
  const [detail, setDetail] = useState<DetailState | null>(null);
  const [loading, setLoading] = useState(true);
  const [listRefreshing, setListRefreshing] = useState(false);
  const [detailRevision, setDetailRevision] = useState(0);
  const [settingsDirty, setSettingsDirty] = useState(false);
  const detailRequestId = useRef(0);
  const switchConfirm = useConfirm();
  const hasSelected = sessions.some((session) => session.index === idx);
  const selected = sessions.find((session) => session.index === idx) ?? null;

  const refreshList = useCallback(
    async (opts?: { silent?: boolean }) => {
      if (!opts?.silent) setListRefreshing(true);
      try {
        await onRefreshSessions();
      } finally {
        if (!opts?.silent) setListRefreshing(false);
      }
    },
    [onRefreshSessions],
  );

  // タブへ戻った時点で一覧を再取得し、以後は表示中のみ15秒poll。
  useEffect(() => {
    void refreshList();
  }, [refreshList]);
  useVisiblePolling(() => refreshList({ silent: true }), LIST_POLL_INTERVAL_MS);

  const refetch = useCallback(
    async (opts?: { silent?: boolean }) => {
      const requestId = ++detailRequestId.current;
      if (!opts?.silent) setLoading(true);
      if (!hasSelected) {
        if (requestId === detailRequestId.current) setLoading(false);
        return;
      }

      const next = await api.getSessionDetail(idx);
      // 高速切替やpoll重複で古い応答が後着しても、現在の詳細を上書きさせない。
      if (requestId !== detailRequestId.current) return;
      if (next) {
        setDetail({ idx, status: next.status, users: next.users });
      } else {
        // 同じセッションの一時失敗では取得済みデータを維持する。別idxの古い詳細は下で表示しない。
        console.warn("session detail refetch failed");
      }
      if (!opts?.silent) setLoading(false);
    },
    [detailRevision, hasSelected, idx],
  );

  useEffect(() => {
    void refetch();
  }, [refetch]);
  useVisiblePolling(() => refetch({ silent: true }), DETAIL_POLL_INTERVAL_MS);

  const status = hasSelected && detail?.idx === idx ? detail.status : null;
  const users = hasSelected && detail?.idx === idx ? detail.users : [];

  const commitFocus = useCallback(
    (nextIdx: number) => {
      if (nextIdx === idx) return;
      detailRequestId.current += 1;
      setSettingsDirty(false);
      setDetail(null);
      setLoading(true);
      onFocus(nextIdx);
    },
    [idx, onFocus],
  );

  const handleFocus = useCallback(
    (nextIdx: number) => {
      if (nextIdx === idx) return;
      if (!settingsDirty) {
        commitFocus(nextIdx);
        return;
      }
      switchConfirm.ask({
        title: t("session.discardTitle"),
        message: t("session.discardMessage"),
        danger: true,
        onConfirm: () => commitFocus(nextIdx),
      });
    },
    [commitFocus, idx, settingsDirty, switchConfirm, t],
  );

  const handleSessionClosed = useCallback(async () => {
    detailRequestId.current += 1;
    setDetail(null);
    await onSessionClosed();
    // 先頭のindexが閉じた場合はidx値が同じまま別セッションになるため、一覧更新と同じcommitで再取得を起動する。
    setDetailRevision((current) => current + 1);
  }, [onSessionClosed]);

  return (
    <ScrollArea h="100%" type="hover">
      <Stack gap="lg" pb="md">
        <Box w="100%" maw={CONTENT_MAX_WIDTH} mx="auto">
          <SessionList
            sessions={sessions}
            focusedIdx={idx}
            refreshing={listRefreshing}
            onFocus={handleFocus}
            onRefresh={() => void refreshList()}
            onOpenNewSession={onOpenNewSession}
          />
        </Box>

        {!selected ? null : !status ? (
          <Center mih={180}>
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
        ) : (
          <>
            {/* 一覧より下は既存のインスペクタ構造と左右配置を維持する。 */}
            <SplitColumns
              left={
                <Stack gap="lg">
                  <SessionSettings
                    idx={idx}
                    status={status}
                    onChanged={refetch}
                    onSessionsChanged={() => refreshList({ silent: true })}
                    onClosed={handleSessionClosed}
                    onDirtyChange={setSettingsDirty}
                    refreshing={loading}
                  />
                  <SpawnImpulseCard idx={idx} templates={templates} />
                </Stack>
              }
              right={<SessionUsers idx={idx} users={users} onChanged={refetch} selfUserId={selfUserId} />}
            />
          </>
        )}
      </Stack>
      <ConfirmHost confirm={switchConfirm} />
    </ScrollArea>
  );
}
