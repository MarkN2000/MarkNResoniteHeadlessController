// write 操作の結果トースト（7-7 第1層）。UI サイドエフェクト（Mantine notifications）を
// ここ1か所に閉じ込め、api.ts（純データ層）と各タブを低結合に保つ。
//
// 失敗判定は「方針A の範囲」= HTTP/トランスポートレベルのみ:
//   - WriteResult.ok===false（ヘッドレス停止/タイムアウト/プロセス死亡/通信不通/入力不正）を赤トースト。
//   - 「届いたが意味的に無効」（コンタクト外への申請等）は方針A 上 ok===true で来るため検知しない。
// 成功は success 文言が渡されたときだけ控えめな緑トースト（"送信しました" 等の受理ニュアンス）。
import { notifications } from "@mantine/notifications";
import i18n from "../i18n";
import type { WriteResult } from "../api";

// 値が WriteResult か（フックは Promise<unknown> を await するため、バッチ等の void と区別する）。
function isWriteResult(v: unknown): v is WriteResult {
  return typeof v === "object" && v !== null && typeof (v as { ok?: unknown }).ok === "boolean";
}

// 失敗本文を code から localize する。未知 code は backend の生メッセージ → 汎用文言の順でフォールバック。
function writeErrorText(r: WriteResult): string {
  switch (r.code) {
    case "not_ready":
      return i18n.t("toast.errNotReady");
    case "timeout":
      return i18n.t("toast.errTimeout");
    case "process_gone":
      return i18n.t("toast.errProcessGone");
    case "network":
      return i18n.t("toast.errNetwork");
    // steam 系（/steam/config・/steam/download・/steam/cancel）
    case "headless_running":
      return i18n.t("toast.errHeadlessRunning");
    case "steam_not_configured":
      return i18n.t("toast.errSteamNotConfigured");
    case "update_in_progress":
      return i18n.t("toast.errUpdateInProgress");
    case "no_update":
      return i18n.t("toast.errNoUpdate");
    case "steam_password_invalid":
      return i18n.t("toast.errSteamPasswordInvalid");
    default:
      return r.error || i18n.t("toast.errGeneric");
  }
}

// 任意の赤い失敗トースト。WriteResult を経由しない失敗（例: 起動失敗）からも使う。
// title 省略時は汎用の「操作に失敗しました」。
export function notifyError(message: string, title?: string): void {
  notifications.show({
    color: "red",
    title: title ?? i18n.t("toast.writeFailTitle"),
    message,
    autoClose: 6000,
  });
}

// 中立の情報トースト（失敗でも完了でもない経過の通知。例: ランタイム設置を伴う起動受付）。
export function notifyInfo(message: string): void {
  notifications.show({ color: "cyan", message, autoClose: 5000 });
}

// 実行チョークポイント（useAsyncAction.run / useConfirm.confirm）から呼ぶ。
// r が WriteResult でなければ何もしない（成功/失敗の判定対象外）。
export function reportWriteResult(r: unknown, success?: string): void {
  if (!isWriteResult(r)) return;
  if (r.ok) {
    if (success) {
      notifications.show({ color: "teal", message: success, autoClose: 2000 });
    }
    return;
  }
  notifyError(writeErrorText(r));
}
