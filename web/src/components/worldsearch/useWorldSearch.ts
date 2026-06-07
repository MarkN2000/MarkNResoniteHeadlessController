import { useRef, useState } from "react";
import * as api from "../../api";
import type { WorldResult } from "../../api";

// ワールド検索の状態一式（新規セッションタブとコンフィグ編集で共用・UI改善②）。
// 検索 = go.resonite.com の公開ワールド（HTML スクレイピング・上位24件・バックエンド経由）。
// reqId は連打/順序逆転で古い結果を捨てる stale ガード（旧 WorldSearchPanel から移設）。
// searched は「初回未検索」と「検索したが0件」を区別する（空表示の出し分け用）。
export interface WorldSearchState {
  keyword: string;
  setKeyword: (v: string) => void;
  loading: boolean;
  searched: boolean;
  results: WorldResult[];
  showingFavorites: boolean;
  toggleFavoritesView: () => void;
  doSearch: () => Promise<void>;
}

export function useWorldSearch(): WorldSearchState {
  const [keyword, setKeyword] = useState("");
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [results, setResults] = useState<WorldResult[]>([]);
  const [showingFavorites, setShowingFavorites] = useState(false);
  const reqId = useRef(0);

  const doSearch = async () => {
    const term = keyword.trim();
    if (!term || loading) return;
    const id = ++reqId.current;
    setLoading(true);
    setSearched(true);
    setShowingFavorites(false); // 検索したらお気に入り表示は解除
    const r = await api.searchResoniteWorlds(term);
    if (id !== reqId.current) return; // 古い応答は破棄
    setResults(r);
    setLoading(false);
  };

  return {
    keyword,
    setKeyword,
    loading,
    searched,
    results,
    showingFavorites,
    toggleFavoritesView: () => setShowingFavorites((v) => !v),
    doSearch,
  };
}
