import { Box, ScrollArea } from "@mantine/core";
import { SplitColumns } from "../../components/SplitColumns";
import { StartPanel } from "./StartPanel";
import { WorldSearchPanel } from "./WorldSearchPanel";
import { useFavorites } from "./useFavorites";

// 新規セッションタブ（docs §3.3 #3 / phase-7-spec §3.12）。2セクション構成:
//   ① StartPanel（左）     = URL / テンプレートから新ワールドを起動（URL欄で★お気に入り登録）
//   ② WorldSearchPanel（右）= go.resonite.com を検索して結果から起動
// onStarted = 起動成功後にトップバーのセッション一覧を再取得（新ワールドを出現させる）。両枠で共有。
// お気に入りは useFavorites を親で一度だけ呼び両枠へ配布（単一の真実源・★状態を即時同期）。
export function NewSessionTab({ onStarted }: { onStarted: () => void }) {
  const { favorites, isFavorited, toggle } = useFavorites();
  return (
    <ScrollArea h="100%" type="hover">
      {/* xl 未満=1カラム縦積み / xl 以上=① 起動方法(左)・② 検索して起動(右)の2カラム。 */}
      <Box pb="md">
        <SplitColumns
          left={<StartPanel onStarted={onStarted} isFavorited={isFavorited} onToggleFavorite={toggle} />}
          right={
            <WorldSearchPanel
              onStarted={onStarted}
              favorites={favorites}
              isFavorited={isFavorited}
              onToggleFavorite={toggle}
            />
          }
        />
      </Box>
    </ScrollArea>
  );
}
