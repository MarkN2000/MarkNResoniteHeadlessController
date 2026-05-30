import { useCallback, useState } from "react";

// 非同期アクションの共通フック。
//   - busy: 実行中フラグ（ボタンの loading/disabled に使う）
//   - run(fn): fn を実行し、完了後に onDone（例: 再取得）を呼ぶ。例外時も busy は戻す。
// 各タブの「操作 → 完了後に再取得（方針A）」を集約する。
export function useAsyncAction(onDone?: () => void) {
  const [busy, setBusy] = useState(false);
  const run = useCallback(
    async (fn: () => Promise<unknown>) => {
      setBusy(true);
      try {
        await fn();
      } finally {
        setBusy(false);
      }
      onDone?.();
    },
    [onDone],
  );
  return { busy, run };
}
