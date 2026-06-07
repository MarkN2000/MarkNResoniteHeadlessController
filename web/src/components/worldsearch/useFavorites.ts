import { useEffect, useMemo, useState } from "react";
import * as api from "../../api";
import type { WorldResult } from "../../api";

// ワールドお気に入りの取得・トグル（サーバー保存 favorites.json と同期）。
// 新規セッションタブでは NewSessionTab で一度だけ呼び StartPanel/WorldSearchPanel の両方へ渡す
// （同一タブ内で別 state を持つと追加が片方に反映されない不整合を防ぐ＝単一の真実源）。
// コンフィグ編集（WorldUrlSearch）は別タブのため独立インスタンスでよい（サーバー保存で整合）。
// favorites=一覧表示用 / isFavorited=recordId で塗り判定 / toggle=登録↔解除（無音・更新後一覧で同期）。
export function useFavorites() {
  const [favorites, setFavorites] = useState<WorldResult[]>([]);

  // マウント時に取得（★の塗り判定とお気に入り表示に使う）。
  useEffect(() => {
    void api.getFavorites().then(setFavorites);
  }, []);

  const favSet = useMemo(() => new Set(favorites.map((f) => f.recordId)), [favorites]);
  const isFavorited = (recordId: string) => favSet.has(recordId);

  // 登録↔解除（無音・更新後一覧で同期）。失敗(null)時はローカル状態を維持。
  const toggle = async (wld: WorldResult) => {
    const updated = favSet.has(wld.recordId)
      ? await api.removeFavorite(wld.recordId)
      : await api.addFavorite(wld);
    if (updated) setFavorites(updated);
  };

  return { favorites, isFavorited, toggle };
}
