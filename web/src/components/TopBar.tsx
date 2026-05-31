import type { CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { ActionIcon, Burger, Button, Group, Menu, Select, Text } from "@mantine/core";
import type { ConfigSummary, Status, World } from "../api";
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
  // 停止中
  configs: ConfigSummary[];
  selectedConfig: string | null;
  onSelectConfig: (name: string | null) => void;
  onStart: () => void;
  // 共通
  onLogout: () => void;
  navOpened: boolean;
  onToggleNav: () => void;
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

// トップバー（2モード）。docs/design/phase-7-spec.md §3.2。
//   稼働中: 🎯フォーカス切替 + ⋮（強制停止/更新[P9]/言語/ログアウト）
//   停止中: [起動] + config選択 + ⋮（更新[P9]/言語/ログアウト）
export function TopBar(props: TopBarProps) {
  const { t, i18n } = useTranslation();
  const stopped = (props.status?.state ?? "stopped") === "stopped";

  const overflowMenu = (showForceStop: boolean) => (
    <Menu position="bottom-end" withinPortal>
      <Menu.Target>
        <ActionIcon aria-label="menu" size="lg" style={{ flexShrink: 0, marginLeft: "auto" }}>
          ⋮
        </ActionIcon>
      </Menu.Target>
      <Menu.Dropdown>
        {showForceStop && (
          <>
            <Menu.Item color="red" onClick={props.onStop}>
              {t("topbar.forceStop")}
            </Menu.Item>
            <Menu.Divider />
          </>
        )}
        <Menu.Item disabled>{t("topbar.checkUpdate")}</Menu.Item>
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
      ) : (
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
                <SessionTwoLine s={s} maxWidth="100%" clampLines={3} />
              </Menu.Item>
            ))}
          </Menu.Dropdown>
        </Menu>
      )}

      {overflowMenu(!stopped)}
    </Group>
  );
}
