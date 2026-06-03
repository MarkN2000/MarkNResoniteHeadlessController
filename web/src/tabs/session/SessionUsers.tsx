import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Divider, Group, Stack, Text } from "@mantine/core";
import * as api from "../../api";
import type { UserInfo } from "../../api";
import { InspectorButton, InspectorCard, InspectorSelect } from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { MessageModal } from "../../components/MessageModal";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useConfirm } from "../../hooks/useConfirm";

interface Props {
  idx: number;
  users: UserInfo[];
  onChanged: () => void; // 操作後の refetch（方針A）
  selfUserId: string | null; // ヘッドレス自身(ホスト)の UserID（status.loginUserId）。自分への危険操作を無効化（R3）
}

// 確認が要るユーザー操作の種別と、その表示・危険度・実行 API を1か所にまとめる。
type ConfirmKind = "kick" | "ban" | "respawn" | "silence" | "unsilence";
const CONFIRM_ACTIONS: Record<
  ConfirmKind,
  {
    titleKey: string;
    msgKey: string;
    danger: boolean;
    successKey: string;
    fn: (idx: number, user: string) => Promise<unknown>;
  }
> = {
  kick: { titleKey: "session.kick", msgKey: "session.confirmKick", danger: true, successKey: "toast.kickDone", fn: api.kickUser },
  ban: { titleKey: "session.ban", msgKey: "session.confirmBan", danger: true, successKey: "toast.banDone", fn: api.banUser },
  respawn: { titleKey: "session.respawn", msgKey: "session.confirmRespawn", danger: false, successKey: "toast.respawnDone", fn: api.respawnUser },
  silence: { titleKey: "session.silence", msgKey: "session.confirmSilence", danger: false, successKey: "toast.silenceDone", fn: api.silenceUser },
  unsilence: { titleKey: "session.unsilence", msgKey: "session.confirmUnsilence", danger: false, successKey: "toast.unsilenceDone", fn: api.unsilenceUser },
};

