/**
 * Valve KeyValues (VDF / ACF) の最小パーサ。
 *
 * 対応スコープは「quoted key-value の列」と「nested block」のみで、
 * コメント (`//`) や escape (`\n`, `\"` など)、unquoted identifier、include 文などはサポートしない。
 * SteamCMD `+app_info_print` の出力をパースするために使用する。
 *
 * 例:
 *   "AppState"
 *   {
 *       "appid"    "2519830"
 *       "buildid"  "22720293"
 *       "UserConfig"
 *       {
 *           "BetaKey"   "public"
 *       }
 *   }
 *
 * 戻り値は nested なオブジェクト:
 *   { AppState: { appid: "2519830", buildid: "22720293", UserConfig: { BetaKey: "public" } } }
 *
 * 同じキーが複数回出現した場合は後勝ちとする（通常のACF/app_infoでは発生しない）。
 */
export interface KeyValuesNode {
  [key: string]: string | KeyValuesNode;
}

export function parseKeyValues(text: string): KeyValuesNode {
  let i = 0;
  const len = text.length;

  const skipWhitespace = (): void => {
    while (i < len) {
      const ch = text.charCodeAt(i);
      // space, \t, \r, \n
      if (ch === 32 || ch === 9 || ch === 13 || ch === 10) {
        i++;
        continue;
      }
      // `//` 行コメントは最小限サポート（念のため）
      if (ch === 47 /* '/' */ && text.charCodeAt(i + 1) === 47) {
        while (i < len && text.charCodeAt(i) !== 10) i++;
        continue;
      }
      break;
    }
  };

  const readQuotedString = (): string | null => {
    if (text.charCodeAt(i) !== 34 /* '"' */) return null;
    i++; // skip opening quote
    let out = '';
    while (i < len) {
      const ch = text.charCodeAt(i);
      if (ch === 92 /* '\\' */ && i + 1 < len) {
        // ごく簡易な escape: \" \\ \n \t のみ扱う
        const next = text.charCodeAt(i + 1);
        if (next === 34) { out += '"'; i += 2; continue; }
        if (next === 92) { out += '\\'; i += 2; continue; }
        if (next === 110) { out += '\n'; i += 2; continue; }
        if (next === 116) { out += '\t'; i += 2; continue; }
        // 未知の escape は literal として扱う
        out += text[i];
        i++;
        continue;
      }
      if (ch === 34 /* '"' */) {
        i++; // skip closing quote
        return out;
      }
      out += text[i];
      i++;
    }
    return out;
  };

  const parseBlock = (): KeyValuesNode => {
    const node: KeyValuesNode = {};
    while (i < len) {
      skipWhitespace();
      if (i >= len) break;

      const ch = text.charCodeAt(i);
      if (ch === 125 /* '}' */) {
        i++;
        return node;
      }

      if (ch !== 34 /* '"' */) {
        // 未知のトークンはスキップしてループ続行（堅牢性のため）
        i++;
        continue;
      }

      const key = readQuotedString();
      if (key === null) break;

      skipWhitespace();
      if (i >= len) break;

      const nextCh = text.charCodeAt(i);
      if (nextCh === 123 /* '{' */) {
        i++; // skip '{'
        node[key] = parseBlock();
      } else if (nextCh === 34 /* '"' */) {
        const value = readQuotedString();
        node[key] = value ?? '';
      } else {
        // 未対応トークン。スキップして次へ。
        i++;
      }
    }
    return node;
  };

  // トップレベルは暗黙の block として扱う
  return parseBlock();
}

/**
 * KeyValues ノードから、ドット区切りパスで子ノードを取得する。
 * 大文字・小文字を区別せず部分一致する（app_info_print の出力でキーのケーシングが変わることがあるため）。
 *
 * 例: getByPath(root, 'depots.branches.headless.buildid')
 */
export function getByPath(root: KeyValuesNode, dottedPath: string): string | KeyValuesNode | null {
  const parts = dottedPath.split('.');
  let cur: string | KeyValuesNode | null = root;
  for (const part of parts) {
    if (cur === null || typeof cur !== 'object') return null;
    const node: KeyValuesNode = cur;
    const lowerPart = part.toLowerCase();
    const matchedKey: string | undefined = Object.keys(node).find(
      (k) => k.toLowerCase() === lowerPart
    );
    if (matchedKey === undefined) return null;
    cur = node[matchedKey] ?? null;
  }
  return cur;
}
