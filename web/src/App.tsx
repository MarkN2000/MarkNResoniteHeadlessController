import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Alert, AppShell, Box, Button, Center, Group, NavLink, Stack, Text } from "@mantine/core";
import * as api from "./api";
import type { ConfigSummary, LogLine, Status, UpdateInfo, World } from "./api";
import { notifyError, notifyInfo } from "./lib/notify";
import { TABS, type TabId } from "./nav";
import { SURFACE } from "./theme";
import { Login } from "./components/Login";
import { TopBar } from "./components/TopBar";
import { AccountSetupModal } from "./components/AccountSetupModal";
import { UpdateModal } from "./components/UpdateModal";
import { ConfirmHost } from "./components/ConfirmHost";
import { useConfirm } from "./hooks/useConfirm";
import { ShutdownScreen } from "./components/ShutdownScreen";
import { CommandTab } from "./tabs/CommandTab";
import { StartPrompt, TabPlaceholder } from "./tabs/Placeholder";
import { SessionTab } from "./tabs/session/SessionTab";
import { FriendsTab } from "./tabs/friends/FriendsTab";
import { NewSessionTab } from "./tabs/newsession/NewSessionTab";
import { ConfigTab } from "./tabs/config/ConfigTab";
import { SettingsTab } from "./tabs/settings/SettingsTab";
import { ScheduleTab } from "./tabs/schedule/ScheduleTab";
import { LogsTab } from "./tabs/logs/LogsTab";

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  // MRHC 終了依頼後の静止画面（自己更新の「今すぐ終了」）。Shell ごと差し替えて
  // SSE 購読等を unmount する（サーバー停止後の再接続ループを残さない）。
  const [shutdownInfo, setShutdownInfo] = useState<UpdateInfo | null>(null);

  // 初回: 既存 Cookie セッションを確認
  useEffect(() => {
    api.getStatus().then((s) => setAuthed(s !== null));
  }, []);

  if (authed === null) {
    return (
      <Center h="100%">
        <Text c="dimmed">…</Text>
      </Center>
    );
  }
  if (!authed) return <Login onAuthed={() => setAuthed(true)} />;
  if (shutdownInfo) return <ShutdownScreen info={shutdownInfo} />;
  return <Shell onLogout={() => setAuthed(false)} onShutdownDone={setShutdownInfo} />;
}

