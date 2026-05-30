import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { Box, Button, Divider, Group, Modal, Stack, Text, Textarea } from "@mantine/core";
import * as api from "../../api";
import type { UserInfo } from "../../api";
import { InspectorButton, InspectorCard, InspectorSelect } from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useAsyncAction } from "../../hooks/useAsyncAction";

interface Props {
  idx: number;
  users: UserInfo[];
  onChanged: () => void; // 操作後の refetch（方針A）
}

// 確認が要る操作の種別と、その表示・危険度・実行 API の対応。
type ConfirmKind = "kick" | "ban" | "respawn" | "silence" | "unsilence";
const CONFIRM_META: Record<ConfirmKind, { titleKey: string; msgKey: string; danger: boolean }> = {
  kick: { titleKey: "session.kick", msgKey: "session.confirmKick", danger: true },
  ban: { titleKey: "session.ban", msgKey: "session.confirmBan", danger: true },
  respawn: { titleKey: "session.respawn", msgKey: "session.confirmRespawn", danger: false },
  silence: { titleKey: "session.silence", msgKey: "session.confirmSilence", danger: false },
  unsilence: { titleKey: "session.unsilence", msgKey: "session.confirmUnsilence", danger: false },
};

// ユーザー一覧（案B: 2行コンパクト）。
//   情報行 = 状態ドット + 名前（左） / 権限ドロップダウン + 在席離席（右）
//   操作行 = リスポーン/ミュート/メッセージ（中立）+ キック/BAN（危険・右に分離）
// 権限は選択した瞬間に即適用。respawn/silence/kick/ban は確認、message は入力モーダル。
export function SessionUsers({ idx, users, onChanged }: Props) {
  const { t } = useTranslation();
  const { busy, run } = useAsyncAction(onChanged);
  const [confirm, setConfirm] = useState<null | { kind: ConfirmKind; user: string }>(null);
  const [msgTo, setMsgTo] = useState<string | null>(null);
  const [msg, setMsg] = useState("");

  function execConfirm(kind: ConfirmKind, user: string): Promise<unknown> {
    switch (kind) {
      case "kick":
        return api.kickUser(idx, user);
      case "ban":
        return api.banUser(idx, user);
      case "respawn":
        return api.respawnUser(idx, user);
      case "silence":
        return api.silenceUser(idx, user);
      case "unsilence":
        return api.unsilenceUser(idx, user);
    }
  }

  const meta = confirm ? CONFIRM_META[confirm.kind] : null;

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
              onConfirm={(kind) => setConfirm({ kind, user: u.name })}
              onMessage={() => {
                setMsg("");
                setMsgTo(u.name);
              }}
              onRun={run}
            />
              </Fragment>
            ))}
          </Stack>
        )}
      </InspectorCard>

      <ConfirmModal
        opened={confirm !== null}
        title={meta ? t(meta.titleKey) : ""}
        message={confirm && meta ? t(meta.msgKey, { user: confirm.user }) : ""}
        danger={meta?.danger}
        loading={busy}
        onConfirm={() => {
          if (!confirm) return;
          const c = confirm;
          setConfirm(null);
          void run(() => execConfirm(c.kind, c.user));
        }}
        onClose={() => setConfirm(null)}
      />

      <Modal
        opened={msgTo !== null}
        onClose={() => setMsgTo(null)}
        title={t("session.messageTo", { user: msgTo ?? "" })}
        centered
      >
        <Textarea
          value={msg}
          onChange={(e) => setMsg(e.currentTarget.value)}
          placeholder={t("session.messagePlaceholder")}
          autosize
          minRows={3}
        />
        <Group justify="flex-end" gap="xs" mt="md">
          <Button variant="default" onClick={() => setMsgTo(null)}>
            {t("common.cancel")}
          </Button>
          <Button
            loading={busy}
            disabled={!msg.trim()}
            onClick={() => {
              const to = msgTo;
              const text = msg;
              if (!to) return;
              setMsgTo(null);
              void run(() => api.messageUser(idx, to, text));
            }}
          >
            {t("session.send")}
          </Button>
        </Group>
      </Modal>
    </>
  );
}

function UserCard({
  idx,
  u,
  busy,
  onConfirm,
  onMessage,
  onRun,
}: {
  idx: number;
  u: UserInfo;
  busy: boolean;
  onConfirm: (kind: ConfirmKind) => void; // 確認モーダルを開く（respawn/silence/unsilence/kick/ban）
  onMessage: () => void;
  onRun: (fn: () => Promise<unknown>) => void; // 即適用（権限）
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
            data={[...api.ROLES]}
            value={roleValue}
            placeholder={u.role}
            onChange={(v) => v && v !== u.role && onRun(() => api.setUserRole(idx, u.name, v))}
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
            disabled={busy}
            onClick={() => onConfirm(u.silenced ? "unsilence" : "silence")}
          >
            {u.silenced ? `🔈 ${t("session.unsilence")}` : `🔇 ${t("session.silence")}`}
          </InspectorButton>
          <InspectorButton severity="neutral" disabled={busy} onClick={onMessage}>
            ✉ {t("session.message")}
          </InspectorButton>
        </Group>
        <Group gap={4} wrap="nowrap">
          <InspectorButton severity="danger" disabled={busy} onClick={() => onConfirm("kick")}>
            {t("session.kick")}
          </InspectorButton>
          <InspectorButton severity="danger" disabled={busy} onClick={() => onConfirm("ban")}>
            {t("session.ban")}
          </InspectorButton>
        </Group>
      </Group>
    </Box>
  );
}
