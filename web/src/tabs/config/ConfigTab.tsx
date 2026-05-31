import { useEffect, useState } from "react";
import { Box, Button, Center, Group, Loader, Modal, ScrollArea, Text, TextInput } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import type { ConfigSummary } from "../../api";
import { notifyError } from "../../lib/notify";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useConfirm } from "../../hooks/useConfirm";
import { SplitColumns } from "../../components/SplitColumns";
import { ConfigEditor } from "./ConfigEditor";
import { ConfigList } from "./ConfigList";
import type { ConfigMap } from "./configModel";
import { defaultConfig } from "./configModel";

// config 名のバリデーション（backend の SanitizeName と同じ・パストラバーサル防止）。
const NAME_RE = /^[A-Za-z0-9_-]{1,64}$/;

interface NameModalState {
  mode: "new" | "duplicate";
  value: string;
  error?: string;
  source?: ConfigMap; // 複製元の全文（行から複製時に GET したもの）
}

// コンフィグタブの container（§3.14）。一覧/読込/保存/複製/削除と未保存ガードを集約。
// working map（cfg）を単一の真実とし、未知/レア項目はキーを落とさず温存される。
export function ConfigTab({ onConfigsChanged }: { onConfigsChanged: () => void }) {
  const { t } = useTranslation();
  const [list, setList] = useState<ConfigSummary[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [cfg, setCfg] = useState<ConfigMap | null>(null);
  const [original, setOriginal] = useState<ConfigMap | null>(null);
  const [loading, setLoading] = useState(false);
  const [nameModal, setNameModal] = useState<NameModalState | null>(null);
  const confirm = useConfirm();
  const apply = useAsyncAction();

  // 新規/複製（original=null）は常に dirty。既存は working≠original で判定（キー順は in-place 編集で保持）。
  const dirty = cfg !== null && (original === null || JSON.stringify(cfg) !== JSON.stringify(original));

  const refreshList = async () => {
    const l = await api.getConfigs();
    setList(l);
    onConfigsChanged(); // トップバーの config 選択肢も更新
    return l;
  };

  const load = async (name: string) => {
    setLoading(true);
    const m = await api.getConfig(name);
    setLoading(false);
    if (!m) {
      notifyError(t("config.loadFailed"));
      return;
    }
    setSelected(name);
    setCfg(m);
    setOriginal(m);
  };

  // 初回: 一覧取得 → 先頭を読込。
  useEffect(() => {
    void (async () => {
      const l = await api.getConfigs();
      setList(l);
      if (l[0]) void load(l[0].name);
    })();
  }, []);

  // 未保存編集があれば破棄確認を挟んでから action を実行（一覧切替/新規/複製で共通）。
  const guardDiscard = (action: () => void) => {
    if (dirty) {
      confirm.ask({
        title: t("config.discardTitle"),
        message: t("config.discardMessage"),
        danger: true,
        onConfirm: () => action(),
      });
    } else {
      action();
    }
  };

  const select = (name: string) => {
    if (name !== selected) guardDiscard(() => void load(name));
  };

  const save = () =>
    apply.run(async () => {
      if (!selected || !cfg) return { ok: true };
      const r = await api.saveConfig(selected, cfg);
      if (r.ok) {
        setOriginal(cfg);
        await refreshList();
      }
      return r;
    }, t("config.toastSaved"));

  const openNew = () => guardDiscard(() => setNameModal({ mode: "new", value: "" }));
  // 複製は一覧の行から（name 指定）。複製元の全文が要るので GET してからモーダルを開く。
  const openDuplicate = (name: string) =>
    guardDiscard(() => {
      void (async () => {
        const m = await api.getConfig(name);
        if (!m) {
          notifyError(t("config.loadFailed"));
          return;
        }
        setNameModal({ mode: "duplicate", value: `${name}-copy`, source: m });
      })();
    });

  const confirmName = () => {
    if (!nameModal) return;
    const name = nameModal.value.trim();
    if (!NAME_RE.test(name)) {
      setNameModal({ ...nameModal, error: t("config.invalidName") });
      return;
    }
    if (list.some((c) => c.name === name)) {
      setNameModal({ ...nameModal, error: t("config.nameCollision") });
      return;
    }
    // 複製は GET した複製元（source）をクローン。新規は同梱デフォルト雛形。
    const base: ConfigMap =
      nameModal.mode === "duplicate" && nameModal.source
        ? (JSON.parse(JSON.stringify(nameModal.source)) as ConfigMap)
        : defaultConfig();
    setSelected(name);
    setCfg(base);
    setOriginal(null); // 未保存 → dirty=true（保存で upsert）
    setNameModal(null);
  };

  const selectFirst = (l: ConfigSummary[]) => {
    if (l[0]) void load(l[0].name);
    else {
      setSelected(null);
      setCfg(null);
      setOriginal(null);
    }
  };

  // 削除は一覧の行から（name 指定・行は保存済み config のみ）。選択中を消したら先頭へ繰り上げ。
  const onDelete = (name: string) =>
    confirm.ask({
      title: t("config.deleteTitle"),
      message: t("config.confirmDelete", { name }),
      danger: true,
      success: t("config.toastDeleted"),
      onConfirm: async () => {
        const r = await api.deleteConfig(name);
        if (r.ok) {
          const l = await refreshList();
          if (name === selected) selectFirst(l);
        }
        return r;
      },
    });

  return (
    <ScrollArea h="100%" type="hover">
      {/* 他タブと共通の SplitColumns（560+560・xl で2カラム / 未満は縦積み）。左=一覧 / 右=編集。 */}
      <Box pb="md">
        <SplitColumns
          left={
            <ConfigList
              list={list}
              selected={selected}
              onSelect={select}
              onNew={openNew}
              onDuplicate={openDuplicate}
              onDelete={onDelete}
            />
          }
          right={
            loading ? (
              <Center h={200}>
                <Loader />
              </Center>
            ) : cfg && selected ? (
              <ConfigEditor
                key={selected}
                name={selected}
                cfg={cfg}
                onChange={setCfg}
                dirty={dirty}
                saving={apply.busy}
                onSave={save}
              />
            ) : (
              <Center h={200}>
                <Text c="dimmed">{list.length === 0 ? t("config.empty") : t("config.selectPrompt")}</Text>
              </Center>
            )
          }
        />
      </Box>

      <Modal
        opened={nameModal !== null}
        onClose={() => setNameModal(null)}
        title={nameModal?.mode === "duplicate" ? t("config.duplicateTitle") : t("config.newTitle")}
        centered
      >
        <TextInput
          label={t("config.nameLabel")}
          placeholder="my-config"
          value={nameModal?.value ?? ""}
          error={nameModal?.error}
          data-autofocus
          onChange={(e) => nameModal && setNameModal({ ...nameModal, value: e.currentTarget.value, error: undefined })}
          onKeyDown={(e) => {
            if (e.key === "Enter") confirmName();
          }}
        />
        <Group justify="flex-end" gap="xs" mt="md">
          <Button variant="default" onClick={() => setNameModal(null)}>
            {t("common.cancel")}
          </Button>
          <Button color="brand" onClick={confirmName}>
            {t("common.confirm")}
          </Button>
        </Group>
      </Modal>

      <ConfirmModal
        opened={confirm.request !== null}
        title={confirm.request?.title ?? ""}
        message={confirm.request?.message}
        danger={confirm.request?.danger}
        loading={confirm.busy}
        onConfirm={() => void confirm.confirm()}
        onClose={confirm.close}
      />
    </ScrollArea>
  );
}
