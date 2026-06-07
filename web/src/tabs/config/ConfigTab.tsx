import { useEffect, useState } from "react";
import { Box, Center, Loader, ScrollArea, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import type { ConfigSummary } from "../../api";
import { notifyError } from "../../lib/notify";
import { ConfirmHost } from "../../components/ConfirmHost";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { useConfirm } from "../../hooks/useConfirm";
import { SplitColumns } from "../../components/SplitColumns";
import { ConfigEditor } from "./ConfigEditor";
import { ConfigList } from "./ConfigList";
import type { ConfigMap } from "./configModel";

// config 名のバリデーション（backend の SanitizeName と同じ・パストラバーサル防止）。
const NAME_RE = /^[A-Za-z0-9_-]{1,64}$/;

// コンフィグタブの container（§3.14）。一覧/読込/保存/複製/削除と未保存ガードを集約。
// working map（cfg）を単一の真実とし、未知/レア項目はキーを落とさず温存される。
// 新規/複製は即時作成方式: 押した瞬間にサーバーが採番して実体を作り（POST）、エディタは常に
// 「保存済み config の編集」だけになる（未保存ドラフト状態は無い）。名前変更＋保存＝リネーム。
export function ConfigTab({ onConfigsChanged }: { onConfigsChanged: () => void }) {
  const { t } = useTranslation();
  const [list, setList] = useState<ConfigSummary[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [cfg, setCfg] = useState<ConfigMap | null>(null);
  const [original, setOriginal] = useState<ConfigMap | null>(null);
  const [loading, setLoading] = useState(false);
  const [centralUserId, setCentralUserId] = useState(""); // customSessionId prefix の自動入力元（R12）
  // dataFolder/cacheFolder 既定値（UI改善⑤）。リセットマーカーの比較値（GeneralSection）に使う
  // （新規作成の雛形は backend の Create がテンプレから直接作るため、ここでは seed しない）。
  const [folderDefaults, setFolderDefaults] = useState<api.ConfigDefaults | null>(null);
  const [draftName, setDraftName] = useState(""); // 編集可能なコンフィグ名（selected と変えて保存＝リネーム）
  const [formNonce, setFormNonce] = useState(0); // 編集セッションごとに++＝ConfigEditor の key（毎回再マウントしバッファ入力を再シード）
  const confirm = useConfirm();
  const apply = useAsyncAction();

  // dirty = working≠original または名前変更（キー順は in-place 編集で保持）。
  // 即時作成方式によりエディタは常に保存済み config を編集する＝未保存ドラフト特例（original=null）は無い。
  const dirty = cfg !== null && (JSON.stringify(cfg) !== JSON.stringify(original) || draftName !== selected);
  // 名前の検証は親に一元化。エラー文言は「入力済みで不正」な時だけ出す（空の新規欄を即赤にしない）。
  const nameValid = NAME_RE.test(draftName.trim());
  const nameError = draftName.trim() !== "" && !nameValid ? t("config.invalidName") : undefined;
  const canSave = dirty && nameValid; // 保存ボタンの活性＝変更あり かつ 名前が有効

  const refreshList = async () => {
    const l = await api.getConfigs();
    setList(l);
    onConfigsChanged(); // トップバーの config 選択肢も更新
    return l;
  };

  // 編集セッションを初期化（読込/空 で共通）。formNonce を進めて ConfigEditor を再マウントし、
  // バッファ付き入力（タグ/autoSpawn 等）を確実に再シードする。空（config 0件）は cfg=null。
  const seedEditor = (sel: string | null, name: string, cfgVal: ConfigMap | null, orig: ConfigMap | null) => {
    setSelected(sel);
    setDraftName(name);
    setCfg(cfgVal);
    setOriginal(orig);
    setFormNonce((n) => n + 1);
  };

  const load = async (name: string) => {
    setLoading(true);
    const m = await api.getConfig(name);
    setLoading(false);
    if (!m) {
      notifyError(t("config.loadFailed"));
      return;
    }
    seedEditor(name, name, m, m);
  };

  // 初回: 一覧取得 → 先頭を読込。中央アカウントの解決済 UserID も取得（prefix 自動入力用・R12）。
  useEffect(() => {
    void (async () => {
      const l = await api.getConfigs();
      setList(l);
      if (l[0]) void load(l[0].name);
    })();
    void api.getCredentials().then((c) => {
      if (c) setCentralUserId(c.userId);
    });
    void api.getConfigDefaults().then(setFolderDefaults);
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

  // 保存（上書き／リネーム）。
  //   name===selected: 同名上書き ／ name=別名: 保存リネーム（from=selected の内容を name で保存し元を削除）。
  //   リネーム先が既存名なら上書き確認を挟む。無効名は保存ガード（ボタンも disabled）。
  const save = () => {
    if (!cfg) return;
    const body = cfg;
    const name = draftName.trim();
    if (!NAME_RE.test(name)) return;
    const from = selected !== null && selected !== name ? selected : undefined;
    const persist = async () => {
      const r = await api.saveConfig(name, body, from);
      if (r.ok) {
        setSelected(name);
        setDraftName(name);
        setOriginal(body);
        await refreshList();
      }
      return r;
    };
    if (from !== undefined && list.some((c) => c.name === name)) {
      confirm.ask({
        title: t("config.overwriteTitle"),
        message: t("config.renameOverwriteMessage", { from, name }),
        danger: true,
        success: t("config.toastSaved"),
        onConfirm: persist,
      });
    } else {
      void apply.run(persist, t("config.toastSaved"));
    }
  };

  // 作成系（新規/複製）の共通処理: サーバーが採番して即時作成 → 一覧を更新して読込（即時作成方式）。
  const createAndLoad = (create: () => Promise<api.WriteResult>, successMsg: string) =>
    guardDiscard(() => {
      void apply.run(async () => {
        const r = await create();
        const name = (r.data as { name?: string } | undefined)?.name;
        if (r.ok && name) {
          await refreshList();
          await load(name);
        }
        return r;
      }, successMsg);
    });

  // 新規＝テンプレ（dataFolder/cacheFolder 既定値焼き込み・comment 空）から new-config, … を作成。
  const openNew = () => createAndLoad(() => api.createConfig(), t("config.toastCreated"));
  // 複製＝サーバー側バイトコピーで {元名}-copy, … を作成（password も写る・対象はディスク上の保存済み内容）。
  const openDuplicate = (name: string) => createAndLoad(() => api.duplicateConfig(name), t("config.toastDuplicated"));

  const selectFirst = (l: ConfigSummary[]) => {
    if (l[0]) void load(l[0].name);
    else seedEditor(null, "", null, null);
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
            ) : cfg ? (
              <ConfigEditor
                key={formNonce}
                draftName={draftName}
                onDraftNameChange={setDraftName}
                nameError={nameError}
                cfg={cfg}
                onChange={setCfg}
                canSave={canSave}
                saving={apply.busy}
                onSave={save}
                centralUserId={centralUserId}
                folderDefaults={folderDefaults}
              />
            ) : (
              <Center h={200}>
                <Text c="dimmed">{list.length === 0 ? t("config.empty") : t("config.selectPrompt")}</Text>
              </Center>
            )
          }
        />
      </Box>

      <ConfirmHost confirm={confirm} />
    </ScrollArea>
  );
}
