import { useCallback, useRef, useState } from "react";
import { Box, ScrollArea } from "@mantine/core";
import * as api from "../../api";
import type { BanEntry, ResoniteUser, UserInfo } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { SourcePanel } from "./SourcePanel";
import { ResultList } from "./ResultList";

// フレンドタブで②に表示できるソース種別。
//   requests/bans = ヘッドレス（focus 不要）/ focused = フォーカス中セッションの在席者 / search = Resonite 公開API。
export type FriendSource = "requests" | "bans" | "search" | "focused";

// 「取得」系（引数なしで取れるソース）。検索だけは search(term) で別扱い。
export type LoadSource = Exclude<FriendSource, "search">;

// フレンドタブ（docs §3.3 #2 / phase-7-spec §3.9）。2セクション構成:
//   ① SourcePanel = 取得/検索ソースの選択（オンデマンド。開いた時は取得しない）
//   ② ResultList  = 選んだソースの結果を1か所に集約（行内ボタンで承認/解除/申請/招待）
// idx = フォーカス中セッション（招待 と フォーカス内ユーザー取得に必要）。
export function FriendsTab({ idx }: { idx: number }) {
  const [source, setSource] = useState<FriendSource | null>(null);
  const [requests, setRequests] = useState<string[]>([]);
  const [bans, setBans] = useState<BanEntry[]>([]);
  const [searchResults, setSearchResults] = useState<ResoniteUser[]>([]);
  const [focusedUsers, setFocusedUsers] = useState<UserInfo[]>([]);
  const [searchTerm, setSearchTerm] = useState(""); // ⟳ で再検索するため最後の検索語を保持
  const [loading, setLoading] = useState(false);
  // 取得シーケンス番号。取得完了前にソースを切り替えた場合、古い結果と loading 解除を破棄する。
  const reqId = useRef(0);

  // 取得系ソース（requests/bans/focused）を②へ。タブを開いただけでは取得しない。
  const load = useCallback(
    async (src: LoadSource) => {
      const id = ++reqId.current;
      setSource(src);
      setLoading(true);
      if (src === "requests") {
        const r = await api.getFriendRequests();
        if (id !== reqId.current) return;
        setRequests(r);
      } else if (src === "bans") {
        const b = await api.getListBans();
        if (id !== reqId.current) return;
        setBans(b);
      } else {
        const d = await api.getSessionDetail(idx);
        if (id !== reqId.current) return;
        setFocusedUsers(d?.users ?? []);
      }
      setLoading(false);
    },
    [idx],
  );

  // ユーザー検索（Resonite 公開API）。結果を②へ。
  const search = useCallback(async (term: string) => {
    const id = ++reqId.current;
    setSource("search");
    setSearchTerm(term);
    setLoading(true);
    const users = await api.searchResoniteUsers(term);
    if (id !== reqId.current) return;
    setSearchResults(users);
    setLoading(false);
  }, []);

  // ⟳ / 操作後は「今表示中のソースだけ」再取得。
  const refetch = useCallback(() => {
    if (source === "search") {
      if (searchTerm) void search(searchTerm);
    } else if (source) {
      void load(source);
    }
  }, [source, searchTerm, search, load]);

  return (
    <ScrollArea h="100%" type="hover">
      {/* xl 未満=1カラム / xl 以上=① ソース選択(左)・② 結果(右)の2カラム。 */}
      <Box pb="md">
        <SplitColumns
          left={<SourcePanel active={source} loading={loading} onLoad={(s) => void load(s)} onSearch={(q) => void search(q)} />}
          right={
            <ResultList
              idx={idx}
              source={source}
              requests={requests}
              bans={bans}
              searchResults={searchResults}
              focusedUsers={focusedUsers}
              loading={loading}
              onRefetch={refetch}
            />
          }
        />
      </Box>
    </ScrollArea>
  );
}
