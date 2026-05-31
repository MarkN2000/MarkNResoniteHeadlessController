import { Box, ScrollArea } from "@mantine/core";
import { SplitColumns } from "../../components/SplitColumns";
import { StartPanel } from "./StartPanel";
import { WorldSearchPanel } from "./WorldSearchPanel";

// 新規セッションタブ（docs §3.3 #3 / phase-7-spec §3.12）。2セクション構成:
//   ① StartPanel（左）     = URL / テンプレートから新ワールドを起動（機能）
//   ② WorldSearchPanel（右）= ワールド検索して起動（将来対応・disabled プレースホルダ）
// onStarted = 起動成功後にトップバーのセッション一覧を再取得（新ワールドを出現させる）。
// 右枠を今から SplitColumns で予約しておくことで、将来②を実装してもレイアウトは変わらない。
export function NewSessionTab({ onStarted }: { onStarted: () => void }) {
  return (
    <ScrollArea h="100%" type="hover">
      {/* xl 未満=1カラム縦積み / xl 以上=① 起動方法(左)・② 検索して起動(右)の2カラム。 */}
      <Box pb="md">
        <SplitColumns left={<StartPanel onStarted={onStarted} />} right={<WorldSearchPanel />} />
      </Box>
    </ScrollArea>
  );
}
