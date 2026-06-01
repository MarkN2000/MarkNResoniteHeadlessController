import type { ReactNode } from "react";
import { Box, Group, Text, UnstyledButton } from "@mantine/core";

// インスペクタ風の入力欄の共通スタイル。
// ルール: 縁取りは「キーボードで文字入力できる」欄のみ（TextInput/NumberInput/Textarea）。
// プルダウン（Select）は縁取りなし・グレー fill のみ。読み取り専用はプレーン Text で区別。

// 文字入力欄（キーボード入力可）：グレー fill＋縁取り・中央寄せ。
export const FIELD_INPUT_STYLES = {
  input: {
    backgroundColor: "var(--mantine-color-dark-6)",
    border: "2px solid var(--mantine-color-dark-3)",
    textAlign: "center" as const,
  },
};

// 複数行入力（説明）用：グレー fill＋縁取り、左寄せ（複数行テキストは中央寄せが読みにくいため）。
export const FIELD_TEXTAREA_STYLES = {
  input: {
    backgroundColor: "var(--mantine-color-dark-6)",
    border: "2px solid var(--mantine-color-dark-3)",
  },
};

// プルダウン（Select）：グレー fill・縁取りなし・中央寄せ（キーボード入力しないため縁取り対象外）。
export const FIELD_SELECT_STYLES = {
  input: {
    backgroundColor: "var(--mantine-color-dark-6)",
    border: "none",
    textAlign: "center" as const,
  },
};

// Select 右端のアイコン。既定の chevron(⌄) を下向きの正三角形(▼)1つに差し替える。
// rightSectionPointerEvents="none" と併用して、入力欄クリックでドロップダウンが開くようにする。
export const SELECT_DOWN_ICON = (
  <span style={{ fontSize: 10, lineHeight: 1, color: "var(--mantine-color-dark-1)" }}>▼</span>
);

// 項目マーカー（ハンドル）の形状。背景色はボタン時のみ CSS クラス側で持つ（hover 切替を効かせるため）。
const MARKER_SHAPE = {
  width: 15,
  height: 18,
  flexShrink: 0,
  borderRadius: 3,
  display: "flex",
  alignItems: "stretch",
  overflow: "hidden",
} as const;

// ハンドル中身: 左に色付き縦バー（種別色＝marker）＋ 右に白い横3本線（ドラッグハンドル風）。
function MarkerInner({ color }: { color: string }) {
  return (
    <>
      <Box style={{ width: 3, flexShrink: 0, backgroundColor: color }} />
      <Box
        style={{ flex: 1, display: "flex", flexDirection: "column", justifyContent: "center", gap: 2.5, padding: "0 2px" }}
      >
        <Box style={{ height: 1.5, borderRadius: 1, backgroundColor: "var(--mantine-color-dark-0)" }} />
        <Box style={{ height: 1.5, borderRadius: 1, backgroundColor: "var(--mantine-color-dark-0)" }} />
        <Box style={{ height: 1.5, borderRadius: 1, backgroundColor: "var(--mantine-color-dark-0)" }} />
      </Box>
    </>
  );
}

// マーカー本体。onClick ありでボタン化（hover で背景明色化・キーボード操作可）、なしは見た目だけの非操作ハンドル。
// 背景色は inline だと :hover を表現できないため、ボタン時はクラス（index.css の .mrhc-field-marker）で持つ。
function FieldMarker({ color, onClick, label }: { color: string; onClick?: () => void; label?: string }) {
  if (onClick) {
    return (
      <UnstyledButton
        type="button"
        onClick={onClick}
        aria-label={label}
        title={label}
        className="mrhc-field-marker"
        style={{ ...MARKER_SHAPE, cursor: "pointer" }}
      >
        <MarkerInner color={color} />
      </UnstyledButton>
    );
  }
  return (
    <Box style={{ ...MARKER_SHAPE, backgroundColor: "var(--mantine-color-dark-5)" }}>
      <MarkerInner color={color} />
    </Box>
  );
}

// 1行のフィールド：左に項目名（色マーカー付き）、右に値/入力欄。モバイル優先の1列。
//   - align="start" で textarea 等の縦長コントロールに対応（ラベルを上寄せ）
//   - onMarkerClick を渡すとマーカーがボタンになる（markerLabel=アクセシブル名/ツールチップ）。主用途は「既定値に戻す」。
export function FieldRow({
  label,
  children,
  marker = "var(--mantine-color-brand-6)",
  labelWidth = 175,
  align = "center",
  onMarkerClick,
  markerLabel,
}: {
  label: string;
  children: ReactNode;
  marker?: string;
  labelWidth?: number;
  align?: "center" | "start";
  onMarkerClick?: () => void;
  markerLabel?: string;
}) {
  return (
    <Group wrap="nowrap" gap="xs" align={align} style={{ minHeight: 34 }}>
      <Group gap={6} wrap="nowrap" style={{ width: labelWidth, flexShrink: 0, paddingTop: align === "start" ? 6 : 0 }}>
        <FieldMarker color={marker} onClick={onMarkerClick} label={markerLabel} />
        <Text size="sm" c="dark.1" style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
          {label}
        </Text>
      </Group>
      <Box style={{ flex: 1, minWidth: 0 }}>{children}</Box>
    </Group>
  );
}
