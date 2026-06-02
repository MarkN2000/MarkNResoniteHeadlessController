import { Fragment, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { Avatar, Box, Button, Center, Divider, Group, Loader, Modal, Stack, Text, Textarea } from "@mantine/core";
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
  selfUserId: string | null; // ヘッドレス自身(ホスト)の UserID。自分への申請/解除/招待を無効化（R2）
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
export function ResultList({ idx, source, requests, bans, searchResults, focusedUsers, selfUserId, loading, onRefetch }: Props) {
  const { t } = useTranslation();
  const accept = useAsyncAction(onRefetch); // 承認は内向き操作なので即時
  const send = useAsyncAction(); // メッセージ送信（検索リストは変化しないので refetch なし・R1）
  const confirm = useConfirm();
  const [msgTo, setMsgTo] = useState<string | null>(null); // メッセージ入力モーダルの宛先（username）
  const [msgText, setMsgText] = useState("");

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

  // モデレーション操作（検索結果・R1）。ban/unban は userId 駆動なので UserRow を受け取る。
  // 検索リストは ban/unban で変化しないため操作後 refetch しない（トーストのみ）。
  const askBan = (u: UserRow) =>
    confirm.ask({
      title: t("friends.banTitle"),
      message: t("friends.confirmBan", { user: u.username, userId: u.id }),
      danger: true,
      success: t("toast.banDone"),
      onConfirm: () => api.banByID(u.id),
    });
  const askUnbanById = (u: UserRow) =>
    confirm.ask({
      title: t("friends.unbanTitle"),
      message: t("friends.confirmUnban", { user: u.username, userId: u.id }),
      danger: true,
      success: t("toast.unbanDone"),
      onConfirm: () => api.unban(u.id),
    });
  // メッセージは入力モーダル（確認ダイアログではなく本文入力・session タブと同方式）。
  const openMessage = (username: string) => {
    setMsgText("");
    setMsgTo(username);
  };

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
        showModeration={source === "search"} // メッセージ/BAN/BAN解除 は検索結果のみ（R1・在席者はセッションタブ）
        emptyText={source === "search" ? t("friends.noResults") : t("friends.noFocusedUsers")}
        selfUserId={selfUserId}
        onSendRequest={askSendRequest}
        onRemoveFriend={askRemoveFriend}
        onInvite={askInvite}
        onMessage={openMessage}
        onBan={askBan}
        onUnban={askUnbanById}
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

      <Modal
        opened={msgTo !== null}
        onClose={() => setMsgTo(null)}
        title={t("session.messageTo", { user: msgTo ?? "" })}
        centered
      >
        <Textarea
          value={msgText}
          onChange={(e) => setMsgText(e.currentTarget.value)}
          placeholder={t("session.messagePlaceholder")}
          autosize
          minRows={3}
        />
        <Group justify="flex-end" gap="xs" mt="md">
          <Button variant="default" onClick={() => setMsgTo(null)}>
            {t("common.cancel")}
          </Button>
          <Button
            loading={send.busy}
            disabled={!msgText.trim()}
            onClick={() => {
              const to = msgTo;
              const text = msgText;
              if (!to) return;
              setMsgTo(null);
              void send.run(() => api.messageUser(idx, to, text), t("toast.messageDone"));
            }}
          >
            {t("session.send")}
          </Button>
        </Group>
      </Modal>
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
  showModeration,
  emptyText,
  selfUserId,
  onSendRequest,
  onRemoveFriend,
  onInvite,
  onMessage,
  onBan,
  onUnban,
}: {
  users: UserRow[];
  showInvite: boolean;
  showModeration: boolean; // メッセージ/BAN/BAN解除 段を出すか（検索結果のみ・R1）
  emptyText: string;
  selfUserId: string | null; // 自分(ホスト)＝申請/解除/招待/モデレーションを無効化（R2/R1・search/focused 共通）
  onSendRequest: (username: string) => void;
  onRemoveFriend: (username: string) => void;
  onInvite: (username: string) => void;
  onMessage: (username: string) => void;
  onBan: (u: UserRow) => void; // banByID は userId 必須なので行ごと渡す
  onUnban: (u: UserRow) => void; // unbanByID も userId 必須
}) {
  const { t } = useTranslation();
  if (users.length === 0) return <Empty text={emptyText} />;
  return (
    <Stack gap="xs">
      {users.map((u, i) => {
        const isSelf = !!selfUserId && u.id === selfUserId; // 自分への申請/解除/招待は無意味→無効化
        return (
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
            {/* 関係操作行: 申請 / 招待 / 解除（モバイルで折返し）。自分(ホスト)は無効化（R2）。 */}
            <Group gap={4} wrap="wrap">
              <InspectorButton severity="neutral" disabled={isSelf} onClick={() => onSendRequest(u.username)}>
                {t("friends.sendRequest")}
              </InspectorButton>
              {showInvite && (
                <InspectorButton severity="neutral" disabled={isSelf} onClick={() => onInvite(u.username)}>
                  {t("friends.invite")}
                </InspectorButton>
              )}
              <InspectorButton severity="danger" disabled={isSelf} onClick={() => onRemoveFriend(u.username)}>
                {t("friends.removeFriend")}
              </InspectorButton>
            </Group>
            {/* モデレーション行（検索結果のみ・R1）: メッセージ / BAN / BAN解除。
                BAN/BAN解除 は userId 必須なので id 空の行は無効化。メッセージは username 駆動。 */}
            {showModeration && (
              <Group gap={4} wrap="wrap" mt={4}>
                <InspectorButton severity="neutral" disabled={isSelf} onClick={() => onMessage(u.username)}>
                  ✉ {t("friends.message")}
                </InspectorButton>
                <InspectorButton severity="danger" disabled={isSelf || !u.id} onClick={() => onBan(u)}>
                  {t("friends.ban")}
                </InspectorButton>
                <InspectorButton severity="danger" disabled={isSelf || !u.id} onClick={() => onUnban(u)}>
                  {t("friends.banUnban")}
                </InspectorButton>
              </Group>
            )}
          </Box>
        </Fragment>
        );
      })}
    </Stack>
  );
}
