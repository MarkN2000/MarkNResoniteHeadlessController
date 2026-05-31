import { useCallback, useEffect, useState } from "react";
import { Box, Center, Collapse, Divider, Group, Loader, Stack, Text, UnstyledButton } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import type { AppSettings } from "../../api";
import { FieldRow, InspectorCard, InspectorNumberInput, InspectorTextInput } from "../../components/inspector";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { SaveButton } from "./SaveButton";

// 反映タイミングの注記（入力欄の下・右寄せ・控えめ）。
function Note({ children }: { children: string }) {
  return (
    <Text size="xs" c="dimmed" ta="right">
      {children}
    </Text>
  );
}

// アプリ設定（port / Resonite ヘッドレスパス / 上級: コンフィグ保存先）。encoding は UI 非搭載。
// 反映: port・configDir = MRHC 再起動後 / Resonite パス = 次回ヘッドレス起動。
export function AppSettingsSection() {
  const { t } = useTranslation();
  const [orig, setOrig] = useState<AppSettings | null>(null);
  const [port, setPort] = useState<number | string>("");
  const [path, setPath] = useState("");
  const [dir, setDir] = useState("");
  const [advanced, setAdvanced] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const apply = useAsyncAction();

  const load = useCallback(async () => {
    const s = await api.getAppSettings();
    if (s) {
      setOrig(s);
      setPort(s.port);
      setPath(s.resoniteHeadlessPath);
      setDir(s.headlessConfigDir);
      setLoadFailed(false);
    } else {
      // 取得失敗（通信不通等）はローダーで固まらせず明示する（コンフィグタブと同様）。
      setLoadFailed(true);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  const portNum = Number(port);
  const portValid = Number.isInteger(portNum) && portNum >= 1 && portNum <= 65535;
  const dirty =
    !!orig &&
    ((portValid && portNum !== orig.port) ||
      path !== orig.resoniteHeadlessPath ||
      dir !== orig.headlessConfigDir);
  const canSave = portValid && dirty;

  const save = () =>
    apply.run(async () => {
      const body: AppSettings = { port: portNum, resoniteHeadlessPath: path.trim(), headlessConfigDir: dir.trim() };
      const r = await api.putAppSettings(body);
      if (r.ok) setOrig(body);
      return r;
    }, t("settings.toastAppSaved"));

  return (
    <InspectorCard title={t("settings.appSection")}>
      {!orig ? (
        <Center h={60}>
          {loadFailed ? <Text size="sm" c="red.6">{t("settings.loadError")}</Text> : <Loader size="sm" />}
        </Center>
      ) : (
        <Stack gap={6}>
          <FieldRow label={t("settings.port")}>
            <InspectorNumberInput value={port} onChange={setPort} min={1} max={65535} allowNegative={false} />
          </FieldRow>
          <Note>{t("settings.restartNote")}</Note>

          <FieldRow label={t("settings.headlessPath")}>
            <InspectorTextInput value={path} onChange={(e) => setPath(e.currentTarget.value)} />
          </FieldRow>
          <Note>{t("settings.headlessPathNote")}</Note>

          <Divider my={2} color="dark.4" />
          <UnstyledButton onClick={() => setAdvanced((a) => !a)}>
            <Group gap={4}>
              <Text size="xs" c="dimmed">
                {t("settings.advanced")}
              </Text>
              <Text size="xs" c="dimmed">
                {advanced ? "▴" : "▾"}
              </Text>
            </Group>
          </UnstyledButton>
          <Collapse in={advanced}>
            <Box pt={4}>
              <FieldRow label={t("settings.configDir")}>
                <InspectorTextInput
                  value={dir}
                  onChange={(e) => setDir(e.currentTarget.value)}
                  placeholder={t("settings.configDirPlaceholder")}
                />
              </FieldRow>
              <Note>{t("settings.restartNote")}</Note>
            </Box>
          </Collapse>

          <SaveButton label={t("settings.save")} onClick={save} disabled={!canSave} loading={apply.busy} />
        </Stack>
      )}
    </InspectorCard>
  );
}
