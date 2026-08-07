import { useCallback, useEffect, useState } from "react";
import { Anchor, Center, Loader, Stack, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import { FieldRow, InspectorCard, InspectorTextInput } from "../../components/inspector";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { SaveButton } from "./SaveButton";

const isIPv4 = (value: string): boolean => {
  const parts = value.split(".");
  return (
    parts.length === 4 &&
    parts.every(
      (part) =>
        /^\d{1,3}$/.test(part) &&
        (part === "0" || !part.startsWith("0")) &&
        Number(part) >= 0 &&
        Number(part) <= 255,
    )
  );
};

const isValidIPv4 = (value: string): boolean => value === "" || isIPv4(value);

// Resonite Headless の Config.json にある quicConfig.publicIP だけを編集する。
// QUIC 対応可否は起動ログでしか判定できないため、状態化・自動再起動は行わない。
export function QUICSection() {
  const { t } = useTranslation();
  const [orig, setOrig] = useState<api.QUICConfig | null>(null);
  const [publicIP, setPublicIP] = useState("");
  const [loadFailed, setLoadFailed] = useState(false);
  const apply = useAsyncAction();

  const load = useCallback(async () => {
    const config = await api.getQUICConfig();
    if (config) {
      setOrig(config);
      setPublicIP(config.publicIP);
      setLoadFailed(false);
    } else {
      setLoadFailed(true);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  const normalized = publicIP.trim();
  const valid = isValidIPv4(normalized);
  const dirty = orig !== null && normalized !== orig.publicIP;
  const save = () =>
    apply.run(async () => {
      const body: api.QUICConfig = { publicIP: normalized };
      const result = await api.putQUICConfig(body);
      if (result.ok) setOrig(body);
      return result;
    }, t("settings.toastQUICSaved"));

  return (
    <InspectorCard title={t("settings.quicSection")}>
      {!orig ? (
        <Center h={60}>
          {loadFailed ? <Text size="sm" c="red.6">{t("settings.loadError")}</Text> : <Loader size="sm" />}
        </Center>
      ) : (
        <Stack gap={8}>
          <Text size="xs" c="dimmed">
            {t("settings.quicDesc")}
          </Text>
          <FieldRow label={t("settings.quicPublicIP")}>
            <InspectorTextInput
              value={publicIP}
              onChange={(event) => setPublicIP(event.currentTarget.value)}
              placeholder={t("settings.quicPublicIPPlaceholder")}
              error={valid ? undefined : t("settings.quicInvalidIP")}
            />
          </FieldRow>
          <Text size="xs" c="dimmed" ta="right">
            {t("settings.quicNextStart")}
          </Text>
          <Text size="xs" c="dimmed">
            {t("settings.quicSupportNote")} {" "}
            <Anchor
              size="xs"
              href="https://learn.microsoft.com/dotnet/fundamentals/networking/quic/quic-overview#platform-dependencies"
              target="_blank"
              rel="noreferrer"
            >
              {t("settings.quicDependencies")}
            </Anchor>
          </Text>
          <SaveButton label={t("settings.save")} onClick={save} disabled={!dirty || !valid} loading={apply.busy} />
        </Stack>
      )}
    </InspectorCard>
  );
}
