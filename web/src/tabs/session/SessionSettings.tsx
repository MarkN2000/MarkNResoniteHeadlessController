import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Divider, Group, Stack, Switch } from "@mantine/core";
import * as api from "../../api";
import type { WorldStatus } from "../../api";
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
import { ConfirmModal } from "../../components/ConfirmModal";
import { useAsyncAction } from "../../hooks/useAsyncAction";

interface Props {
  idx: number;
  status: WorldStatus;
  onChanged: () => void; // 適用 / lifecycle 操作後の refetch（方針A）
  refreshing?: boolean; // ヘッダ更新アイコンのスピナー（タブ全体の refetch 中）
}

// セッション設定（名前/アクセス/最大/説明/一覧から隠す）= バッチ適用。
// インスペクタ風：1列・項目名(左)/入力欄(右)。lifecycle（保存/再起動/閉じる）は確認ダイアログ経由。
export function SessionSettings({ idx, status, onChanged, refreshing }: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState(status.name);
  const [level, setLevel] = useState(status.accessLevel);
  const [maxUsers, setMaxUsers] = useState<number | string>(status.maxUsers);
  const [description, setDescription] = useState(status.description);
  const [hide, setHide] = useState(status.hiddenFromListing);
  const [confirm, setConfirm] = useState<null | "save" | "restart" | "close">(null);
  const apply = useAsyncAction(onChanged);
  const life = useAsyncAction(onChanged);

  // フォームの再同期は「別セッションを表示したとき」のみ（sessionId が変化したとき）。
  // 同一セッションの refetch（ユーザー操作後 / ⟳ / 将来の自動poll）では再同期せず、
  // 未適用の編集を保持する（M1: 編集中に他操作で入力が消える問題の対策）。
  // focus 切替・セッション再起動は sessionId が変わるため再同期される。
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

  function doApply() {
    void apply.run(async () => {
      if (name !== status.name) await api.setSessionName(idx, name);
      if (level !== status.accessLevel) await api.setAccessLevel(idx, level);
      if (muValid && muNum !== status.maxUsers) await api.setMaxUsers(idx, muNum);
      if (description !== status.description) await api.setDescription(idx, description);
      if (hide !== status.hiddenFromListing) await api.setHideFromListing(idx, hide);
    });
  }

  async function doLifecycle(kind: "save" | "restart" | "close") {
    await life.run(() => {
      if (kind === "save") return api.saveSession(idx);
      if (kind === "restart") return api.restartSession(idx);
      return api.closeSession(idx);
    });
    setConfirm(null);
  }

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
        <FieldRow label={t("session.hideFromListing")}>
          <Switch checked={hide} onChange={(e) => setHide(e.currentTarget.checked)} />
        </FieldRow>

        {/* 適用は主アクション（変更時のみ cyan filled で点灯）。severity ボタンとは別扱い。 */}
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
          <InspectorButton severity="neutral" onClick={() => setConfirm("save")}>
            {t("session.save")}
          </InspectorButton>
          <InspectorButton severity="warning" onClick={() => setConfirm("restart")}>
            {t("session.restart")}
          </InspectorButton>
          <InspectorButton severity="danger" onClick={() => setConfirm("close")}>
            {t("session.close")}
          </InspectorButton>
        </Group>
      </Stack>

      <ConfirmModal
        opened={confirm === "save"}
        title={t("session.save")}
        message={t("session.confirmSave")}
        loading={life.busy}
        onConfirm={() => void doLifecycle("save")}
        onClose={() => setConfirm(null)}
      />
      <ConfirmModal
        opened={confirm === "restart"}
        title={t("session.restart")}
        message={t("session.confirmRestart")}
        danger
        loading={life.busy}
        onConfirm={() => void doLifecycle("restart")}
        onClose={() => setConfirm(null)}
      />
      <ConfirmModal
        opened={confirm === "close"}
        title={t("session.close")}
        message={t("session.confirmClose")}
        danger
        loading={life.busy}
        onConfirm={() => void doLifecycle("close")}
        onClose={() => setConfirm(null)}
      />
    </InspectorCard>
  );
}
