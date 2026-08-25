import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ActionIcon, Box, Burger, Button, Group, Indicator, Loader, Menu, Select, Text } from "@mantine/core";
import type { ConfigSummary, Status, UpdateInfo } from "../api";
import { useSystemMetrics } from "../hooks/useSystemMetrics";
import { LANGUAGES, setLanguage } from "../i18n";

interface TopBarProps {
  status: Status | null;
  onStop: () => void;
  // 停止中
  configs: ConfigSummary[];
  selectedConfig: string | null;
  onSelectConfig: (name: string | null) => void;
  onStart: () => void;
  // 共通
  onLogout: () => void;
  navOpened: boolean;
  onToggleNav: () => void;
  // 自己更新（ログイン時チェックの結果＝⋮ の赤丸とメニュー文言を駆動。null は未取得/失敗）
  updateInfo: UpdateInfo | null;
  onOpenUpdate: () => void;
}

// 停止ボタン（稼働中/起動中・⋮メニューの左隣＝ヘッダー右端）。テキストを使わず赤い停止アイコン（■）の
// 正方形で、スマホ幅でも常時表示する。誤爆防止のためフォーカス切替の隣から右端へ移設し、marginLeft:auto で
// 右クラスタ（■停止＋⋮）をまとめて右へ寄せる（フォーカス切替の flex:1 が空き幅を吸う狭画面では auto は効かず
// ⋮ の直左に隣接する）。押下時の確認は呼び出し側（App.onStop）で共通 ConfirmModal を挟む。
// filled red は autoContrast={false} で白アイコンを保つ（theme.ts のテーマ規約に従う）。
function StopButton({ onClick, disabled = false }: { onClick: () => void; disabled?: boolean }) {
  const { t } = useTranslation();
  return (
    <ActionIcon
      aria-label={t("topbar.stop")}
      title={t("topbar.stop")}
      onClick={onClick}
      disabled={disabled}
      variant="filled"
      color="red"
      autoContrast={false}
      size={36}
      style={{ flexShrink: 0, marginLeft: "auto" }}
    >
      <span style={{ fontSize: 16, lineHeight: 1 }}>■</span>
    </ActionIcon>
  );
}

// 起動中インジケータ（state==="starting" の間だけマウント）。経過秒は startedAt から算出し、
// 自前の1秒 interval で更新する（このコンポーネントだけ再描画＝不要時はタイマーも止まる）。
// startedAt 欠落時は秒を出さず「起動中…」のみ。クロックスキューで負値にならないよう 0 でクランプ。
function StartingIndicator({ startedAt }: { startedAt?: string }) {
  const { t } = useTranslation();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const sec = startedAt ? Math.max(0, Math.floor((now - new Date(startedAt).getTime()) / 1000)) : null;
  return (
    <Group gap="xs" wrap="nowrap" style={{ flex: 1, minWidth: 0 }}>
      <Loader size="sm" />
      <Text size="sm" c="dimmed" style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
        {sec === null ? t("topbar.starting") : t("topbar.startingElapsed", { sec })}
      </Text>
    </Group>
  );
}

function SystemUsage() {
  const { t } = useTranslation();
  const metrics = useSystemMetrics();
  const ready = metrics?.supported === true;

  return (
    <Group gap="xs" wrap="nowrap" style={{ flexShrink: 0 }}>
      <Text size="xs" c="dimmed" style={{ whiteSpace: "nowrap" }}>
        {t("topbar.cpuShort")}{" "}
        <Text component="span" size="xs" c="dark.0" fw={600}>
          {ready ? `${Math.round(metrics.cpuPercent)}%` : "—"}
        </Text>
      </Text>
      <Text size="xs" c="dimmed" style={{ whiteSpace: "nowrap" }}>
        {t("topbar.memoryShort")}{" "}
        <Text component="span" size="xs" c="dark.0" fw={600}>
          {ready ? `${Math.round(metrics.memPercent)}%` : "—"}
        </Text>
      </Text>
    </Group>
  );
}

function RunningIndicator() {
  const { t } = useTranslation();
  return (
    <Group gap="xs" wrap="nowrap" style={{ flex: 1, minWidth: 0 }}>
      <Box
        aria-hidden="true"
        style={{ width: 8, height: 8, borderRadius: "50%", flexShrink: 0, backgroundColor: "var(--mantine-color-green-6)" }}
      />
      <Text size="sm" truncate visibleFrom="sm">
        {t("topbar.running")}
      </Text>
      <SystemUsage />
    </Group>
  );
}

