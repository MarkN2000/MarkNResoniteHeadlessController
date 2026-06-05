import { useCallback, useEffect, useState } from "react";
import { Box, Button, Center, Divider, Group, Loader, Progress, ScrollArea, Stack, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import type { SteamConfig, SteamStatus, Status } from "../../api";
import { FieldRow, InspectorCard, InspectorTextInput } from "../../components/inspector";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { SaveButton } from "./SaveButton";

// 進捗ログの小窓（milestone / log / result の行を表示）。進捗 % 行は流さない（多すぎるため）。
function SteamLog({ logs }: { logs: string[] }) {
  if (logs.length === 0) return null;
  return (
    <ScrollArea h={120} type="auto">
      <Box style={{ fontFamily: "monospace", fontSize: 11, lineHeight: 1.5, whiteSpace: "pre-wrap" }}>
        {logs.map((l, i) => (
          <div key={i}>{l}</div>
        ))}
      </Box>
    </ScrollArea>
  );
}

// Steam（DepotDownloader）による Resonite 入手/更新（P9-B・設定タブ）。
// 上段=DL用 Steam アカウント(A)の設定（秘密は hasXxx 表示・空=変更なし）。
// 下段=「今すぐ更新」（停止中のみ）＋ SSE による進捗/ログ/結果のライブ表示・中止。
export function SteamSection({ status }: { status: Status | null }) {
  const { t } = useTranslation();

  // 設定フォーム
  const [orig, setOrig] = useState<SteamConfig | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [branchCode, setBranchCode] = useState("");
  const [installDir, setInstallDir] = useState("");
  const [loadFailed, setLoadFailed] = useState(false);
  const save = useAsyncAction();
  const update = useAsyncAction();

  // 更新の進行（SSE 駆動）
  const [st, setSt] = useState<SteamStatus>({ state: "idle", percent: 0 });
  const [logs, setLogs] = useState<string[]>([]);

  const load = useCallback(async () => {
    const c = await api.getSteamConfig();
    if (c) {
      setOrig(c);
      setUsername(c.username);
      setInstallDir(c.installDir);
      setPassword("");
      setBranchCode("");
      setLoadFailed(false);
    } else {
      setLoadFailed(true);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  // SSE /steam/events: 進捗・ログ・結果をライブ反映。接続直後に steam-status が現状を一度送る。
  useEffect(() => {
    const es = new EventSource("/api/v1/steam/events");
    const addLog = (s: string) =>
      setLogs((prev) => {
        const next = [...prev, s];
        return next.length > 200 ? next.slice(next.length - 200) : next;
      });
    es.addEventListener("steam-status", (e) => setSt(JSON.parse((e as MessageEvent).data) as SteamStatus));
    es.addEventListener("steam-progress", (e) => {
      const d = JSON.parse((e as MessageEvent).data) as { percent?: number; file?: string };
      setSt((s) => ({ ...s, state: "running", percent: d.percent ?? 0, file: d.file }));
    });
    es.addEventListener("steam-milestone", (e) => {
      const d = JSON.parse((e as MessageEvent).data) as { text?: string };
      if (d.text) {
        setSt((s) => ({ ...s, phase: d.text }));
        addLog(d.text);
      }
    });
    es.addEventListener("steam-log", (e) => {
      const d = JSON.parse((e as MessageEvent).data) as { text?: string };
      if (d.text) addLog(d.text);
    });
    es.addEventListener("steam-result", (e) => {
      const d = JSON.parse((e as MessageEvent).data) as { text?: string };
      const err = d.text ?? "";
      setSt((s) => ({
        ...s,
        state: err ? "failed" : "success",
        lastError: err || undefined,
        percent: err ? s.percent : 100,
      }));
      addLog(err ? `${t("settings.steamResultFailed")}: ${err}` : t("settings.steamResultSuccess"));
    });
    return () => es.close();
  }, [t]);

  const dirty =
    !!orig &&
    (username.trim() !== orig.username ||
      installDir.trim() !== orig.installDir ||
      password !== "" ||
      branchCode !== "");
  const canSave = dirty && username.trim() !== "";

  const onSave = () =>
    save.run(async () => {
      const r = await api.putSteamConfig({
        username: username.trim(),
        password,
        branchCode,
        installDir: installDir.trim(),
      });
      if (r.ok) {
        setPassword("");
        setBranchCode("");
        await load();
      }
      return r;
    }, t("settings.toastSteamSaved"));

  const running = st.state === "running";
  const headlessStopped = status?.state === "stopped";
  const configured = !!orig && orig.hasPassword && orig.hasBranchCode && orig.username.trim() !== "";
  const canUpdate = configured && !!headlessStopped && !running && !dirty;

  let hint = "";
  if (!configured) hint = t("settings.steamUpdateHintConfig");
  else if (dirty) hint = t("settings.steamUpdateHintSave");
  else if (!headlessStopped) hint = t("settings.steamUpdateHintStopped");

  const onUpdate = () =>
    update.run(async () => {
      const r = await api.steamDownload();
      if (r.ok) {
        setLogs([]);
        setSt({ state: "running", percent: 0 });
      }
      return r;
    });
  const onCancel = () => update.run(async () => api.steamCancel());

  return (
    <InspectorCard title={t("settings.steamSection")}>
      {!orig ? (
        <Center h={60}>
          {loadFailed ? (
            <Text size="sm" c="red.6">
              {t("settings.loadError")}
            </Text>
          ) : (
            <Loader size="sm" />
          )}
        </Center>
      ) : (
        <Stack gap={8}>
          <Text size="xs" c="dimmed">
            {t("settings.steamDesc")}
          </Text>

          <FieldRow label={t("settings.steamUsername")}>
            <InspectorTextInput
              value={username}
              onChange={(e) => setUsername(e.currentTarget.value)}
              placeholder={t("settings.steamUsernamePlaceholder")}
            />
          </FieldRow>
          <FieldRow label={t("settings.steamPassword")}>
            <InspectorTextInput
              type="password"
              value={password}
              onChange={(e) => setPassword(e.currentTarget.value)}
              placeholder={orig.hasPassword ? t("settings.passwordKeep") : undefined}
            />
          </FieldRow>
          <FieldRow label={t("settings.steamBranchCode")}>
            <InspectorTextInput
              type="password"
              value={branchCode}
              onChange={(e) => setBranchCode(e.currentTarget.value)}
              placeholder={orig.hasBranchCode ? t("settings.passwordKeep") : t("settings.steamBranchCodePlaceholder")}
            />
          </FieldRow>
          <FieldRow label={t("settings.steamInstallDir")}>
            <InspectorTextInput
              value={installDir}
              onChange={(e) => setInstallDir(e.currentTarget.value)}
              placeholder={t("settings.steamInstallDirPlaceholder")}
            />
          </FieldRow>
          <SaveButton label={t("settings.save")} onClick={onSave} disabled={!canSave} loading={save.busy} />

          <Divider my={4} color="dark.4" />

          {running ? (
            <Stack gap={6}>
              <Group justify="space-between" wrap="nowrap">
                <Text size="xs" c="cyan.4" truncate>
                  {st.phase ? `${t("settings.steamUpdating")} — ${st.phase}` : t("settings.steamUpdating")}
                </Text>
                <Text size="xs" c="dimmed">
                  {Math.round(st.percent)}%
                </Text>
              </Group>
              <Progress value={st.percent} size="sm" animated />
              {st.file && (
                <Text size="xs" c="dimmed" truncate>
                  {st.file}
                </Text>
              )}
              <SteamLog logs={logs} />
              <Button fullWidth size="xs" variant="light" color="red" onClick={onCancel} loading={update.busy}>
                {t("settings.steamCancel")}
              </Button>
            </Stack>
          ) : (
            <Stack gap={6}>
              <SaveButton
                label={t("settings.steamUpdate")}
                onClick={onUpdate}
                disabled={!canUpdate}
                loading={update.busy}
              />
              {hint && (
                <Text size="xs" c="dimmed" ta="center">
                  {hint}
                </Text>
              )}
              {st.state === "success" && (
                <Text size="xs" c="teal.4" ta="center">
                  {t("settings.steamResultSuccess")}
                </Text>
              )}
              {st.state === "failed" && st.lastError && (
                <Text size="xs" c="red.5" ta="center">
                  {t("settings.steamResultFailed")}: {st.lastError}
                </Text>
              )}
              <SteamLog logs={logs} />
            </Stack>
          )}
        </Stack>
      )}
    </InspectorCard>
  );
}
