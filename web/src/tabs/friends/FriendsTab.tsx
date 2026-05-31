import { useCallback, useState } from "react";
import { Box, ScrollArea } from "@mantine/core";
import * as api from "../../api";
import type { BanEntry } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { SourcePanel } from "./SourcePanel";
import { ResultList } from "./ResultList";

// フレンドタブで②に表示できるソース種別。検索/フォーカス内は P9 で追加（"search"|"focused"）。
export type FriendSource = "requests" | "bans";

// フレンドタブ（docs §3.3 #2）。2セクション構成:
//   ① SourcePanel = 何を取得/検索するか（オンデマンド。開いた時は取得しない）
//   ② ResultList  = 選んだソースの結果を1か所に集約表示（行内ボタンで承認/解除）
// friendrequests/listbans は global（focus 不要）のため idx は持たない。
export function FriendsTab() {
  const [source, setSource] = useState<FriendSource | null>(null);
  const [requests, setRequests] = useState<string[]>([]);
  const [bans, setBans] = useState<BanEntry[]>([]);
  const [loading, setLoading] = useState(false);

  // 押されたソースだけ取得して②へ。タブを開いただけでは何も取得しない。
  const load = useCallback(async (src: FriendSource) => {
    setSource(src);
    setLoading(true);
    if (src === "requests") setRequests(await api.getFriendRequests());
    else setBans(await api.getListBans());
    setLoading(false);
  }, []);

  // ⟳ / 操作後は「今表示中のソースだけ」再取得（全要素を取りに行かない）。
  const refetch = useCallback(() => {
    if (source) void load(source);
  }, [source, load]);

  return (
    <ScrollArea h="100%" type="hover">
      {/* xl 未満=1カラム / xl 以上=① ソース選択(左)・② 結果(右)の2カラム。 */}
      <Box pb="md">
        <SplitColumns
          left={<SourcePanel active={source} loading={loading} onLoad={(src) => void load(src)} />}
          right={<ResultList source={source} requests={requests} bans={bans} loading={loading} onRefetch={refetch} />}
        />
      </Box>
    </ScrollArea>
  );
}
