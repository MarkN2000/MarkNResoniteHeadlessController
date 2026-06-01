import { Divider, Stack } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { FieldRow, InspectorNumberInput, InspectorTextInput } from "../../components/inspector";
import { ConfirmModal } from "../../components/ConfirmModal";
import { useConfirm } from "../../hooks/useConfirm";
import type { ConfigMap } from "./configModel";
import { arrayToCsv, asNum, asStr, csvToArray, defaultConfig, getStringArray } from "./configModel";
import { BufferedTextInput, HostListInput } from "./fields";

// config トップレベル（全体設定）＋アカウントのフォーム。map のキーを直接読み書きする。
export function GeneralSection({ cfg, onChange }: { cfg: ConfigMap; onChange: (cfg: ConfigMap) => void }) {
  const { t } = useTranslation();
  const set = (key: string, value: unknown) => onChange({ ...cfg, [key]: value });
  const num = (key: string) => (v: number | string) => set(key, v === "" ? "" : Number(v));

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
      <FieldRow label={t("config.comment")} {...resetProps("comment", t("config.comment"))}>
        <InspectorTextInput value={asStr(cfg.comment)} onChange={(e) => set("comment", e.currentTarget.value)} />
      </FieldRow>
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
      <FieldRow label={t("config.usernameOverride")} {...resetProps("usernameOverride", t("config.usernameOverride"))}>
        <InspectorTextInput
          value={asStr(cfg.usernameOverride)}
          onChange={(e) => set("usernameOverride", e.currentTarget.value)}
        />
      </FieldRow>
      <FieldRow label={t("config.dataFolder")} {...resetProps("dataFolder", t("config.dataFolder"))}>
        <InspectorTextInput value={asStr(cfg.dataFolder)} onChange={(e) => set("dataFolder", e.currentTarget.value)} />
      </FieldRow>
      <FieldRow label={t("config.cacheFolder")} {...resetProps("cacheFolder", t("config.cacheFolder"))}>
        <InspectorTextInput value={asStr(cfg.cacheFolder)} onChange={(e) => set("cacheFolder", e.currentTarget.value)} />
      </FieldRow>
      <FieldRow label={t("config.logsFolder")} {...resetProps("logsFolder", t("config.logsFolder"))}>
        <InspectorTextInput value={asStr(cfg.logsFolder)} onChange={(e) => set("logsFolder", e.currentTarget.value)} />
      </FieldRow>
      <FieldRow
        label={t("config.allowedHosts")}
        align="start"
        {...resetProps("allowedUrlHosts", t("config.allowedHosts"))}
      >
        <HostListInput
          hosts={getStringArray(cfg.allowedUrlHosts)}
          onChange={(h) => set("allowedUrlHosts", h)}
          addLabel={t("config.add")}
          placeholder={t("config.hostPlaceholder")}
        />
      </FieldRow>
      {/* autoSpawnItems はバッファ付き入力（内部 state）でリセットが表示へ反映されないため対象外。 */}
      <FieldRow label={t("config.autoSpawnItems")}>
        <BufferedTextInput
          initial={arrayToCsv(cfg.autoSpawnItems)}
          parse={csvToArray}
          onCommit={(v) => set("autoSpawnItems", v)}
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
