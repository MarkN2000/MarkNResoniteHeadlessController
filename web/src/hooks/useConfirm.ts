import { useState } from "react";

// 確認ダイアログの「開く要求」。onConfirm は同期/非同期どちらも可。
export interface ConfirmRequest {
  title: string;
  message?: string;
  danger?: boolean;
  onConfirm: () => void | Promise<void>;
}

// 確認ダイアログの開閉と実行を1か所で扱う共通フック。
//   - ask(req): ダイアログを開く
//   - confirm(): req.onConfirm を実行（async なら完了まで busy=true でモーダル継続）→ 閉じる
//   - close(): キャンセル（実行中は閉じない）
// 各タブはこのフック + 1つの <ConfirmModal> で全確認操作をまかなう（kind ごとの分岐を持たない）。
export function useConfirm() {
  const [request, setRequest] = useState<ConfirmRequest | null>(null);
  const [busy, setBusy] = useState(false);

  const close = () => {
    if (!busy) setRequest(null);
  };
  const confirm = async () => {
    if (!request) return;
    setBusy(true);
    try {
      await request.onConfirm();
    } finally {
      setBusy(false);
      setRequest(null);
    }
  };
  return { request, busy, ask: setRequest, close, confirm };
}
