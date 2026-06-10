import { useEffect, useState } from "react";
import type { ItemTemplate } from "../api";

// アイテムテンプレート一覧をマウント時に1回取得する（backend がリモートリスト＋
// フォールバックで常に何かしら返す）。fetcher は api.getAnnounceTemplates /
// api.getSpawnTemplates などモジュールレベルの安定参照を渡すこと。
export function useItemTemplates(fetcher: () => Promise<ItemTemplate[]>): ItemTemplate[] {
  const [templates, setTemplates] = useState<ItemTemplate[]>([]);
  useEffect(() => {
    let alive = true;
    void fetcher().then((list) => {
      if (alive) setTemplates(list);
    });
    return () => {
      alive = false;
    };
  }, [fetcher]);
  return templates;
}
