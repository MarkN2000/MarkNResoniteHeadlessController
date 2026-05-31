import { useEffect, useRef } from "react";

// 表示中のみ fn を intervalMs ごとに実行する poll フック（Page Visibility 連動）。
//   - 重複起動しない: 前回の fn 完了後に次をスケジュール（再帰 setTimeout）。遅い/詰まる
//     poll が積み上がらない。
//   - タブ非表示(document.hidden)で停止し、再表示で即時実行＋再開。
//   - unmount で確実に停止。
//   - fn は ref 経由で常に最新を呼ぶ（依存に入れず interval を再購読させない＝idx 変化等で
//     タイマーをリセットしない）。
// 初回 poll は +intervalMs（マウント直後の即時取得は呼び出し側が別途行う前提＝二重取得回避）。
// phase-7-spec §3.4「データ鮮度戦略」。
export function useVisiblePolling(fn: () => void | Promise<void>, intervalMs: number) {
  const fnRef = useRef(fn);
  fnRef.current = fn;

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let cancelled = false;

    const clear = () => {
      if (timer !== undefined) {
        clearTimeout(timer);
        timer = undefined;
      }
    };
    const tick = async () => {
      clear();
      if (cancelled || document.hidden) return;
      try {
        await fnRef.current();
      } finally {
        // fn が throw しても poll ループを止めない（次回で復帰）。停止/非表示/unmount 時のみ再予約しない。
        if (!cancelled && !document.hidden) timer = setTimeout(() => void tick(), intervalMs);
      }
    };
    const onVisibility = () => {
      if (document.hidden) clear();
      else void tick(); // 再表示: 即時実行＋再開
    };

    document.addEventListener("visibilitychange", onVisibility);
    if (!document.hidden) timer = setTimeout(() => void tick(), intervalMs);

    return () => {
      cancelled = true;
      clear();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [intervalMs]);
}
