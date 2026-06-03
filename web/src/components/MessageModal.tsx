import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Group, Modal, Textarea } from "@mantine/core";
import * as api from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";

// ユーザーへのメッセージ送信モーダル（セッションタブ／フレンド検索結果で共用・L2）。
// target=null で閉じ、宛先（username/name）を渡すと開く。送信は方針A（失敗/成功はトースト）。
// onSent を渡すと送信完了後に呼ぶ＝セッションタブは一覧 refetch・フレンド検索は不要（省略）。
export function MessageModal({
  idx,
  target,
  onClose,
  onSent,
}: {
  idx: number;
  target: string | null;
  onClose: () => void;
  onSent?: () => void;
}) {
  const { t } = useTranslation();
  const { busy, run } = useAsyncAction(onSent);
  const [text, setText] = useState("");
  // 新しい宛先で開くたびに本文をクリア（前回の入力を持ち越さない）。
  useEffect(() => {
    if (target !== null) setText("");
  }, [target]);

  const submit = () => {
    if (!target) return;
    const to = target;
    onClose();
    void run(() => api.messageUser(idx, to, text), t("toast.messageDone"));
  };

  return (
    <Modal opened={target !== null} onClose={onClose} title={t("session.messageTo", { user: target ?? "" })} centered>
      <Textarea
        value={text}
        onChange={(e) => setText(e.currentTarget.value)}
        placeholder={t("session.messagePlaceholder")}
        autosize
        minRows={3}
      />
      <Group justify="flex-end" gap="xs" mt="md">
        <Button variant="default" onClick={onClose}>
          {t("common.cancel")}
        </Button>
        <Button loading={busy} disabled={!text.trim()} onClick={submit}>
          {t("session.send")}
        </Button>
      </Group>
    </Modal>
  );
}