function Shell({ onLogout, onShutdownDone }: { onLogout: () => void; onShutdownDone: (info: UpdateInfo) => void }) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<Status | null>(null);
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [sessions, setSessions] = useState<World[]>([]);
  const [configs, setConfigs] = useState<ConfigSummary[]>([]);
  const [selectedConfig, setSelectedConfig] = useState<string | null>(null);
  const [focusedIdx, setFocusedIdx] = useState(0);
  const [activeTab, setActiveTab] = useState<TabId>("session");
  const [navOpened, setNavOpened] = useState(false);
  const seenSeq = useRef<Set<number>>(new Set());
  // 中央 Resonite アカウントの設定状態（初回モーダル + 未設定バナーを駆動）。
  // null=未取得/取得失敗（バナー出さず）。取得成功して username+password が揃えば false。
  const [credUnset, setCredUnset] = useState<boolean | null>(null);
  const [setupOpen, setSetupOpen] = useState(false);
  const setupShown = useRef(false);
  // 自己更新: ログイン後に1回だけチェックして ⋮ の赤丸とメニュー表示を駆動
  // （常時ポーリングはしない・docs/design/self-update.md）。失敗（null）はバッジを出さないだけ。
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [updateOpen, setUpdateOpen] = useState(false);
  useEffect(() => {
    void api.checkUpdate().then(setUpdateInfo);
  }, []);

  const running = status?.state === "running";
  // 起動できない致命要因（duplicate_instance 等）を SSE status で受けたら、新規発生時に1回だけ赤トースト。
  // 起動失敗は非同期（プロセスが起動直後に落ちる）ため /start の応答では拾えず status.fault で通知される。
  const lastFault = useRef("");
  useEffect(() => {
    const f = status?.fault ?? "";
    if (f && f !== lastFault.current && f === "duplicate_instance") {
      notifyError(t("toast.errDuplicateInstance"), t("toast.startFailTitle"));
    }
    lastFault.current = f;
  }, [status, t]);

  // SSE（status + log）をシェル最上位で購読し、トップバーのモードとログを駆動。
  useEffect(() => {
    const es = new EventSource("/api/v1/events");
    es.addEventListener("status", (e) => setStatus(JSON.parse((e as MessageEvent).data) as Status));
    es.addEventListener("log", (e) => {
      const line = JSON.parse((e as MessageEvent).data) as LogLine;
      if (seenSeq.current.has(line.seq)) return;
      seenSeq.current.add(line.seq);
      setLogs((prev) => {
        const next = [...prev, line];
        return next.length > 1000 ? next.slice(next.length - 1000) : next;
      });
    });
    return () => es.close();
  }, []);

  // 停止中トップバーの config 選択肢。コンフィグタブの CRUD 後にも再取得する（ConfigTab に渡す）。
  const refreshConfigs = useCallback(() => {
    Promise.all([api.getConfigs(), api.getLastUsedConfig()]).then(([cs, last]) => {
      setConfigs(cs);
      // 現選択が消えていたら last-used→先頭に繰り上げ（削除/リネーム後の dangling 防止）。
      setSelectedConfig((cur) =>
        cur && cs.some((c) => c.name === cur)
          ? cur
          : cs.some((c) => c.name === last)
            ? last
            : (cs[0]?.name ?? null),
      );
    });
  }, []);
  useEffect(() => {
    refreshConfigs();
  }, [refreshConfigs]);

  const refreshSessions = useCallback(() => {
    api.getSessions().then(setSessions);
  }, []);

  // アカウント設定状態の再評価（マウント時＋設定タブ/モーダルでの保存後）。
  // 取得失敗（null）はバナーを出さない（一時的なエラーで誤って煽らない）。
  const refreshCred = useCallback(() => {
    void api.getCredentials().then((c) => {
      if (c) setCredUnset(!(c.username.trim() !== "" && c.hasPassword));
    });
  }, []);
  useEffect(() => {
    refreshCred();
  }, [refreshCred]);
  // 未設定を検知したら初回のみモーダルを自動表示（閉じてもバナーは残る）。
  useEffect(() => {
    if (credUnset === true && !setupShown.current) {
      setupShown.current = true;
      setSetupOpen(true);
    }
  }, [credUnset]);

  // 稼働開始でセッション取得、停止でクリア。
  useEffect(() => {
    if (running) refreshSessions();
    else setSessions([]);
  }, [running, refreshSessions]);

  // トップバー稼働中の赤い停止ボタン用・共通確認。誤タップ対策にワンクッション挟む（R7 で無確認だったが
  // 常時表示ボタンへ昇格したため確認を追加）。確定で /stop/graceful 受付＝進行/中止はスケジュールタブに出る。
  // 成功/失敗トーストは useConfirm.confirm() が WriteResult を reportWriteResult へ流して処理する。
  const stopConfirm = useConfirm();
  function onGracefulStop() {
    stopConfirm.ask({
      title: t("topbar.gracefulStop"),
      message: t("topbar.gracefulStopConfirm"),
      danger: true,
      success: t("topbar.gracefulStopAccepted"),
      onConfirm: () => api.gracefulStop(),
    });
  }

  async function onStart() {
    if (!selectedConfig) return;
    // 起動失敗は write 操作とは別系統（WriteResult を通さない）。赤トーストで明示する（7-7 第1層）。
    // api.start は通信不通で throw し得るため try/catch で network も拾う。
    try {
      const r = await api.start(selectedConfig);
      if (!r.ok) {
        // 未DL（既定パスに Resonite が無い）は専用文言で取得導線を案内（R-A）。
        const msg =
          r.code === "headless_not_installed" ? t("toast.errHeadlessNotInstalled") : r.error || t("toast.errGeneric");
        notifyError(msg, t("toast.startFailTitle"));
      } else if (r.runtimePrepare) {
        // .NET ランタイムの設置を伴う起動受付（進捗は設定タブ・結果はコンソールの sys ログ）。
        notifyInfo(t("toast.startRuntimePrepare"));
      }
    } catch {
      notifyError(t("toast.errNetwork"), t("toast.startFailTitle"));
    }
  }

  function renderTab() {
    const def = TABS.find((tb) => tb.id === activeTab)!;
    const stopped = (status?.state ?? "stopped") === "stopped";
    if (!def.availableWhenStopped && stopped) return <StartPrompt />;
    if (activeTab === "session")
      return running ? <SessionTab idx={focusedIdx} selfUserId={status?.loginUserId ?? null} /> : <StartPrompt />;
    if (activeTab === "friends")
      return running ? <FriendsTab idx={focusedIdx} selfUserId={status?.loginUserId ?? null} /> : <StartPrompt />;
    if (activeTab === "newSession") return running ? <NewSessionTab onStarted={refreshSessions} /> : <StartPrompt />;
    if (activeTab === "command") return <CommandTab logs={logs} onSend={(c) => void api.sendCommand(c)} />;
    if (activeTab === "config") return <ConfigTab onConfigsChanged={refreshConfigs} />;
    if (activeTab === "settings") return <SettingsTab onCredentialsChanged={refreshCred} status={status} />;
    if (activeTab === "schedule") return <ScheduleTab running={running} configs={configs} />;
    if (activeTab === "logs") return <LogsTab />;
    return <TabPlaceholder titleKey={def.labelKey} />;
  }

  return (
    <>
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 220, breakpoint: "sm", collapsed: { mobile: !navOpened } }}
      padding="md"
      withBorder={false}
    >
      <AppShell.Header>
        <TopBar
          status={status}
          sessions={sessions}
          focusedIdx={focusedIdx}
          onFocus={(idx) => setFocusedIdx(idx)}
          onRefreshSessions={refreshSessions}
          onStop={() => void api.stop()}
          onGracefulStop={onGracefulStop}
          configs={configs}
          selectedConfig={selectedConfig}
          onSelectConfig={setSelectedConfig}
          onStart={onStart}
          onLogout={async () => {
            await api.logout();
            onLogout();
          }}
          navOpened={navOpened}
          onToggleNav={() => setNavOpened((o) => !o)}
          updateInfo={updateInfo}
          onOpenUpdate={() => setUpdateOpen(true)}
        />
      </AppShell.Header>

      <AppShell.Navbar p="xs" style={{ backgroundColor: SURFACE.sidebarBg }}>
        {TABS.map((tab) => {
          const isActive = activeTab === tab.id;
          return (
            <NavLink
              key={tab.id}
              active={isActive}
              label={t(tab.labelKey)}
              onClick={() => {
                setActiveTab(tab.id);
                setNavOpened(false);
              }}
              styles={{
                root: { backgroundColor: isActive ? SURFACE.navActiveBg : "transparent" },
                label: {
                  // 選択=yellow(Hero) / 未選択=Light（パレット色は Mantine 変数で参照）
                  color: isActive ? "var(--mantine-color-yellow-6)" : "var(--mantine-color-dark-0)",
                  fontWeight: isActive ? 600 : 400,
                },
              }}
            />
          );
        })}
      </AppShell.Navbar>

      <AppShell.Main style={{ height: "100dvh" }}>
        <Stack h="100%" gap={0}>
          {/* Resonite アカウント未設定の常設バナー（事実のみ・帰結文は付けない）。全タブで表示。 */}
          {credUnset === true && (
            <Alert variant="light" color="orange" radius={0} p="xs" styles={{ message: { width: "100%" } }}>
              <Group justify="space-between" wrap="nowrap" gap="sm">
                <Text size="sm">{t("settings.bannerUnset")}</Text>
                <Button size="xs" variant="default" style={{ flexShrink: 0 }} onClick={() => setSetupOpen(true)}>
                  {t("settings.bannerAction")}
                </Button>
              </Group>
            </Alert>
          )}
          <Box style={{ flex: 1, minHeight: 0 }}>{renderTab()}</Box>
        </Stack>
      </AppShell.Main>
    </AppShell>

    {/* トップバーの通常停止ボタン用の共通確認モーダル（赤い確定ボタン）。 */}
    <ConfirmHost confirm={stopConfirm} />

    {/* 初回オンボーディング: アカウント未設定時にログイン直後 1 回自動表示（バナーからも開ける）。 */}
    <AccountSetupModal opened={setupOpen} onClose={() => setSetupOpen(false)} onSaved={refreshCred} />

    {/* 自己更新（⋮ メニューから。適用→再起動手順／「今すぐ終了」→ App が静止画面へ差し替え）。 */}
    <UpdateModal
      opened={updateOpen}
      onClose={() => setUpdateOpen(false)}
      info={updateInfo}
      onInfoChange={setUpdateInfo}
      onShutdownDone={onShutdownDone}
    />
    </>
  );
}
