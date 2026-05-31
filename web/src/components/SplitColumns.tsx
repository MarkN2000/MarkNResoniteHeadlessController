import type { ReactNode } from "react";
import { Box, Flex } from "@mantine/core";

// 2パネルのレスポンシブ・レイアウト（タブ共通）。
//   - xl 未満: 1カラム（各パネル上限 PANEL_WIDTH・狭画面では画面幅まで縮小・中央寄せで縦積み）
//   - xl 以上(1408px〜): 2カラム（左/右とも PANEL_WIDTH 固定・中央寄せで横並び）
// 広い画面では上限 PANEL_WIDTH で見た目を一定に保ちつつ、560px 未満では親(ScrollArea)から
// はみ出さないようにする。ScrollArea 中身は display:table（shrink-to-fit）のため、固定 w=560 +
// maw="100%" だと table が 560px に伸びて maw が無効化される。base を w="100%"（固定床を外す）+
// maw=PANEL_WIDTH（definite な上限）に変えることで、狭画面では画面幅、広画面では 560px に収まる。
export const PANEL_WIDTH = 560;

export function SplitColumns({ left, right }: { left: ReactNode; right: ReactNode }) {
  return (
    <Flex
      direction={{ base: "column", xl: "row" }}
      gap={{ base: "xl", xl: "lg" }}
      justify="center"
      align={{ base: "center", xl: "flex-start" }}
      w="100%"
    >
      <Box w={{ base: "100%", xl: PANEL_WIDTH }} maw={PANEL_WIDTH} style={{ flexShrink: 0 }}>
        {left}
      </Box>
      <Box w={{ base: "100%", xl: PANEL_WIDTH }} maw={PANEL_WIDTH} style={{ flexShrink: 0 }}>
        {right}
      </Box>
    </Flex>
  );
}
