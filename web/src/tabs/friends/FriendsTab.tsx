import { useCallback, useRef, useState } from "react";
import { Box, ScrollArea } from "@mantine/core";
import * as api from "../../api";
import type { BanEntry, UserInfo } from "../../api";
import { SplitColumns } from "../../components/SplitColumns";
import { SourcePanel } from "./SourcePanel";
import { ResultList } from "./ResultList";

// ユーザータブで②に表示できるソース種別。
//   requests/bans = ヘッドレス（focus 不要）/ focused = フォーカス中セッションの在席者 / search = Resonite 公開API。
export type FriendSource = "requests" | "bans" | "search" | "focused";

// 「取得」系（引数なしで取れるソース）。検索だけは search(term) で別扱い。
export type LoadSource = Exclude<FriendSource, "search">;

// 統一リストで扱うユーザー行（検索結果=ResoniteUser / フォーカス内=UserInfo / リクエスト=username を正規化）。
// bans 以外は全てこの形に揃え、requests/focused は Resonite 公開API で icon/id を解決して付与する（main 方式）。
export interface UserRow {
  id: string;
  username: string;
  iconUrl?: string;
}

// 各 username を Resonite 公開APIで解決し id+icon を付与する（requests 用）。
// 正規化ユーザー名の完全一致のみ採用（別人のアイコンを付けない）。失敗/不一致は icon 無しで残す。
async function enrichByNames(names: string[]): Promise<UserRow[]> {
  return Promise.all(
    names.map(async (name) => {
      const norm = name.trim().toLowerCase();
      const found = await api.searchResoniteUsers(name);
      const hit = found.find((u) => u.username.trim().toLowerCase() === norm);
      return hit ? { id: hit.id, username: name, iconUrl: hit.iconUrl } : { id: "", username: name };
    }),
  );
}

// 在席者(UserInfo)を id で解決して icon を付与する（focused 用）。名前/id は在席情報を優先。
// 匿名(id 無=ヘッドレス自身など)は解決せずプレースホルダで残す（main は解決失敗を落としていたが残す）。
async function enrichUsers(users: UserInfo[]): Promise<UserRow[]> {
  return Promise.all(
    users.map(async (u) => {
      if (!u.id) return { id: "", username: u.name };
      const found = await api.searchResoniteUsers(u.id); // u.id は "U-xxx"＝ID検索
      return { id: u.id, username: u.name, iconUrl: found[0]?.iconUrl };
    }),
  );
}

// ユーザータブ（docs §3.3 #2 / phase-7-spec §3.9）。2セクション構成:
//   ① SourcePanel = 取得/検索ソースの選択（オンデマンド。開いた時は取得しない）
//   ② ResultList  = 選んだソースの結果を統一行で集約（bans のみ別表示・行内ボタン方式）
// idx = フォーカス中セッション（招待 / メッセージ / フォーカス内ユーザー取得に必要）。
export function FriendsTab({ idx, selfUserId }: { idx: number; selfUserId: string | null }) {
  const [source, setSource] = useState<FriendSource | null>(null);
  const [requests, setRequests] = useState<UserRow[]>([]);
  const [bans, setBans] = useState<BanEntry[]>([]);
  const [searchResults, setSearchResults] = useState<UserRow[]>([]);
  const [focused, setFocused] = useState<UserRow[]>([]);
  const [searchTerm, setSearchTerm] = useState(""); // ⟳ で再検索するため最後の検索語を保持
  const [loading, setLoading] = useState(false);
  // 取得シーケンス番号。取得/解決完了前にソースを切り替えた場合、古い結果と loading 解除を破棄する。
  const reqId = useRef(0);

  // 取得系ソース（requests/bans/focused）を②へ。requests/focused は icon/id を解決してから表示。
  const load = useCallback(
    async (src: LoadSource) => {
      const id = ++reqId.current;
      setSource(src);
      setLoading(true);
      if (src === "requests") {
        const names = await api.getFriendRequests();
        if (id !== reqId.current) return;
        const rows = await enrichByNames(names); // 解決は非同期＝完了後に再度ガード
        if (id !== reqId.current) return;
        setRequests(rows);
      } else if (src === "bans") {
        const b = await api.getListBans();
        if (id !== reqId.current) return;
        setBans(b);
      } else {
        const d = await api.getSessionDetail(idx);
        if (id !== reqId.current) return;
        const rows = await enrichUsers(d?.users ?? []);
        if (id !== reqId.current) return;
        setFocused(rows);
      }
      setLoading(false);
    },
    [idx],
  );

  // ユーザー検索（Resonite 公開API）。結果(ResoniteUser=UserRow)を②へ。
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

  // ②に渡す行（bans 以外の現ソースの正規化済み行）。
  const rows =
    source === "search" ? searchResults : source === "focused" ? focused : source === "requests" ? requests : [];

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
              rows={rows}
              bans={bans}
              selfUserId={selfUserId}
              loading={loading}
              onRefetch={refetch}
            />
          }
        />
      </Box>
    </ScrollArea>
  );
}
