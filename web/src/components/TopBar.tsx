import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { ActionIcon, Box, Burger, Button, Group, Indicator, Loader, Menu, Select, Text } from "@mantine/core";
import type { ConfigSummary, Status, UpdateInfo, World } from "../api";
import { LANGUAGES, setLanguage } from "../i18n";

// フォーカスボタン（＝ドロップダウン）の最大幅。ヘッダーの空き幅まで伸ばしつつ、
// 広い画面での伸びすぎを防ぐ上限(px)。ドロップダウンは width="target" でこの実幅に追従する。
const FOCUS_MAX_WIDTH = 960;

interface TopBarProps {
  status: Status | null;
  // 稼働中
  sessions: World[];
  focusedIdx: number;
  onFocus: (idx: number) => void;
  onRefreshSessions: () => void;
  onStop: () => void;
  onGracefulStop: () => void;
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

// フォーカスボタンのセッション名表示を整える。
//   - <br> は改行に変換
//   - 改行を含む → そのまま改行・サイズ半分程度
//   - 単一行で長い → 実効幅(CJK≈2)に応じてサイズを段階的に縮小して収める（簡易ヒューリスティック）
function nameDisplay(name: string): { text: string; fontSize: number; multiline: boolean } {
  const text = name.replace(/<br\s*\/?>/gi, "\n");
  if (text.includes("\n")) return { text, fontSize: 9, multiline: true };
  const w = [...text].reduce((a, c) => a + (c.charCodeAt(0) > 0x2e80 ? 2 : 1), 0);
  const fontSize = w > 30 ? 10 : w > 22 ? 12 : w > 16 ? 14 : 16;
  return { text, fontSize, multiline: false };
}

// セッションを2行で表示（上=名前 / 下=小さい present/users/max · accessLevel）。
// フォーカスボタンとプルダウンの両方で共用（DRY）。名前は nameDisplay の規則に従い、
// 改行ありは clampLines 行で頭打ちにして親（ヘッダ高さ等）を超えないようにする。
function SessionTwoLine({ s, maxWidth, clampLines }: { s: World; maxWidth: number | string; clampLines: number }) {
  const nd = nameDisplay(s.name);
  const nameStyle: CSSProperties = {
    fontSize: nd.fontSize,
    fontWeight: 600,
    color: "var(--mantine-color-dark-0)",
    ...(nd.multiline
      ? {
          whiteSpace: "pre-line",
          display: "-webkit-box",
          WebkitBoxOrient: "vertical",
          WebkitLineClamp: clampLines,
          overflow: "hidden",
        }
      : { whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", maxWidth: "100%" }),
  };
  return (
    <span
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "flex-start",
        lineHeight: 1.15,
        maxWidth,
        // 文字列(="100%")指定時は親(ドロップダウン項目)幅いっぱいに広げ、
        // minWidth:0 で flex 内でも省略表示(ellipsis)が効くようにする。
        width: typeof maxWidth === "string" ? "100%" : undefined,
        minWidth: 0,
        overflow: "hidden",
      }}
    >
      <span style={nameStyle}>{nd.text}</span>
      <span style={{ fontSize: 10, color: "var(--mantine-color-dark-2)", whiteSpace: "nowrap" }}>
        {s.present}/{s.users}/{s.maxUsers} · {s.accessLevel}
      </span>
    </span>
  );
}

// セッションバッジ（稼働中のみ・フォーカスプルダウンの左）。上段=フォーカス中の worlds index
// （0始まり＝コンソールの focus N と同じ番号・brand色）、下段=/セッション総数（dimmed）。
// 正方形でスペースを取らない（スマホ/PC共通・UI改善①）。フォーカス対象が無い時は「−」。
function SessionCountBadge({ idx, total, title }: { idx: number | null; total: number; title: string }) {
  return (
    <Box
      title={title}
      style={{
        width: 36,
        height: 36,
        flexShrink: 0,
        borderRadius: "var(--mantine-radius-md)",
        backgroundColor: "var(--mantine-color-dark-6)",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      <Text fz={13} fw={700} c="brand.6" lh={1.1}>
        {idx ?? "−"}
      </Text>
      <Text fz={9} c="dark.2" lh={1.1}>
        /{total}
      </Text>
    </Box>
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

// トップバー（3モード）。docs/design/phase-7-spec.md §3.2。
//   稼働中: 🎯フォーカス切替 + ⋮（通常停止/強制停止・更新[P9]/言語/ログアウト）
//   起動中: ⟳ 起動中… N秒 + ⋮（強制停止のみ・更新[P9]/言語/ログアウト）
//   停止中: [起動] + config選択 + ⋮（更新[P9]/言語/ログアウト）
export function TopBar(props: TopBarProps) {
  const { t, i18n } = useTranslation();
  const state = props.status?.state ?? "stopped";
  const stopped = state === "stopped";
  const starting = state === "starting";

  // 更新あり or 再起動待ち → ⋮ に赤丸（気づきの導線。チェックはログイン時1回＋メニューを開いた時）。
  const upd = props.updateInfo;
  const updateReady = !!upd && (!!upd.staged || upd.updateAvailable);

  const overflowMenu = (showForceStop: boolean, showGracefulStop: boolean) => (
    <Menu position="bottom-end" withinPortal>
      <Menu.Target>
        <ActionIcon aria-label="menu" size="lg" style={{ flexShrink: 0, marginLeft: "auto" }}>
          <Indicator color="red" size={8} offset={-1} disabled={!updateReady}>
            <span>⋮</span>
          </Indicator>
        </ActionIcon>
      </Menu.Target>
      <Menu.Dropdown>
        {showForceStop && (
          <>
            {showGracefulStop && (
              <Menu.Item onClick={props.onGracefulStop}>{t("topbar.gracefulStop")}</Menu.Item>
            )}
            <Menu.Item color="red" onClick={props.onStop}>
              {t("topbar.forceStop")}
            </Menu.Item>
            <Menu.Divider />
          </>
        )}
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

  const focused = props.sessions.find((s) => s.index === props.focusedIdx);

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
        <StartingIndicator startedAt={props.status?.startedAt} />
      ) : (
        <>
          <SessionCountBadge
            idx={focused ? focused.index : null}
            total={props.sessions.length}
            title={t("topbar.sessionBadge", { idx: focused ? focused.index : "−", total: props.sessions.length })}
          />
          <Menu position="bottom-start" withinPortal width="target" onOpen={props.onRefreshSessions}>
            <Menu.Target>
              <Button
                rightSection="▾"
                styles={{
                  // flex:1 でヘッダーの空き幅まで伸び、minWidth:0 で狭画面では縮む。maxWidth で上限。
                  root: { flex: 1, minWidth: 0, maxWidth: FOCUS_MAX_WIDTH, height: "auto", paddingTop: 4, paddingBottom: 4 },
                  // 名前(label)を左いっぱいに広げ、▾(section)を右端へ押し出す。
                  inner: { width: "100%" },
                  label: { flex: 1, minWidth: 0, overflow: "hidden" },
                  section: { flexShrink: 0 },
                }}
              >
                {focused ? <SessionTwoLine s={focused} maxWidth="100%" clampLines={2} /> : t("topbar.noSession")}
              </Button>
            </Menu.Target>
            <Menu.Dropdown>
              {props.sessions.length === 0 && <Menu.Item disabled>{t("topbar.noSession")}</Menu.Item>}
              {props.sessions.map((s) => (
                <Menu.Item key={s.index} onClick={() => props.onFocus(s.index)}>
                  {/* 左=index番号（バッジと同じ採番・フォーカス中はbrand色）、右=既存の2行表示 */}
                  <Group gap={8} wrap="nowrap">
                    <Text
                      size="xs"
                      fw={700}
                      c={s.index === props.focusedIdx ? "brand.6" : "dimmed"}
                      style={{ flexShrink: 0, minWidth: 16, textAlign: "right" }}
                    >
                      {s.index}
                    </Text>
                    <Box style={{ flex: 1, minWidth: 0 }}>
                      <SessionTwoLine s={s} maxWidth="100%" clampLines={3} />
                    </Box>
                  </Group>
                </Menu.Item>
              ))}
            </Menu.Dropdown>
          </Menu>
        </>
      )}

      {overflowMenu(!stopped, !stopped && !starting)}
    </Group>
  );
}
