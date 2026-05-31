import { useState } from "react";
import { reportWriteResult } from "../lib/notify";

// 確認ダイアログの「開く要求」。onConfirm は同期/非同期どちらも可。
//   - onConfirm が WriteResult を返した場合は結果をトースト（失敗=赤・成功=success 指定時のみ緑）。7-7 第1層。
//   - success: 成功トーストの本文（受理ニュアンス・任意）。
export interface ConfirmRequest {
  title: string;
  message?: string;
  danger?: boolean;
  success?: string;
  onConfirm: () => unknown | Promise<unknown>;
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
      const result = await request.onConfirm();
      reportWriteResult(result, request.success);
    } finally {
      setBusy(false);
      setRequest(null);
    }
  };
  return { request, busy, ask: setRequest, close, confirm };
}
