import { useCallback, useEffect, useState } from "react";
import { Box, Button, Center, Divider, Group, Loader, Progress, ScrollArea, Stack, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";
import * as api from "../../api";
import type { SteamConfig, SteamStatus, Status } from "../../api";
import { FieldRow, InspectorCard, InspectorTextInput } from "../../components/inspector";
import { useAsyncAction } from "../../hooks/useAsyncAction";
import { SaveButton } from "./SaveButton";

// ログ1行の構造化エントリ。受信時に文字列へ焼かず、レンダー時に t() で解決する
// （言語切替で表示中のログも追従し、SSE effect から t 依存を外せる＝再接続もしない）。
type LogEntry =
  | { kind: "text"; text: string }
  | { kind: "msg"; msgKey: string; msgArgs?: Record<string, string>; text?: string }
  | { kind: "result"; code?: string; text?: string; detail?: string };

type Translator = ReturnType<typeof useTranslation>["t"];

// 既知の errorCode → locale キー。動的キー連結はしない（未知 code でキー文字列が
// そのまま画面に出るのを防ぐ）。未知/無 code は steamErrText が原文へフォールバックする。
const STEAM_ERR_KEYS: Record<string, string> = {
  auth_failed: "settings.steamErrAuthFailed",
  two_factor_required: "settings.steamErrTwoFactor",
  cancelled: "settings.steamErrCancelled",
  stalled: "settings.steamErrStalled",
  verify_missing: "settings.steamErrVerifyMissing",
  acquire_failed: "settings.steamErrAcquireFailed",
  dd_failed: "settings.steamErrDDFailed",
  chmod_failed: "settings.steamErrChmodFailed",
};

// backend の Event.MsgKey → locale キー（MRHC 生成ログ行）。未知キーは原文（ja）のまま表示。
const STEAM_LOG_KEYS: Record<string, string> = {
  ddFetch: "settings.steamLogFetch",
  ddFetched: "settings.steamLogFetched",
  shaOk: "settings.steamLogShaOk",
  chmodding: "settings.steamLogChmod",
};

// 失敗表示の文言。既知 code は locale（acquire/dd/chmod 系は detail＝診断詳細を併記）、
// 未知/無 code は原文 → 汎用文言へフォールバック。
function steamErrText(t: Translator, code?: string, raw?: string, detail?: string): string {
  const key = code ? STEAM_ERR_KEYS[code] : undefined;
  if (!key) return raw || t("toast.errGeneric");
  const base = t(key);
  return detail ? `${base}: ${detail}` : base;
}

// ログ1行の表示文字列をレンダー時に解決する。
function logLineText(t: Translator, l: LogEntry): string {
  switch (l.kind) {
    case "text":
      return l.text;
    case "msg": {
      const key = STEAM_LOG_KEYS[l.msgKey];
      return key ? t(key, { ...l.msgArgs }) : (l.text ?? l.msgKey);
    }
    case "result":
      if (!l.code && !l.text) return t("settings.steamResultSuccess");
      if (l.code === "cancelled") return steamErrText(t, l.code, l.text, l.detail); // 中止は失敗扱いにしない
      return `${t("settings.steamResultFailed")}: ${steamErrText(t, l.code, l.text, l.detail)}`;
  }
}

// 進捗ログの小窓（milestone / log / result の行を表示）。進捗 % 行は流さない（多すぎるため）。
function SteamLog({ logs }: { logs: LogEntry[] }) {
  const { t } = useTranslation();
  if (logs.length === 0) return null;
  return (
    <ScrollArea h={120} type="auto">
      <Box style={{ fontFamily: "monospace", fontSize: 11, lineHeight: 1.5, whiteSpace: "pre-wrap" }}>
        {logs.map((l, i) => (
          <div key={i}>{logLineText(t, l)}</div>
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
  const [logs, setLogs] = useState<LogEntry[]>([]);

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
  // ハンドラでは t() を使わず構造化のまま積む（表示は logLineText がレンダー時に解決）＝
  // effect の依存が無く、言語切替で EventSource が張り直されない。
  useEffect(() => {
    const es = new EventSource("/api/v1/steam/events");
    const addLog = (l: LogEntry) =>
      setLogs((prev) => {
        const next = [...prev, l];
        return next.length > 200 ? next.slice(next.length - 200) : next;
      });
    es.addEventListener("steam-status", (e) => {
      // 接続（自動再接続含む）ごとの初期イベント。直後に backend が現 run のログ履歴を
      // リプレイするため、手元のログを空にして入れ替える（再接続時の二重表示防止）。
      setSt(JSON.parse((e as MessageEvent).data) as SteamStatus);
      setLogs([]);
    });
    es.addEventListener("steam-progress", (e) => {
      const d = JSON.parse((e as MessageEvent).data) as { percent?: number; file?: string };
      setSt((s) => ({ ...s, state: "running", percent: d.percent ?? 0, file: d.file }));
    });
    es.addEventListener("steam-milestone", (e) => {
      const d = JSON.parse((e as MessageEvent).data) as { text?: string };
      if (d.text) {
        setSt((s) => ({ ...s, phase: d.text }));
        addLog({ kind: "text", text: d.text });
      }
    });
    es.addEventListener("steam-log", (e) => {
      const d = JSON.parse((e as MessageEvent).data) as {
        text?: string;
        msgKey?: string;
        msgArgs?: Record<string, string>;
      };
      if (d.msgKey) addLog({ kind: "msg", msgKey: d.msgKey, msgArgs: d.msgArgs, text: d.text });
      else if (d.text) addLog({ kind: "text", text: d.text });
    });
    es.addEventListener("steam-result", (e) => {
      const d = JSON.parse((e as MessageEvent).data) as { text?: string; code?: string; detail?: string };
      const failed = !!(d.code || d.text);
      setSt((s) => ({
        ...s,
        state: failed ? "failed" : "success",
        lastError: d.text || undefined,
        errorCode: d.code,
        errorDetail: d.detail,
        percent: failed ? s.percent : 100,
      }));
      addLog({ kind: "result", code: d.code, text: d.text, detail: d.detail });
    });
    return () => es.close();
  }, []);

  // SSE は満杯時に終端 result を取りこぼし得る（pubsub 非ブロッキング）。running 中だけ
  // /steam/status を軽くポーリングし、終端（success/failed）への遷移を取りこぼさず UI 固着を防ぐ（H1）。
  useEffect(() => {
    if (st.state !== "running") return;
    let alive = true;
    const id = setInterval(async () => {
      const s = await api.getSteamStatus();
      if (!alive || !s || s.state === "running") return;
      // SSE result を取りこぼしていた場合の保険として終端状態へ反映する。
      setSt((prev) => (prev.state === "running" ? { ...prev, ...s } : prev));
    }, 4000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [st.state]);

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
          <Text size="xs" c="dimmed">
            {t("settings.steamInstallDirHint")}
          </Text>
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
              {/* 中止（cancelled）はユーザー自身の操作＝失敗ではないので中立表示にする */}
              {st.state === "failed" && st.errorCode === "cancelled" && (
                <Text size="xs" c="dimmed" ta="center">
                  {steamErrText(t, st.errorCode, st.lastError, st.errorDetail)}
                </Text>
              )}
              {st.state === "failed" && st.errorCode !== "cancelled" && st.lastError && (
                <Text size="xs" c="red.5" ta="center">
                  {t("settings.steamResultFailed")}: {steamErrText(t, st.errorCode, st.lastError, st.errorDetail)}
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
