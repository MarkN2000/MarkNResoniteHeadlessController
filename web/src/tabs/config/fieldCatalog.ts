// ③「詳細フィールド追加」のカタログと、①②専用フォームが扱うキー集合（二重表示防止の単一の真実）。
// 公式スキーマ由来。ラベルは i18n（t(`config.${key}`)）で解決するため、ここではキーと型のみ持つ。
//
// 設計（map が単一の真実）:
//   - 専用フォーム（①②）が直接扱うキー = *_DEDICATED_KEYS。
//   - 「詳細フィールド」は map に存在する非 dedicated キーだけを行表示し、ドロップダウンには
//     カタログのうち未追加のキーのみを出す（重複・誤キー不可）。
//   - dedicated でもカタログでもない「未知キー」は json として生 JSON で温存表示する。

export type FieldType = "string" | "int" | "bool" | "enum" | "strarray" | "json";

export interface FieldDef {
  key: string;
  type: FieldType;
  enum?: readonly string[]; // type==="enum" のとき選択肢
}

// 専用フォーム（①②）が扱うトップレベルキー。$schema/startWorlds 等の構造キーも含め、
// ③へ漏れ出さないようにする。
export const TOP_DEDICATED_KEYS: ReadonlySet<string> = new Set([
  "$schema",
  "startWorlds",
  "comment",
  "tickRate",
  "maxConcurrentAssetTransfers",
  "usernameOverride",
  "dataFolder",
  "cacheFolder",
  "logsFolder",
  "allowedUrlHosts",
  "autoSpawnItems",
  "loginCredential",
  "loginPassword",
]);

// 専用フォーム（①②）が扱うワールドキー（①一般14＋②上級10＝24）。
export const WORLD_DEDICATED_KEYS: ReadonlySet<string> = new Set([
  // ①一般
  "isEnabled",
  "sessionName",
  "description",
  "accessLevel",
  "maxUsers",
  "loadWorldPresetName",
  "loadWorldURL",
  "customSessionId",
  "tags",
  "awayKickMinutes",
  "idleRestartInterval",
  "defaultUserRoles",
  "autoInviteUsernames",
  "autoInviteMessage",
  // ②上級
  "forcedRestartInterval",
  "autosaveInterval",
  "saveOnExit",
  "autoSleep",
  "hideFromPublicListing",
  "inviteRequestHandlerUsernames",
  "saveAsOwner",
  "enableResoniteLink",
  "forceResoniteLinkPort",
  "forcePort",
]);

// ③カタログ（トップレベル）。当面は universeId のみ（残りは全て①②）。
export const TOP_NICHE_CATALOG: readonly FieldDef[] = [{ key: "universeId", type: "string" }];

// ③カタログ（ワールド・12キー）。
export const WORLD_NICHE_CATALOG: readonly FieldDef[] = [
  { key: "keepOriginalRoles", type: "bool" },
  { key: "useCustomJoinVerifier", type: "bool" },
  { key: "waitForLogin", type: "bool" },
  { key: "mobileFriendly", type: "bool" },
  { key: "autoRecover", type: "bool" },
  { key: "roleCloudVariable", type: "string" },
  { key: "allowUserCloudVariable", type: "string" },
  { key: "denyUserCloudVariable", type: "string" },
  { key: "requiredUserJoinCloudVariable", type: "string" },
  { key: "requiredUserJoinCloudVariableDenyMessage", type: "string" },
  { key: "parentSessionIds", type: "strarray" },
  { key: "overrideCorrespondingWorldId", type: "json" },
];

// map に存在する「非 dedicated キー」を、カタログ順→未知キー（出現順）で返す。
// ③エディタの行表示はこの順序を使う（カタログの並びが安定した表示順になる）。
export function extraKeysInOrder(
  obj: Record<string, unknown>,
  dedicated: ReadonlySet<string>,
  catalog: readonly FieldDef[],
): string[] {
  const present = Object.keys(obj).filter((k) => !dedicated.has(k));
  const presentSet = new Set(present);
  const ordered: string[] = [];
  for (const def of catalog) {
    if (presentSet.has(def.key)) ordered.push(def.key);
  }
  // カタログに無い未知キーは出現順で末尾へ。
  for (const k of present) {
    if (!catalog.some((d) => d.key === k)) ordered.push(k);
  }
  return ordered;
}

// キーの FieldDef を返す。カタログ外（未知キー）は json 扱い。
export function defForKey(catalog: readonly FieldDef[], key: string): FieldDef {
  return catalog.find((d) => d.key === key) ?? { key, type: "json" };
}

// 型別の「追加直後の初期値」。bool は false、それ以外は null（キーは存在するが未設定）。
export function initialValueFor(type: FieldType): unknown {
  return type === "bool" ? false : null;
}
