import { Divider, Stack } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { CollapsibleSection, FieldRow, InspectorNumberInput, InspectorTextarea, InspectorTextInput } from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConfirm } from "../../hooks/useConfirm";
import type { ConfigMap } from "./configModel";
import { arrayToCsv, asNum, asStr, csvToArray, defaultConfig, getStringArray } from "./configModel";
import { BufferedTextInput, StringListInput } from "./fields";
import { AdvancedFieldsEditor } from "./AdvancedFieldsEditor";
import { TOP_DEDICATED_KEYS, TOP_NICHE_CATALOG } from "./fieldCatalog";

// config トップレベル（全体設定）＋アカウントのフォーム。map のキーを直接読み書きする。
// 基本は「メモ」のみ常時表示し、技術系（tickRate/フォルダ/ホスト等）とアカウントは上級設定へ畳む（点5）。
// コンフィグ名は ConfigEditor 側で常時表示（メモと並ぶ基本項目）。
export function GeneralSection({ cfg, onChange }: { cfg: ConfigMap; onChange: (cfg: ConfigMap) => void }) {
  const { t } = useTranslation();
  const set = (key: string, value: unknown) => onChange({ ...cfg, [key]: value });
  // 数値欄: 空欄は undefined（保存JSONからキーを省く＝headless 既定）。"" を書くと数値型へ不整合になるため（M1）。
  const num = (key: string) => (v: number | string) => set(key, v === "" ? undefined : Number(v));
  // テキスト欄: 空文字（空白のみ含む）は null（未設定）として保存し「空欄を登録しない」。
  // 対象は JSON Schema 上いずれも null 許容。配列欄（allowedUrlHosts/autoSpawnItems）は各 onChange で空→null。
  const setText = (key: string, v: string) => set(key, v.trim() === "" ? null : v);

  // マーカー（ハンドル）クリック＝その項目を defaultConfig() の既定値へ戻す（確認あり）。
  // 雛形に無いキーは undefined＝暗黙の既定（空/フォールバック）に戻る。
  const confirm = useConfirm();
  const resetProps = (key: string, fieldLabel: string) => ({
    markerLabel: t("common.resetToDefault"),
    onMarkerClick: () =>
      confirm.ask({
        title: t("common.resetConfirmTitle"),
        message: t("common.resetConfirmMsg", { field: fieldLabel }),
        onConfirm: () => set(key, defaultConfig()[key]),
      }),
  });

  return (
    <Stack gap={6}>
      <FieldRow label={t("config.comment")} align="start" {...resetProps("comment", t("config.comment"))}>
        {/* メモは長文想定で autosize の Textarea（自動折り返し＋欄が縦に自動拡張）。
            空は "" のまま＝Resonite 不使用の表示用メモなので null 正規化対象外。 */}
        <InspectorTextarea value={asStr(cfg.comment)} onChange={(e) => set("comment", e.currentTarget.value)} minRows={1} />
      </FieldRow>
      {/* tickRate / 最大同時転送は基本（一般）として常時表示。 */}
      <FieldRow label={t("config.tickRate")} {...resetProps("tickRate", t("config.tickRate"))}>
        <InspectorNumberInput value={asNum(cfg.tickRate)} onChange={num("tickRate")} min={1} allowNegative={false} />
      </FieldRow>
      <FieldRow
        label={t("config.maxConcurrentAssetTransfers")}
        {...resetProps("maxConcurrentAssetTransfers", t("config.maxConcurrentAssetTransfers"))}
      >
        <InspectorNumberInput
          value={asNum(cfg.maxConcurrentAssetTransfers)}
          onChange={num("maxConcurrentAssetTransfers")}
          min={1}
          allowNegative={false}
        />
      </FieldRow>
      {/* 技術系（フォルダ/ホスト等）＋アカウントは上級設定へ畳む。WorldsSection と同じ CollapsibleSection・既定は閉じ。 */}
      <CollapsibleSection title={t("common.advancedSection")}>
        <Stack gap={6}>
          <FieldRow label={t("config.usernameOverride")} {...resetProps("usernameOverride", t("config.usernameOverride"))}>
            <InspectorTextInput
              value={asStr(cfg.usernameOverride)}
              onChange={(e) => setText("usernameOverride", e.currentTarget.value)}
            />
          </FieldRow>
          <FieldRow label={t("config.dataFolder")} {...resetProps("dataFolder", t("config.dataFolder"))}>
            <InspectorTextInput value={asStr(cfg.dataFolder)} onChange={(e) => setText("dataFolder", e.currentTarget.value)} />
          </FieldRow>
          <FieldRow label={t("config.cacheFolder")} {...resetProps("cacheFolder", t("config.cacheFolder"))}>
            <InspectorTextInput value={asStr(cfg.cacheFolder)} onChange={(e) => setText("cacheFolder", e.currentTarget.value)} />
          </FieldRow>
          <FieldRow label={t("config.logsFolder")} {...resetProps("logsFolder", t("config.logsFolder"))}>
            <InspectorTextInput value={asStr(cfg.logsFolder)} onChange={(e) => setText("logsFolder", e.currentTarget.value)} />
          </FieldRow>
          <FieldRow
            label={t("config.allowedHosts")}
            align="start"
            {...resetProps("allowedUrlHosts", t("config.allowedHosts"))}
          >
            <StringListInput
              items={getStringArray(cfg.allowedUrlHosts)}
              onChange={(h) => set("allowedUrlHosts", h.length ? h : null)}
              addLabel={t("config.add")}
              placeholder={t("config.hostPlaceholder")}
            />
          </FieldRow>
          {/* autoSpawnItems はバッファ付き入力（内部 state）でリセットが表示へ反映されないため対象外。 */}
          <FieldRow label={t("config.autoSpawnItems")}>
            <BufferedTextInput
              initial={arrayToCsv(cfg.autoSpawnItems)}
              parse={csvToArray}
              onCommit={(v) => set("autoSpawnItems", Array.isArray(v) && v.length ? v : null)}
              placeholder={t("config.csvPlaceholder")}
            />
          </FieldRow>

          <Divider my={4} color="dark.4" label={t("config.accountSection")} labelPosition="center" />
          <FieldRow label={t("config.loginCredential")} {...resetProps("loginCredential", t("config.loginCredential"))}>
            <InspectorTextInput
              value={asStr(cfg.loginCredential)}
              onChange={(e) => set("loginCredential", e.currentTarget.value)}
              placeholder={t("config.accountHint")}
            />
          </FieldRow>
          <FieldRow label={t("config.loginPassword")} {...resetProps("loginPassword", t("config.loginPassword"))}>
            <InspectorTextInput
              type="password"
              value={asStr(cfg.loginPassword)}
              onChange={(e) => set("loginPassword", e.currentTarget.value)}
              placeholder={t("config.passwordHint")}
            />
          </FieldRow>
          <Divider my={4} color="dark.4" />
          {/* ③詳細フィールド（トップレベル）: 専用フォームに無い公式キー（universeId 等）を追加。
              dedicated=TOP_DEDICATED_KEYS により $schema/startWorlds 等の構造キーは出さない。 */}
          <AdvancedFieldsEditor obj={cfg} onChange={onChange} dedicated={TOP_DEDICATED_KEYS} catalog={TOP_NICHE_CATALOG} />
        </Stack>
      </CollapsibleSection>

      <ConfirmModal
        opened={confirm.request !== null}
        title={confirm.request?.title ?? ""}
        message={confirm.request?.message}
        danger={confirm.request?.danger}
        loading={confirm.busy}
        onConfirm={() => void confirm.confirm()}
        onClose={confirm.close}
      />
    </Stack>
  );
}
