import type { ReactNode } from "react";
import { Box, Group, Text } from "@mantine/core";

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

// 1行のフィールド：左に項目名（色マーカー付き）、右に値/入力欄。モバイル優先の1列。
//   - align="start" で textarea 等の縦長コントロールに対応（ラベルを上寄せ）
export function FieldRow({
  label,
  children,
  marker = "var(--mantine-color-brand-6)",
  labelWidth = 116,
  align = "center",
}: {
  label: string;
  children: ReactNode;
  marker?: string;
  labelWidth?: number;
  align?: "center" | "start";
}) {
  return (
    <Group wrap="nowrap" gap="xs" align={align} style={{ minHeight: 34 }}>
      <Group gap={6} wrap="nowrap" style={{ width: labelWidth, flexShrink: 0, paddingTop: align === "start" ? 6 : 0 }}>
        <Box w={8} h={8} style={{ backgroundColor: marker, borderRadius: 2, flexShrink: 0 }} />
        <Text size="sm" c="dark.1" style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
          {label}
        </Text>
      </Group>
      <Box style={{ flex: 1, minWidth: 0 }}>{children}</Box>
    </Group>
  );
}
