import { Divider, Stack } from "@mantine/core";
import { useTranslation } from "react-i18next";
import { FieldRow, InspectorNumberInput, InspectorTextInput } from "../../components/inspector";
import type { ConfigMap } from "./configModel";
import { arrayToCsv, asNum, asStr, csvToArray, getStringArray } from "./configModel";
import { BufferedTextInput, HostListInput } from "./fields";

// config トップレベル（全体設定）＋アカウントのフォーム。map のキーを直接読み書きする。
export function GeneralSection({ cfg, onChange }: { cfg: ConfigMap; onChange: (cfg: ConfigMap) => void }) {
  const { t } = useTranslation();
  const set = (key: string, value: unknown) => onChange({ ...cfg, [key]: value });
  const num = (key: string) => (v: number | string) => set(key, v === "" ? "" : Number(v));

  return (
    <Stack gap={6}>
      <FieldRow label={t("config.comment")}>
        <InspectorTextInput value={asStr(cfg.comment)} onChange={(e) => set("comment", e.currentTarget.value)} />
      </FieldRow>
      <FieldRow label={t("config.tickRate")}>
        <InspectorNumberInput value={asNum(cfg.tickRate)} onChange={num("tickRate")} min={1} allowNegative={false} />
      </FieldRow>
      <FieldRow label={t("config.maxConcurrentAssetTransfers")}>
        <InspectorNumberInput
          value={asNum(cfg.maxConcurrentAssetTransfers)}
          onChange={num("maxConcurrentAssetTransfers")}
          min={1}
          allowNegative={false}
        />
      </FieldRow>
      <FieldRow label={t("config.usernameOverride")}>
        <InspectorTextInput
          value={asStr(cfg.usernameOverride)}
          onChange={(e) => set("usernameOverride", e.currentTarget.value)}
        />
      </FieldRow>
      <FieldRow label={t("config.dataFolder")}>
        <InspectorTextInput value={asStr(cfg.dataFolder)} onChange={(e) => set("dataFolder", e.currentTarget.value)} />
      </FieldRow>
      <FieldRow label={t("config.cacheFolder")}>
        <InspectorTextInput value={asStr(cfg.cacheFolder)} onChange={(e) => set("cacheFolder", e.currentTarget.value)} />
      </FieldRow>
      <FieldRow label={t("config.logsFolder")}>
        <InspectorTextInput value={asStr(cfg.logsFolder)} onChange={(e) => set("logsFolder", e.currentTarget.value)} />
      </FieldRow>
      <FieldRow label={t("config.allowedHosts")} align="start">
        <HostListInput
          hosts={getStringArray(cfg.allowedUrlHosts)}
          onChange={(h) => set("allowedUrlHosts", h)}
          addLabel={t("config.add")}
          placeholder={t("config.hostPlaceholder")}
        />
      </FieldRow>
      <FieldRow label={t("config.autoSpawnItems")}>
        <BufferedTextInput
          initial={arrayToCsv(cfg.autoSpawnItems)}
          parse={csvToArray}
          onCommit={(v) => set("autoSpawnItems", v)}
          placeholder={t("config.csvPlaceholder")}
        />
      </FieldRow>

      <Divider my={4} color="dark.4" label={t("config.accountSection")} labelPosition="center" />
      <FieldRow label={t("config.loginCredential")}>
        <InspectorTextInput
          value={asStr(cfg.loginCredential)}
          onChange={(e) => set("loginCredential", e.currentTarget.value)}
          placeholder={t("config.accountHint")}
        />
      </FieldRow>
      <FieldRow label={t("config.loginPassword")}>
        <InspectorTextInput
          type="password"
          value={asStr(cfg.loginPassword)}
          onChange={(e) => set("loginPassword", e.currentTarget.value)}
          placeholder={t("config.passwordHint")}
        />
      </FieldRow>
    </Stack>
  );
}
