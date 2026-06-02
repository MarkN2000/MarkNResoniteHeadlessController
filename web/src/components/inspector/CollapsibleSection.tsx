import { useState, type ReactNode } from "react";
import { Box, Collapse, Group, Text, UnstyledButton } from "@mantine/core";

// 折りたたみ可能なセクション（▾/▴ ヘッダ＋Collapse）。設定タブの上級折りたたみを汎用化（R11）。
// 上級/運用など二次的な項目群をまとめ、スマホでの縦長を抑える。既定は閉じ（defaultOpen で変更可）。
// ヘッダは設定タブの既存スタイル（xs・dimmed）を踏襲し、タブ間で見た目を統一する。
export function CollapsibleSection({
  title,
  defaultOpen = false,
  children,
}: {
  title: string;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <Box>
      <UnstyledButton onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        <Group gap={4}>
          <Text size="xs" c="dimmed">
            {title}
          </Text>
          <Text size="xs" c="dimmed">
            {open ? "▴" : "▾"}
          </Text>
        </Group>
      </UnstyledButton>
      <Collapse in={open}>
        <Box pt={4}>{children}</Box>
      </Collapse>
    </Box>
  );
}
