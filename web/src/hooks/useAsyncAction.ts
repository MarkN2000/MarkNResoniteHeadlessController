import { useCallback, useState } from "react";
import { reportWriteResult } from "../lib/notify";

// 非同期アクションの共通フック。
//   - busy: 実行中フラグ（ボタンの loading/disabled に使う）
//   - run(fn, success?): fn を実行し、完了後に onDone（例: 再取得）を呼ぶ。例外時も busy は戻す。
//     fn が WriteResult を返した場合は結果をトースト（失敗=赤・成功=success 指定時のみ緑）。7-7 第1層。
// 各タブの「操作 → 完了後に再取得（方針A）」を集約する。
export function useAsyncAction(onDone?: () => void) {
  const [busy, setBusy] = useState(false);
  const run = useCallback(
    async (fn: () => Promise<unknown>, success?: string) => {
      setBusy(true);
      try {
        const result = await fn();
        reportWriteResult(result, success);
      } finally {
        setBusy(false);
      }
      onDone?.();
    },
    [onDone],
  );
  return { busy, run };
}
