import type { ReactNode } from "react";
import { Box, Flex } from "@mantine/core";

// 2パネルのレスポンシブ・レイアウト（タブ共通）。
//   - xl 未満: 1カラム（各パネル幅 PANEL_WIDTH・中央寄せで縦積み）
//   - xl 以上(1408px〜): 2カラム（左/右とも PANEL_WIDTH 固定・中央寄せで横並び）
// 幅を固定することで、どのタブ・どの画面幅でもパネルの見た目が一定になる（共通化向き・可変は不採用）。
export const PANEL_WIDTH = 560;

export function SplitColumns({ left, right }: { left: ReactNode; right: ReactNode }) {
  return (
    <Flex
      direction={{ base: "column", xl: "row" }}
      gap={{ base: "md", xl: "lg" }}
      justify="center"
      align={{ base: "center", xl: "flex-start" }}
    >
      <Box w={PANEL_WIDTH} maw="100%" style={{ flexShrink: 0 }}>
        {left}
      </Box>
      <Box w={PANEL_WIDTH} maw="100%" style={{ flexShrink: 0 }}>
        {right}
      </Box>
    </Flex>
  );
}
