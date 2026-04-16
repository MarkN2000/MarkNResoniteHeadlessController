import { promises as fs } from 'node:fs';
import path from 'node:path';

/**
 * Valve KeyValues (VDF / ACF) の最小パーサ。
 *
 * 対応スコープは「quoted key-value の列」と「nested block」のみで、
 * コメント (`//`) や escape (`\n`, `\"` など)、unquoted identifier、include 文などはサポートしない。
 * appmanifest_<appId>.acf と `app_info_print` の出力の両方を扱える最小の実装である。
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

export interface InstalledManifest {
  /** ルートの `buildid`（現在インストールされているビルド ID） */
  buildid: string | null;
  /** `UserConfig.BetaKey`（選択されているベータブランチ名、public なら "public"） */
  betaKey: string | null;
  /** 全体の生ノード（デバッグ用） */
  raw: KeyValuesNode | null;
}

/**
 * Steam の appmanifest_<appId>.acf を読み込み、installed buildid 等を返す。
 *
 * SteamCMD は `+force_install_dir` でゲームファイルの配置先を変更しても、
 * appmanifest は SteamCMD 自身の `steamapps/` ディレクトリに書き込む。
 * そのため、以下の優先順位で manifest を検索する:
 *
 *   1. `<steamcmd_dir>/steamapps/appmanifest_<appId>.acf`（SteamCMD が書く場所）
 *   2. `installDir/../../appmanifest_<appId>.acf`（Steam クライアント標準構造のフォールバック）
 *
 * - ファイル欠落 (ENOENT) → 次の候補を試す / 全候補で見つからなければ全フィールド null
 * - read 失敗・パース失敗 → 全フィールド null
 * - 例外は投げない（呼び出し側が null 扱いで継続できるように）
 */
export async function readInstalledManifest(
  installDir: string,
  appId: string,
  steamcmdPath?: string
): Promise<InstalledManifest> {
  const manifestName = `appmanifest_${appId}.acf`;

  // 候補パスを優先順に構築
  const candidates: string[] = [];
  if (steamcmdPath) {
    candidates.push(path.resolve(path.dirname(steamcmdPath), 'steamapps', manifestName));
  }
  candidates.push(path.resolve(installDir, '..', '..', manifestName));

  // 重複除去（両方が同じパスに解決される場合）
  const seen = new Set<string>();
  const uniqueCandidates = candidates.filter((p) => {
    const resolved = path.resolve(p);
    if (seen.has(resolved)) return false;
    seen.add(resolved);
    return true;
  });

  for (const manifestPath of uniqueCandidates) {
    try {
      const text = await fs.readFile(manifestPath, 'utf-8');
      const root = parseKeyValues(text);

      const appState =
        (getByPath(root, 'AppState') as KeyValuesNode | null) ?? root;

      const buildidRaw = getByPath(appState, 'buildid');
      const buildid = typeof buildidRaw === 'string' && buildidRaw !== '' ? buildidRaw : null;

      const betaKeyRaw = getByPath(appState, 'UserConfig.BetaKey');
      const betaKey = typeof betaKeyRaw === 'string' && betaKeyRaw !== '' ? betaKeyRaw : null;

      console.log(`[SteamManifestReader] Read manifest from ${manifestPath} (buildid=${buildid})`);

      return {
        buildid,
        betaKey,
        raw: root
      };
    } catch (error: any) {
      if (error?.code !== 'ENOENT') {
        console.warn(`[SteamManifestReader] Failed to read ${manifestPath}:`, error?.message ?? error);
      }
      // 次の候補を試す
    }
  }

  console.warn(
    `[SteamManifestReader] appmanifest not found in any candidate path: ${uniqueCandidates.join(', ')}`
  );
  return {
    buildid: null,
    betaKey: null,
    raw: null
  };
}
