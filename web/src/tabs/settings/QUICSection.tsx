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

const ipv6PartCount = (section: string): number | null => {
  if (section === "") return 0;
  const parts = section.split(":");
  let count = 0;
  for (let i = 0; i < parts.length; i += 1) {
    const part = parts[i];
    if (part === "") return null;
    if (part.includes(".")) {
      if (i !== parts.length - 1 || !isIPv4(part)) return null;
      count += 2;
    } else {
      if (!/^[0-9a-f]{1,4}$/i.test(part)) return null;
      count += 1;
    }
  }
  return count;
};

const isIPv6 = (value: string): boolean => {
  if (!value.includes(":")) return false;
  const sections = value.split("::");
  if (sections.length > 2) return false;
  const left = ipv6PartCount(sections[0]);
  const right = sections.length === 2 ? ipv6PartCount(sections[1]) : 0;
  if (left === null || right === null) return false;
  return sections.length === 2 ? left + right < 8 : left === 8;
};

const isValidIP = (value: string): boolean => value === "" || isIPv4(value) || isIPv6(value);

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
  const valid = isValidIP(normalized);
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
