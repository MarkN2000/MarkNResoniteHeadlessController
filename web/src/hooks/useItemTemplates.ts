import { useEffect, useState } from "react";
import * as api from "../api";
import type { ItemTemplate } from "../api";

// アイテムテンプレート一覧をマウント時に1回取得する（backend がリモートリスト＋
// フォールバックで常に何かしら返す）。取得結果はShellから各タブへ共通配布する。
export function useItemTemplates(): ItemTemplate[] {
  const [templates, setTemplates] = useState<ItemTemplate[]>([]);
  useEffect(() => {
    let alive = true;
    void api.getItemTemplates().then((list) => {
      if (alive) setTemplates(list);
    });
    return () => {
      alive = false;
    };
  }, []);
  return templates;
}