function StoppingIndicator() {
  const { t } = useTranslation();
  return (
    <Group gap="xs" wrap="nowrap" style={{ flex: 1, minWidth: 0 }}>
      <Loader size="sm" color="yellow" />
      <Text size="sm" c="dimmed" truncate>
        {t("topbar.stopping")}
      </Text>
    </Group>
  );
}

// トップバー（3モード）。docs/design/phase-7-spec.md §3.2。
//   稼働中: 状態表示 + ■停止 + ⋮（更新[P9]/言語/ログアウト）
//   起動中: ⟳ 起動中… N秒 + ■停止 + ⋮（更新[P9]/言語/ログアウト）
//   終了処理中: ⟳ 停止中 + ■無効 + ⋮（更新[P9]/言語/ログアウト）
//   停止済み: [起動] + config選択 + ⋮（更新[P9]/言語/ログアウト）
export function TopBar(props: TopBarProps) {
  const { t, i18n } = useTranslation();
  const state = props.status?.state ?? "stopped";
  const stopped = state === "stopped";
  const starting = state === "starting";
  const stopping = state === "stopping";

  // 更新あり or 再起動待ち → ⋮ に赤丸（気づきの導線。チェックはログイン時1回＋メニューを開いた時）。
  const upd = props.updateInfo;
  const updateReady = !!upd && (!!upd.staged || upd.updateAvailable);

  const overflowMenu = () => (
    <Menu position="bottom-end" withinPortal>
      {/* ヘッドレス停止中のみ ⋮ を右端へ寄せる auto 余白が必要。それ以外は ■停止(StopButton)の auto が
          右寄せを担うので、ここでは auto を付けず ⋮ を ■停止の直右に隣接させる。 */}
      <Menu.Target>
        <ActionIcon aria-label="menu" size="lg" style={{ flexShrink: 0, marginLeft: stopped ? "auto" : undefined }}>
          <Indicator color="red" size={8} offset={-1} disabled={!updateReady}>
            <span>⋮</span>
          </Indicator>
        </ActionIcon>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Item
          onClick={props.onOpenUpdate}
          rightSection={updateReady ? <span style={{ color: "var(--mantine-color-red-6)" }}>●</span> : undefined}
        >
          {upd?.staged ? t("topbar.updatePending", { version: upd.staged }) : t("topbar.checkUpdate")}
        </Menu.Item>
        <Menu.Divider />
        <Menu.Label>{t("menu.language")}</Menu.Label>
        {LANGUAGES.map((l) => (
          <Menu.Item
            key={l.code}
            onClick={() => setLanguage(l.code)}
            rightSection={i18n.language === l.code ? "✓" : undefined}
          >
            {l.label}
          </Menu.Item>
        ))}
        <Menu.Divider />
        <Menu.Item onClick={props.onLogout}>{t("dashboard.logout")}</Menu.Item>
      </Menu.Dropdown>
    </Menu>
  );

  return (
    <Group h="100%" px="md" gap="sm" wrap="nowrap">
      <Burger opened={props.navOpened} onClick={props.onToggleNav} hiddenFrom="sm" size="sm" style={{ flexShrink: 0 }} />
      <Text fw={700} visibleFrom="sm">
        MRHC
      </Text>

      {stopped ? (
        <>
          <Button onClick={props.onStart} disabled={!props.selectedConfig} style={{ flexShrink: 0 }}>
            {t("dashboard.start")}
          </Button>
          {props.configs.length > 0 ? (
            <Select
              data={props.configs.map((c) => ({ value: c.name, label: c.name }))}
              value={props.selectedConfig}
              onChange={props.onSelectConfig}
              placeholder={t("topbar.selectConfig")}
              allowDeselect={false}
              w={200}
              miw={0}
              comboboxProps={{ withinPortal: true }}
            />
          ) : (
            <Text c="dimmed" size="sm">
              {t("topbar.noConfig")}
            </Text>
          )}
        </>
      ) : starting ? (
        <>
          <StartingIndicator startedAt={props.status?.startedAt} />
          <StopButton onClick={props.onStop} />
        </>
      ) : stopping ? (
        <>
          <StoppingIndicator />
          <StopButton onClick={props.onStop} disabled />
        </>
      ) : (
        <>
          <RunningIndicator />
          <StopButton onClick={props.onStop} />
        </>
      )}

      {overflowMenu()}
    </Group>
  );
}
