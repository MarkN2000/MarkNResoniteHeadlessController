import type { ReactNode } from "react";
import { Box, Group, Text } from "@mantine/core";

// Resonite シーンインスペクタ風のカード。
//   - ヘッダ行 = グレーのタイトルバー（中央=hero/yellow）＋ 右隣に独立した別ボックスのアクション。
//     参考画像の複製/削除アイコンと同じく、タイトルバーとアクションは隙間を空けた別ボックスにする
//     （タイトルバーに重ねて一体化させない）。
//   - 本文 = FieldRow を縦に積む（1列・モバイル優先・背景と同色）。
export function InspectorCard({
  title,
  actions,
  children,
}: {
  title: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Box>
      <Group wrap="nowrap" gap={6} mb="xs" align="center">
        <Box
          bg="dark.6"
          px="sm"
          style={{
            flex: 1,
            minWidth: 0,
            height: 34,
            borderRadius: "var(--mantine-radius-md)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <Text fw={700} size="sm" ta="center" c="yellow.6" truncate>
            {title}
          </Text>
        </Box>
        {actions}
      </Group>
      <Box px="xs">{children}</Box>
    </Box>
  );
}
