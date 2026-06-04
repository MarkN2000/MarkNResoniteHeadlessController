import { Fragment, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { Avatar, Box, Center, Divider, Group, Loader, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { BanEntry } from "../../api";
import { InspectorButton, InspectorCard, RefreshButton } from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { MessageModal } from "../../components/MessageModal";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useConfirm } from "../../hooks/useConfirm";
import type { FriendSource, UserRow } from "./FriendsTab";

interface Props {
  idx: number;
  source: FriendSource | null;
  rows: UserRow[]; // requests/focused/search の正規化済み行（bans 以外を統一）
  bans: BanEntry[];
  selfUserId: string | null; // ヘッドレス自身(ホスト)の UserID。自分への申請/解除/招待/モデレーションを無効化（R2/R1）
  loading: boolean;
  onRefetch: () => void; // 現ソースの再取得（操作後 / ⟳）
}

// ソース別に「有効」な操作。グレーアウト方式なので全行で同じボタンを並べ、ここに無いものは disabled で出す。
//   ban フラグは BAN / BAN解除 の両方を司る（id 必須＝id 無の行は別途自動グレー）。
type ActionFlags = {
  accept?: boolean;
  sendRequest?: boolean;
  invite?: boolean;
  removeFriend?: boolean;
  message?: boolean;
  ban?: boolean;
};
const ENABLED: Record<"search" | "focused" | "requests", ActionFlags> = {
  search: { sendRequest: true, invite: true, removeFriend: true, message: true, ban: true },
  focused: { sendRequest: true, removeFriend: true, message: true, ban: true }, // 招待=在席中のため無効
  requests: { accept: true, invite: true, message: true }, // 申請/解除/BAN系は無効（リクエスト文脈に不適）
};

// ② 統一結果リスト。①で選んだソースに応じて行を描画する（行内ボタン方式）。
//   bans → 専用表示(BansBody)。それ以外 → 統一ユーザー行(UsersBody)＋ソース別 ENABLED でグレーアウト。
export function ResultList({ idx, source, rows, bans, selfUserId, loading, onRefetch }: Props) {
  const { t } = useTranslation();
  const accept = useAsyncAction(onRefetch); // 承認は内向き操作なので即時＋再取得（リストから消える）
  const confirm = useConfirm();
  const [msgTo, setMsgTo] = useState<string | null>(null); // メッセージ入力モーダルの宛先（username）

  const count = source === "bans" ? bans.length : rows.length;
  const title =
    source === "requests"
      ? `${t("friends.requests")} (${count})`
      : source === "bans"
        ? `${t("friends.bans")} (${count})`
        : source === "search"
          ? `${t("friends.searchResults")} (${count})`
          : source === "focused"
            ? `${t("friends.focusedUsers")} (${count})`
            : t("friends.result");

  // unban（BAN一覧）は操作で項目が消えるので確認 → 実行 → 再取得。メッセージは名前+userId。
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

  // 関係操作（申請/解除/招待）は外向きなので確認するが、統一リストは友達関係を表示しないため
  // **操作後 refetch しない**（リストは変化しない・手動 ⟳ で更新）。fn の結果を返すと confirm() がトーストを出す。
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

  // モデレーション操作（R1）。ban/unban は userId 駆動なので UserRow を受け取る。
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

  let body: ReactNode;
  if (loading) {
    body = (
      <Center py="md">
        <Loader size="sm" />
      </Center>
    );
  } else if (source === null) {
    body = <Empty text={t("friends.selectSource")} />;
  } else if (source === "bans") {
    body = <BansBody bans={bans} onUnban={askUnban} />;
  } else {
    const emptyText =
      source === "requests"
        ? t("friends.noRequests")
        : source === "focused"
          ? t("friends.noFocusedUsers")
          : t("friends.noResults");
    body = (
      <UsersBody
        rows={rows}
        enabled={ENABLED[source]}
        emptyText={emptyText}
        selfUserId={selfUserId}
        acceptBusy={accept.busy}
        onAccept={(name) => void accept.run(() => api.acceptFriendRequest(name), t("toast.acceptDone"))}
        onSendRequest={askSendRequest}
        onRemoveFriend={askRemoveFriend}
        onInvite={askInvite}
        onMessage={(u) => setMsgTo(u)}
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

      <ConfirmHost confirm={confirm} />

      <MessageModal idx={idx} target={msgTo} onClose={() => setMsgTo(null)} />
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

// 統一ユーザー行: アバター(icon/頭文字)＋名前＋id ＋ 固定ボタン列。
//   関係行 = 承認 / 申請 / 招待 / 解除 ・ モデレーション行 = ✉メッセージ / BAN / BAN解除。
// 全行で同じ並びを描画し、enabled 外・自分(ホスト)・id 無 は disabled（グレーアウト）で見た目を統一する。
function UsersBody({
  rows,
  enabled,
  emptyText,
  selfUserId,
  acceptBusy,
  onAccept,
  onSendRequest,
  onRemoveFriend,
  onInvite,
  onMessage,
  onBan,
  onUnban,
}: {
  rows: UserRow[];
  enabled: ActionFlags;
  emptyText: string;
  selfUserId: string | null;
  acceptBusy: boolean;
  onAccept: (username: string) => void;
  onSendRequest: (username: string) => void;
  onRemoveFriend: (username: string) => void;
  onInvite: (username: string) => void;
  onMessage: (username: string) => void;
  onBan: (u: UserRow) => void; // banByID は userId 必須なので行ごと渡す
  onUnban: (u: UserRow) => void; // unbanByID も userId 必須
}) {
  const { t } = useTranslation();
  if (rows.length === 0) return <Empty text={emptyText} />;
  return (
    <Stack gap="xs">
      {rows.map((u, i) => {
        const isSelf = !!selfUserId && u.id === selfUserId; // 自分への申請/解除/招待/モデレーションは無効化
        const noId = !u.id; // id 必須の BAN/BAN解除 を自動グレー
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
              {/* 関係操作行: 承認 / 申請 / 招待 / 解除 */}
              <Group gap={4} wrap="wrap">
                <InspectorButton severity="neutral" disabled={!enabled.accept || acceptBusy} onClick={() => onAccept(u.username)}>
                  {t("friends.accept")}
                </InspectorButton>
                <InspectorButton severity="neutral" disabled={!enabled.sendRequest || isSelf} onClick={() => onSendRequest(u.username)}>
                  {t("friends.sendRequest")}
                </InspectorButton>
                <InspectorButton severity="neutral" disabled={!enabled.invite || isSelf} onClick={() => onInvite(u.username)}>
                  {t("friends.invite")}
                </InspectorButton>
                <InspectorButton severity="danger" disabled={!enabled.removeFriend || isSelf} onClick={() => onRemoveFriend(u.username)}>
                  {t("friends.removeFriend")}
                </InspectorButton>
              </Group>
              {/* モデレーション行: ✉メッセージ / BAN / BAN解除 */}
              <Group gap={4} wrap="wrap" mt={4}>
                <InspectorButton severity="neutral" disabled={!enabled.message || isSelf} onClick={() => onMessage(u.username)}>
                  ✉ {t("friends.message")}
                </InspectorButton>
                <InspectorButton severity="danger" disabled={!enabled.ban || isSelf || noId} onClick={() => onBan(u)}>
                  {t("friends.ban")}
                </InspectorButton>
                <InspectorButton severity="danger" disabled={!enabled.ban || isSelf || noId} onClick={() => onUnban(u)}>
                  {t("friends.banUnban")}
                </InspectorButton>
              </Group>
            </Box>
          </Fragment>
        );
      })}
    </Stack>
  );
}
