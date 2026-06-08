import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Divider, Group, Stack, Switch, Text } from "@mantine/core";
import * as api from "../../api";
import type { WorldStatus, WriteResult } from "../../api";
import {
  FieldRow,
  InspectorButton,
  InspectorCard,
  InspectorNumberInput,
  InspectorSelect,
  InspectorTextarea,
  InspectorTextInput,
  RefreshButton,
} from "../../components/inspector";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useConfirm } from "../../hooks/useConfirm";
import { copyText } from "../../lib/clipboard";
import { notifyError, notifyInfo } from "../../lib/notify";

interface Props {
  idx: number;
  status: WorldStatus;
  onChanged: () => void; // 適用 / lifecycle 操作後の refetch（方針A）
  refreshing?: boolean; // ヘッダ更新アイコンのスピナー（タブ全体の refetch 中）
}

// Resonite のセッション参加用 Web リンク。取得済み sessionId から組み立てるだけ（ヘッドレスへの追加通信なし）。
// go.resonite.com 自身がカスタムID（`S-U-...:name` 等の `:` 含む）も未エンコードでリンクするため、ここでも素直に連結する。
const sessionWebUrl = (sessionId: string) => `https://go.resonite.com/session/${sessionId}`;

// セッション設定（名前/アクセス/最大/説明/一覧から隠す）= バッチ適用。
// インスペクタ風：1列・項目名(左)/入力欄(右)。lifecycle（保存/再起動/閉じる）は確認ダイアログ経由。
export function SessionSettings({ idx, status, onChanged, refreshing }: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState(status.name);
  const [level, setLevel] = useState(status.accessLevel);
  const [maxUsers, setMaxUsers] = useState<number | string>(status.maxUsers);
  const [description, setDescription] = useState(status.description);
  const [hide, setHide] = useState(status.hiddenFromListing);
  const apply = useAsyncAction(onChanged);
  const confirm = useConfirm();

  // 参加用 Web リンクをクリップボードへ。copyText は LAN/HTTP（非セキュアコンテキスト）でも動くフォールバック付き。
  const onCopyUrl = async () => {
    const ok = await copyText(sessionWebUrl(status.sessionId));
    if (ok) notifyInfo(t("session.urlCopied"));
    else notifyError(t("session.copyFailed"));
  };

  // フォームの再同期は「別セッションを表示したとき」のみ（sessionId が変化したとき）。
  // 同一セッションの refetch（ユーザー操作後 / ⟳ / 将来の自動poll）では再同期せず、
  // 未適用の編集を保持する（M1）。focus 切替・セッション再起動は sessionId が変わるため再同期される。
  const syncedId = useRef<string | null>(null);
  useEffect(() => {
    if (status.sessionId === syncedId.current) return;
    syncedId.current = status.sessionId;
    setName(status.name);
    setLevel(status.accessLevel);
    setMaxUsers(status.maxUsers);
    setDescription(status.description);
    setHide(status.hiddenFromListing);
  }, [status]);

  // maxUsers は正の整数のみ有効（空欄は Number("")=0 になるため送らない）。
  const muNum = Number(maxUsers);
  const muValid = Number.isInteger(muNum) && muNum >= 1;
  const dirty =
    name !== status.name ||
    level !== status.accessLevel ||
    (muValid && muNum !== status.maxUsers) ||
    description !== status.description ||
    hide !== status.hiddenFromListing;

  // バッチ適用は各 write を順に実行し、最初の失敗を結果として返す（=その失敗をトースト）。
  // 全成功なら {ok:true} を返し applyDone を緑トースト。1件も dirty でないときは ok のまま無音。
  function doApply() {
    void apply.run(async () => {
      const results: WriteResult[] = [];
      if (name !== status.name) results.push(await api.setSessionName(idx, name));
      if (level !== status.accessLevel) results.push(await api.setAccessLevel(idx, level));
      if (muValid && muNum !== status.maxUsers) results.push(await api.setMaxUsers(idx, muNum));
      if (description !== status.description) results.push(await api.setDescription(idx, description));
      if (hide !== status.hiddenFromListing) results.push(await api.setHideFromListing(idx, hide));
      return results.find((r) => !r.ok) ?? { ok: true };
    }, t("toast.applyDone"));
  }

  // lifecycle 操作（保存/再起動/閉じる）は確認 → 実行 → 再取得。結果を返すと confirm() がトースト。
  const askLifecycle = (
    titleKey: string,
    msgKey: string,
    danger: boolean,
    successKey: string,
    op: () => Promise<unknown>,
  ) =>
    confirm.ask({
      title: t(titleKey),
      message: t(msgKey),
      danger,
      success: t(successKey),
      onConfirm: async () => {
        const r = await op();
        onChanged();
        return r;
      },
    });

  return (
    <InspectorCard
      title={t("session.settings")}
      actions={<RefreshButton onClick={onChanged} loading={refreshing} label={t("session.refresh")} />}
    >
      <Stack gap={6}>
        <FieldRow label={t("session.name")}>
          <InspectorTextInput value={name} onChange={(e) => setName(e.currentTarget.value)} />
        </FieldRow>
        <FieldRow label={t("session.accessLevel")}>
          <InspectorSelect data={[...api.ACCESS_LEVELS]} value={level} onChange={(v) => v && setLevel(v)} />
        </FieldRow>
        <FieldRow label={t("session.maxUsers")}>
          <InspectorNumberInput value={maxUsers} onChange={setMaxUsers} min={1} allowNegative={false} />
        </FieldRow>
        <FieldRow label={t("session.description")} align="start">
          <InspectorTextarea value={description} onChange={(e) => setDescription(e.currentTarget.value)} />
        </FieldRow>
        {/* 参加用 Web リンク（読み取り専用＝淡色 Text で表示・選択可）。sessionId が無い間（起動直後など）は非表示。 */}
        {status.sessionId && (
          <FieldRow label={t("session.sessionUrl")} align="start">
            <Stack gap={4}>
              <Text size="xs" c="dimmed" style={{ wordBreak: "break-all" }}>
                {sessionWebUrl(status.sessionId)}
              </Text>
              <Group justify="flex-end">
                <InspectorButton onClick={onCopyUrl}>{t("session.copy")}</InspectorButton>
              </Group>
            </Stack>
          </FieldRow>
        )}
        <FieldRow label={t("session.hideFromListing")}>
          <Switch checked={hide} onChange={(e) => setHide(e.currentTarget.checked)} />
        </FieldRow>

        {/* 適用は主アクション（変更時のみ cyan filled で点灯）。明色背景の文字色は theme の autoContrast が自動で濃色化。 */}
        <Button
          fullWidth
          size="xs"
          mt={4}
          variant={dirty ? "filled" : "default"}
          color="brand"
          disabled={!dirty}
          loading={apply.busy}
          onClick={doApply}
        >
          {t("session.apply")}
        </Button>
        <Divider my={2} color="dark.4" />
        <Group grow gap="xs">
          <InspectorButton
            severity="neutral"
            onClick={() =>
              askLifecycle("session.save", "session.confirmSave", false, "toast.saveDone", () => api.saveSession(idx))
            }
          >
            {t("session.save")}
          </InspectorButton>
          <InspectorButton
            severity="warning"
            onClick={() =>
              askLifecycle("session.restart", "session.confirmRestart", true, "toast.restartDone", () =>
                api.restartSession(idx),
              )
            }
          >
            {t("session.restart")}
          </InspectorButton>
          <InspectorButton
            severity="danger"
            onClick={() =>
              askLifecycle("session.close", "session.confirmClose", true, "toast.closeDone", () =>
                api.closeSession(idx),
              )
            }
          >
            {t("session.close")}
          </InspectorButton>
        </Group>
      </Stack>

      <ConfirmHost confirm={confirm} />
    </InspectorCard>
  );
}
