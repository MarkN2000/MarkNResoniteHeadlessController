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

// セッションバッジ（稼働中のみ・フォーカスボタンの leftSection）。上段=フォーカス中の worlds index
// （0始まり＝コンソールの focus N と同じ番号・brand色）、下段=/セッション総数（dimmed）。
// フォーカスカード内（dark[6] 面）に入るため背景は持たず、数字のみを縦積みで表示する。
// フォーカス対象が無い時は「−」。title（ツールチップ）は表示値と同じ idx/total からここで組み立てる。
function SessionCountBadge({ idx, total }: { idx: number | null; total: number }) {
  const { t } = useTranslation();
  const displayIdx = idx ?? "−";
  return (
    <Box
      title={t("topbar.sessionBadge", { idx: displayIdx, total })}
      style={{
        flexShrink: 0,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      <Text fz={12} fw={700} c="brand.6" lh={1.1}>
        {displayIdx}
      </Text>
      <Text fz={12} c="dark.2" lh={1.1}>
        /{total}
      </Text>
    </Box>
  );
}

// 通常停止ボタン（稼働中のみ・⋮メニューの左隣＝ヘッダー右端）。テキストを使わず赤い停止アイコン（■）の
// 正方形で、スマホ幅でも常時表示する。誤爆防止のためフォーカス切替の隣から右端へ移設し、marginLeft:auto で
// 右クラスタ（■停止＋⋮）をまとめて右へ寄せる（フォーカス切替の flex:1 が空き幅を吸う狭画面では auto は効かず
// ⋮ の直左に隣接する）。押下時の確認は呼び出し側（App.onGracefulStop）で共通 ConfirmModal を挟む。
// filled red は autoContrast={false} で白アイコンを保つ（theme.ts のテーマ規約に従う）。
function GracefulStopButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <ActionIcon
      aria-label={t("topbar.gracefulStop")}
      title={t("topbar.gracefulStop")}
      onClick={onClick}
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

// トップバー（3モード）。docs/design/phase-7-spec.md §3.2。
//   稼働中: 🎯フォーカス切替 + ■通常停止 + ⋮（強制停止・更新[P9]/言語/ログアウト）
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

  const overflowMenu = (showForceStop: boolean) => (
    <Menu position="bottom-end" withinPortal>
      {/* 停止中のみ ⋮ を右端へ寄せる auto 余白が必要。稼働中は ■停止(GracefulStopButton)の auto が、
          起動中は StartingIndicator の flex:1 が右寄せを担うので、ここでは auto を付けず ⋮ を ■停止の直右に隣接させる。 */}
      <Menu.Target>
        <ActionIcon aria-label="menu" size="lg" style={{ flexShrink: 0, marginLeft: stopped ? "auto" : undefined }}>
          <Indicator color="red" size={8} offset={-1} disabled={!updateReady}>
            <span>⋮</span>
          </Indicator>
        </ActionIcon>
      </Menu.Target>
      <Menu.Dropdown>
        {showForceStop && (
          <>
            {/* 通常停止は稼働中ヘッダーの赤い停止ボタンへ昇格したためメニューからは除外。強制停止のみ残置。 */}
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
          <Menu position="bottom-start" withinPortal width="target" onOpen={props.onRefreshSessions}>
            <Menu.Target>
              <Button
                leftSection={<SessionCountBadge idx={focused ? focused.index : null} total={props.sessions.length} />}
                rightSection="▾"
                styles={{
                  // flex:1 でヘッダーの空き幅まで伸び、minWidth:0 で狭画面では縮む。maxWidth で上限。
                  root: { flex: 1, minWidth: 0, maxWidth: FOCUS_MAX_WIDTH, height: "auto", paddingTop: 4, paddingBottom: 4 },
                  // 名前(label)を左いっぱいに広げ、0/1(leftSection)と▾(rightSection)を両端に固定する。
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
          <GracefulStopButton onClick={props.onGracefulStop} />
        </>
      )}

      {overflowMenu(!stopped)}
    </Group>
  );
}
