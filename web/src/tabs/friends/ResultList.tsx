import { Fragment, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Avatar, Box, Center, Divider, Group, Loader, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { BanEntry, ResoniteUser, UserInfo } from "../../api";
import { InspectorButton, InspectorCard, RefreshButton } from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useConfirm } from "../../hooks/useConfirm";
import type { FriendSource } from "./FriendsTab";

interface Props {
  idx: number;
  source: FriendSource | null;
  requests: string[];
  bans: BanEntry[];
  searchResults: ResoniteUser[];
  focusedUsers: UserInfo[];
  loading: boolean;
  onRefetch: () => void; // 現ソースの再取得（操作後 / ⟳）
}

// 統一リストで扱うユーザー行の共通形（検索結果=ResoniteUser / フォーカス内=UserInfo を正規化）。
interface UserRow {
  id: string;
  username: string;
  iconUrl?: string;
}

// ② 統一結果リスト。①で選んだソース種別に応じて行を描画する（行内ボタン方式）。
//   requests→[承認](即時) / bans→[解除](確認) / search・focused→[申請][解除](+search時[招待])（確認）。
export function ResultList({ idx, source, requests, bans, searchResults, focusedUsers, loading, onRefetch }: Props) {
  const { t } = useTranslation();
  const accept = useAsyncAction(onRefetch); // 承認は内向き操作なので即時
  const confirm = useConfirm();

  const title =
    source === "requests"
      ? `${t("friends.requests")} (${requests.length})`
      : source === "bans"
        ? `${t("friends.bans")} (${bans.length})`
        : source === "search"
          ? `${t("friends.searchResults")} (${searchResults.length})`
          : source === "focused"
            ? `${t("friends.focusedUsers")} (${focusedUsers.length})`
            : t("friends.result");

  // unban は操作で項目がリストから消えるので確認 → 実行 → 再取得。メッセージは名前+userId。
  // 結果を return することで confirm() がトーストを出す（失敗=赤 / 成功=success 緑）。
  const askUnban = (b: BanEntry) =>
    confirm.ask({
      title: t("friends.unbanTitle"),
      message: t("friends.confirmUnban", { user: b.username, userId: b.userId }),
      danger: true,
      success: t("toast.unbanDone"),
      onConfirm: async () => {
        const r = await api.unban(b.userId);
        onRefetch();
        return r;
      },
    });

  // 関係操作（申請/解除/招待）は外向きなので確認するが、search/focused のリストは
  // 友達関係を表示しないため**操作後 refetch しない**（リストは変化しない・手動 ⟳ で更新）。
  // fn の結果を返すと confirm() が成功/失敗トーストを出す。
  const askUserOp = (
    labelKey: string,
    msgKey: string,
    danger: boolean,
    username: string,
    successKey: string,
    fn: () => Promise<unknown>,
  ) =>
    confirm.ask({
      title: t(labelKey),
      message: t(msgKey, { user: username }),
      danger,
      success: t(successKey),
      onConfirm: () => fn(),
    });

  const askSendRequest = (u: string) =>
    askUserOp("friends.sendRequest", "friends.confirmSendRequest", false, u, "toast.sendRequestDone", () =>
      api.sendFriendRequest(u),
    );
  const askRemoveFriend = (u: string) =>
    askUserOp("friends.removeFriend", "friends.confirmRemoveFriend", true, u, "toast.removeFriendDone", () =>
      api.removeFriend(u),
    );
  const askInvite = (u: string) =>
    askUserOp("friends.invite", "friends.confirmInvite", false, u, "toast.inviteDone", () => api.inviteUser(idx, u));

  let body: ReactNode;
  if (loading) {
    body = (
      <Center py="md">
        <Loader size="sm" />
      </Center>
    );
  } else if (source === null) {
    body = <Empty text={t("friends.selectSource")} />;
  } else if (source === "requests") {
    body = (
      <RequestsBody
        requests={requests}
        busy={accept.busy}
        onAccept={(name) => void accept.run(() => api.acceptFriendRequest(name), t("toast.acceptDone"))}
      />
    );
  } else if (source === "bans") {
    body = <BansBody bans={bans} onUnban={askUnban} />;
  } else {
    // search / focused を共通の UserRow に正規化して UsersBody で描画。
    const users: UserRow[] =
      source === "search" ? searchResults : focusedUsers.map((u) => ({ id: u.id, username: u.name }));
    body = (
      <UsersBody
        users={users}
        showInvite={source === "search"} // 招待は在席者では無意味（実機 ambient のみ）→ search のみ
        emptyText={source === "search" ? t("friends.noResults") : t("friends.noFocusedUsers")}
        onSendRequest={askSendRequest}
        onRemoveFriend={askRemoveFriend}
        onInvite={askInvite}
      />
    );
  }

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
        {body}
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

function Empty({ text }: { text: string }) {
  return (
    <Text c="dimmed" size="sm" ta="center" py="md">
      {text}
    </Text>
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
  if (requests.length === 0) return <Empty text={t("friends.noRequests")} />;
  return (
    <Stack gap="xs">
      {requests.map((name, i) => (
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
  if (bans.length === 0) return <Empty text={t("friends.noBans")} />;
  return (
    <Stack gap="xs">
      {bans.map((b, i) => (
        <Fragment key={b.userId || `${b.username}#${i}`}>
          {i > 0 && <Divider color="dark.5" />}
          <Box>
            <Group justify="space-between" wrap="nowrap" gap="xs">
              <Text fw={600} truncate>
                {/* 🚫 は装飾（BAN一覧という文脈＋ユーザー名で冗長）なのでスクリーンリーダーから隠す。 */}
                <span aria-hidden="true">🚫</span> {b.username}
              </Text>
              <InspectorButton severity="danger" onClick={() => onUnban(b)}>
                {t("friends.unban")}
              </InspectorButton>
            </Group>
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

// 検索結果 / フォーカス内ユーザーの共通描画。情報行（アバター＋名前＋id）＋操作行（申請/招待/解除）。
function UsersBody({
  users,
  showInvite,
  emptyText,
  onSendRequest,
  onRemoveFriend,
  onInvite,
}: {
  users: UserRow[];
  showInvite: boolean;
  emptyText: string;
  onSendRequest: (username: string) => void;
  onRemoveFriend: (username: string) => void;
  onInvite: (username: string) => void;
}) {
  const { t } = useTranslation();
  if (users.length === 0) return <Empty text={emptyText} />;
  return (
    <Stack gap="xs">
      {users.map((u, i) => (
        <Fragment key={u.id || `${u.username}#${i}`}>
          {i > 0 && <Divider color="dark.5" />}
          <Box>
            {/* 情報行: アバター + 名前 + id */}
            <Group wrap="nowrap" gap="xs" mb={6}>
              <Avatar src={u.iconUrl || undefined} radius="xl" size={32}>
                {(u.username || "?").slice(0, 1).toUpperCase()}
              </Avatar>
              <Box style={{ minWidth: 0 }}>
                <Text fw={600} truncate>
                  {u.username || t("friends.unknownUser")}
                </Text>
                {u.id && (
                  <Text size="xs" c="dimmed" truncate>
                    {u.id}
                  </Text>
                )}
              </Box>
            </Group>
            {/* 操作行: 申請 / 招待 / 解除（モバイルで折返し）。 */}
            <Group gap={4} wrap="wrap">
              <InspectorButton severity="neutral" onClick={() => onSendRequest(u.username)}>
                {t("friends.sendRequest")}
              </InspectorButton>
              {showInvite && (
                <InspectorButton severity="neutral" onClick={() => onInvite(u.username)}>
                  {t("friends.invite")}
                </InspectorButton>
              )}
              <InspectorButton severity="danger" onClick={() => onRemoveFriend(u.username)}>
                {t("friends.removeFriend")}
              </InspectorButton>
            </Group>
          </Box>
        </Fragment>
      ))}
    </Stack>
  );
}