// ユーザー一覧（案B: 2行コンパクト）。
//   情報行 = 状態ドット + 名前（左） / 権限ドロップダウン + 在席離席（右）
//   操作行 = リスポーン/ミュート/メッセージ（中立）+ キック/BAN（危険・右に分離）
// 権限は選択した瞬間に即適用。respawn/silence/kick/ban は確認、message は入力モーダル。
export function SessionUsers({ idx, users, onChanged, selfUserId }: Props) {
  const { t } = useTranslation();
  const { busy, run } = useAsyncAction(onChanged); // 権限の即適用用（メッセージ送信は MessageModal 内）
  const confirm = useConfirm();
  const [msgTo, setMsgTo] = useState<string | null>(null);

  // 確認が要る操作（respawn/silence/kick/ban）を共通ダイアログに乗せる。
  const askConfirm = (kind: ConfirmKind, user: string) => {
    const a = CONFIRM_ACTIONS[kind];
    confirm.ask({
      title: t(a.titleKey),
      message: t(a.msgKey, { user }),
      danger: a.danger,
      success: t(a.successKey),
      onConfirm: async () => {
        const r = await a.fn(idx, user);
        onChanged();
        return r;
      },
    });
  };

  return (
    <>
      {/* セッション設定カードと同じヘッダ（グレー帯＋黄文字中央）で統一。2カラム時に左右で揃う。 */}
      <InspectorCard title={`${t("session.users")} (${users.length})`}>
        {users.length === 0 ? (
          <Text c="dimmed" size="sm" ta="center">
            {t("session.noUsers")}
          </Text>
        ) : (
          <Stack gap="xs">
            {users.map((u, i) => (
              // L1: id は無アカウント時に空になり得るため、空なら名前+indexで一意化（同名匿名の衝突防止）。
              <Fragment key={u.id || `${u.name}#${i}`}>
                {i > 0 && <Divider color="dark.5" />}
                <UserCard
                  idx={idx}
                  u={u}
                  busy={busy}
                  isSelf={!!selfUserId && u.id === selfUserId}
                  onConfirm={(kind) => askConfirm(kind, u.name)}
                  onMessage={() => setMsgTo(u.name)}
                  onRun={run}
                />
              </Fragment>
            ))}
          </Stack>
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

      <MessageModal idx={idx} target={msgTo} onClose={() => setMsgTo(null)} onSent={onChanged} />
    </>
  );
}

function UserCard({
  idx,
  u,
  busy,
  isSelf,
  onConfirm,
  onMessage,
  onRun,
}: {
  idx: number;
  u: UserInfo;
  busy: boolean;
  isSelf: boolean; // 自分(ホスト)＝危険操作/権限変更を無効化し respawn+message のみ許可（R3）
  onConfirm: (kind: ConfirmKind) => void; // 確認モーダルを開く（respawn/silence/unsilence/kick/ban）
  onMessage: () => void;
  onRun: (fn: () => Promise<unknown>, success?: string) => void; // 即適用（権限）
}) {
  const { t } = useTranslation();
  const roleValue = api.ROLES.includes(u.role as (typeof api.ROLES)[number]) ? u.role : null;
  return (
    <Box>
      {/* 情報行: 状態ドット + 名前（左） / 権限 + 在席離席（右） */}
      <Group justify="space-between" wrap="nowrap" gap="xs" mb={6}>
        <Group gap={6} wrap="nowrap" style={{ minWidth: 0 }}>
          <Box
            style={{
              width: 8,
              height: 8,
              borderRadius: "50%",
              flexShrink: 0,
              backgroundColor: u.present ? "var(--mantine-color-green-6)" : "var(--mantine-color-dark-3)",
            }}
          />
          <Text fw={600} truncate>
            {u.name}
          </Text>
          {u.silenced && (
            // L2: 絵文字アイコンに role/aria を付与（スクリーンリーダーで「ミュート中」と読む）。
            <Text size="sm" role="img" aria-label={t("session.silenced")} title={t("session.silenced")}>
              🔇
            </Text>
          )}
        </Group>
        <Group gap="xs" wrap="nowrap" style={{ flexShrink: 0 }}>
          <InspectorSelect
            aria-label={t("session.role")}
            w={132}
            disabled={isSelf} // 自分(ホスト)の権限は変更不可（R3）
            data={[...api.ROLES]}
            value={roleValue}
            placeholder={u.role}
            onChange={(v) => v && v !== u.role && onRun(() => api.setUserRole(idx, u.name, v), t("toast.roleDone"))}
          />
          <Text size="xs" c={u.present ? "green.5" : "dimmed"} style={{ width: 30, textAlign: "right" }}>
            {u.present ? t("session.present") : t("session.away")}
          </Text>
        </Group>
      </Group>

      {/* 操作行: 中立（左）/ 危険（右）。モバイルで崩れないよう wrap で折り返す。 */}
      <Group justify="space-between" gap="xs" wrap="wrap">
        <Group gap={4} wrap="wrap">
          <InspectorButton severity="neutral" disabled={busy} onClick={() => onConfirm("respawn")}>
            ↻ {t("session.respawn")}
          </InspectorButton>
          <InspectorButton
            severity="neutral"
            disabled={busy || isSelf}
            onClick={() => onConfirm(u.silenced ? "unsilence" : "silence")}
          >
            {u.silenced ? `🔈 ${t("session.unsilence")}` : `🔇 ${t("session.silence")}`}
          </InspectorButton>
          <InspectorButton severity="neutral" disabled={busy} onClick={onMessage}>
            ✉ {t("session.message")}
          </InspectorButton>
        </Group>
        <Group gap={4} wrap="nowrap">
          <InspectorButton severity="danger" disabled={busy || isSelf} onClick={() => onConfirm("kick")}>
            {t("session.kick")}
          </InspectorButton>
          <InspectorButton severity="danger" disabled={busy || isSelf} onClick={() => onConfirm("ban")}>
            {t("session.ban")}
          </InspectorButton>
        </Group>
      </Group>
    </Box>
  );
}
