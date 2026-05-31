import { Fragment } from "react";
import { useTranslation } from "react-i18next";
import { Box, Center, Divider, Group, Loader, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { BanEntry } from "../../api";
import { InspectorButton, InspectorCard, RefreshButton } from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useConfirm } from "../../hooks/useConfirm";
import type { FriendSource } from "./FriendsTab";

interface Props {
  source: FriendSource | null;
  requests: string[];
  bans: BanEntry[];
  loading: boolean;
  onRefetch: () => void; // 現ソースの再取得（操作後 / ⟳）
}

// ② 統一結果リスト。①で選んだソース種別に応じて行を描画する（検索/フォーカス内も将来ここに出す）。
// 行内ボタン方式: リクエスト行→[承認]（即時）、BAN行→[解除]（確認）。
export function ResultList({ source, requests, bans, loading, onRefetch }: Props) {
  const { t } = useTranslation();
  const accept = useAsyncAction(onRefetch); // 承認は内向き操作なので即時
  const confirm = useConfirm();

  const title =
    source === "requests"
      ? `${t("friends.requests")} (${requests.length})`
      : source === "bans"
        ? `${t("friends.bans")} (${bans.length})`
        : t("friends.result");

  const askUnban = (b: BanEntry) =>
    confirm.ask({
      title: t("friends.unbanTitle"),
      message: t("friends.confirmUnban", { user: b.username, userId: b.userId }),
      danger: true,
      onConfirm: async () => {
        await api.unban(b.userId);
        onRefetch();
      },
    });

  return (
    <>
      <InspectorCard
        title={title}
        actions={
          source !== null ? (
            <RefreshButton onClick={onRefetch} loading={loading} label={t("friends.refresh")} />
          ) : undefined
        }
      >
        {loading ? (
          <Center py="md">
            <Loader size="sm" />
          </Center>
        ) : source === null ? (
          <Text c="dimmed" size="sm" ta="center" py="md">
            {t("friends.selectSource")}
          </Text>
        ) : source === "requests" ? (
          <RequestsBody requests={requests} busy={accept.busy} onAccept={(name) => void accept.run(() => api.acceptFriendRequest(name))} />
        ) : (
          <BansBody bans={bans} onUnban={askUnban} />
        )}
      </InspectorCard>

      <ConfirmModal
        opened={confirm.request !== null}
        title={confirm.request?.title ?? ""}
        message={confirm.request?.message}
        danger={confirm.request?.danger}
        loading={confirm.busy}
        onConfirm={() => void confirm.confirm()}
        onClose={confirm.close}
      />
    </>
  );
}

function RequestsBody({
  requests,
  busy,
  onAccept,
}: {
  requests: string[];
  busy: boolean;
  onAccept: (name: string) => void;
}) {
  const { t } = useTranslation();
  if (requests.length === 0) {
    return (
      <Text c="dimmed" size="sm" ta="center" py="md">
        {t("friends.noRequests")}
      </Text>
    );
  }
  return (
    <Stack gap="xs">
      {requests.map((name, i) => (
        // 同名 pending が来ても衝突しないよう name+index で一意化。
        <Fragment key={`${name}#${i}`}>
          {i > 0 && <Divider color="dark.5" />}
          <Group justify="space-between" wrap="nowrap" gap="xs">
            <Text fw={600} truncate>
              {name}
            </Text>
            <InspectorButton severity="neutral" disabled={busy} onClick={() => onAccept(name)}>
              {t("friends.accept")}
            </InspectorButton>
          </Group>
        </Fragment>
      ))}
    </Stack>
  );
}

function BansBody({ bans, onUnban }: { bans: BanEntry[]; onUnban: (b: BanEntry) => void }) {
  const { t } = useTranslation();
  if (bans.length === 0) {
    return (
      <Text c="dimmed" size="sm" ta="center" py="md">
        {t("friends.noBans")}
      </Text>
    );
  }
  return (
    <Stack gap="xs">
      {bans.map((b, i) => (
        <Fragment key={b.userId || `${b.username}#${i}`}>
          {i > 0 && <Divider color="dark.5" />}
          <Box>
            {/* 1行目: 🚫 名前 + [解除]（危険・確認あり） */}
            <Group justify="space-between" wrap="nowrap" gap="xs">
              <Text fw={600} truncate>
                🚫 {b.username}
              </Text>
              <InspectorButton severity="danger" onClick={() => onUnban(b)}>
                {t("friends.unban")}
              </InspectorButton>
            </Group>
            {/* 2行目: userId · Machine（dimmed・省略表示） */}
            <Text size="xs" c="dimmed" truncate>
              {b.userId}
              {b.machineIds.length > 0 ? ` · ${t("friends.machine")}: ${b.machineIds.join(" ")}` : ""}
            </Text>
          </Box>
        </Fragment>
      ))}
    </Stack>
  );
}
