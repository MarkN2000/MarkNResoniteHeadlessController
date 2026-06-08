// クリップボードへのコピー（セキュアコンテキスト非依存）。
//
// navigator.clipboard は「セキュアコンテキスト（https / localhost）」でしか使えず、
// LAN/HTTP（例: http://192.168.x.x:8080）では undefined になる（lan-http-no-secure-context-apis）。
// そのため Async Clipboard API → 旧来の execCommand("copy") の順にフォールバックする。
// localhost 確認だけでは気づけない不具合なので、必ずこのヘルパ経由でコピーする。
export async function copyText(text: string): Promise<boolean> {
  // 1) Async Clipboard API（セキュアコンテキストのみ・権限拒否時は 2 へ）
  if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // フォールバックへ
    }
  }
  // 2) execCommand フォールバック（非セキュアコンテキスト/古環境）
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    // 画面外に置きスクロール飛びを防ぐ（選択は可能にする）。
    ta.style.position = "fixed";
    ta.style.top = "-9999px";
    ta.style.left = "-9999px";
    ta.setAttribute("readonly", "");
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}
