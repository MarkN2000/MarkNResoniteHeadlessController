// コンフィグタブのデータモデルとヘルパ（§3.14）。
// config は不透明な JSON map。フォームは特定キーのビューに過ぎず、map を単一の真実として
// 持つ（編集は clone への in-place）。これにより未知/レアフィールドはキーを落とさず自動温存される。
// $schema は保存時に backend が付与するため雛形には含めない。

import type { ConfigDefaults } from "../../api";

export type ConfigMap = Record<string, unknown>;
export type WorldMap = Record<string, unknown>;

// 1ワールドの雛形。専用フォーム（①一般＋②上級）が扱うキーだけを明示的に持つ。
// 公式スキーマのうちニッチな項目（③）は雛形に入れず、「詳細フィールド」から必要時に追加する。
// （map に存在するキーだけを行表示する設計のため、雛形に入れると新規 config で全ニッチ項目が
//  最初から展開されてしまう＝スリム化。fieldCatalog.ts の WORLD_DEDICATED_KEYS と対応する。）
// 専用フォームのキーは UI 表示と保存 JSON を常に一致させ、未設定は null（有害な空文字は使わない）。
// 決定値: accessLevel=Anyone・awayKickMinutes=5・autoSleep=true・idleRestartInterval=1800・
// 強制再起動/自動保存=-1(無効) はここ（①②）に残す。
// 注: autoRecover はニッチ化に伴い雛形から外し headless 既定へ委譲（既定値 true は明示しない）。
export function defaultWorld(): WorldMap {
  return {
    // ①一般
    isEnabled: true,
    sessionName: null,
    description: null,
    accessLevel: "Anyone",
    maxUsers: 16,
    loadWorldPresetName: "Grid",
    loadWorldURL: null,
    customSessionId: null,
    tags: null,
    awayKickMinutes: 5,
    idleRestartInterval: 1800,
    defaultUserRoles: null,
    autoInviteUsernames: null,
    autoInviteMessage: null,
    // ②上級
    forcedRestartInterval: -1,
    autosaveInterval: -1,
    saveOnExit: false,
    autoSleep: true,
    hideFromPublicListing: false,
    inviteRequestHandlerUsernames: null,
    saveAsOwner: null,
    enableResoniteLink: false,
    forceResoniteLinkPort: null,
  };
}

// 同梱デフォルト config の雛形（hlconfig.defaultConfigJSON 相当・creds 空＝中央注入）。
// 専用フォーム（①一般＋②上級）のトップレベルキーのみ明示（未設定は null）。universeId 等の
// ニッチ項目は「詳細フィールド」から追加（スリム化）。$schema は保存時に backend が付与。
// folders = backend /headless-config-defaults の dataFolder/cacheFolder 既定値（UI改善⑤）。
// 未着/取得失敗（null/undefined）は null のまま＝headless 既定に委譲（従来挙動）。
export function defaultConfig(folders?: ConfigDefaults | null): ConfigMap {
  return {
    comment: "",
    tickRate: 60,
    maxConcurrentAssetTransfers: 128,
    usernameOverride: null,
    loginCredential: "",
    loginPassword: "",
    startWorlds: [defaultWorld()],
    dataFolder: folders?.dataFolder ?? null,
    cacheFolder: folders?.cacheFolder ?? null,
    logsFolder: null,
    allowedUrlHosts: ["https://tts.markn2000.com/"],
    autoSpawnItems: null,
  };
}

// startWorlds を配列として安全に取り出す（不在/非配列は空配列）。
export function getWorlds(cfg: ConfigMap): WorldMap[] {
  return Array.isArray(cfg.startWorlds) ? (cfg.startWorlds as WorldMap[]) : [];
}

export type ForcePortProtocol = "lnl" | "quic" | "tcp";

const hasOwn = (obj: Record<string, unknown>, key: string): boolean =>
  Object.prototype.hasOwnProperty.call(obj, key);

const forcePortsMap = (world: WorldMap): Record<string, unknown> | null => {
  const value = world.forcePorts;
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
};

// 旧 forcePort は LNL の表示用フォールバックに限る。forcePorts.lnl が存在すれば新形式を優先する。
export function getForcePort(world: WorldMap, protocol: ForcePortProtocol): number | "" {
  const ports = forcePortsMap(world);
  if (ports && hasOwn(ports, protocol)) {
    return typeof ports[protocol] === "number" ? ports[protocol] : "";
  }
  if (protocol === "lnl" && typeof world.forcePort === "number") return world.forcePort;
  return "";
}

// UI から1プロトコルだけを更新する。未知のプロトコルは温存し、編集時点で旧キーは新形式へ移行する。
export function setForcePort(
  world: WorldMap,
  protocol: ForcePortProtocol,
  value: number | string,
): WorldMap {
  const ports = { ...(forcePortsMap(world) ?? {}) };
  if (!hasOwn(ports, "lnl") && typeof world.forcePort === "number") {
    ports.lnl = world.forcePort;
  }
  if (value === "") delete ports[protocol];
  else ports[protocol] = Number(value);

  const next = { ...world };
  delete next.forcePort;
  if (Object.keys(ports).length > 0) next.forcePorts = ports;
  else delete next.forcePorts;
  return next;
}

export const hasLegacyForcePort = (world: WorldMap): boolean => hasOwn(world, "forcePort");

// LNL と QUIC はどちらも UDP リスナーを使う。同一ワールド内か別ワールドかを問わず、
// 起動対象の全ワールドで両プロトコルの固定ポートが重複しないようにする。
export function getDuplicateUDPPorts(cfg: ConfigMap): Set<number> {
  const counts = new Map<number, number>();
  for (const world of getWorlds(cfg)) {
    if (world.isEnabled === false) continue;
    for (const protocol of ["lnl", "quic"] as const) {
      const port = getForcePort(world, protocol);
      if (typeof port !== "number" || !Number.isInteger(port) || port < 1 || port > 65535) continue;
      counts.set(port, (counts.get(port) ?? 0) + 1);
    }
  }
  return new Set([...counts].filter(([, count]) => count > 1).map(([port]) => port));
}

export function setWorld(cfg: ConfigMap, index: number, world: WorldMap): ConfigMap {
  const worlds = getWorlds(cfg).slice();
  worlds[index] = world;
  return { ...cfg, startWorlds: worlds };
}

export function addWorld(cfg: ConfigMap): ConfigMap {
  return { ...cfg, startWorlds: [...getWorlds(cfg), defaultWorld()] };
}

export function removeWorld(cfg: ConfigMap, index: number): ConfigMap {
  const worlds = getWorlds(cfg).slice();
  worlds.splice(index, 1);
  return { ...cfg, startWorlds: worlds };
}

// --- 値コアーション（フォーム境界用）---

export const asStr = (v: unknown): string => (typeof v === "string" ? v : "");
export const asBool = (v: unknown, fallback = false): boolean => (typeof v === "boolean" ? v : fallback);
// NumberInput の value は number | "" を取る。数値以外は "" にして空欄表示。
export const asNum = (v: unknown): number | "" => (typeof v === "number" ? v : "");
// asNum と同じだが、未設定/非数値のとき "" ではなく fallback（数値）を返す。
// -1=無効 型フィールド（awayKick/idleRestart/forced/autosave）が常に数値を表示するために使う（R6）。
export const asNumOr = (v: unknown, fallback: number): number => (typeof v === "number" ? v : fallback);

// 文字列配列を安全に取り出す（allowedUrlHosts 用）。
export function getStringArray(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

// 配列/文字列 → カンマ区切り表示（tags / autoSpawnItems）。
export function arrayToCsv(v: unknown): string {
  if (Array.isArray(v)) return v.map(String).join(", ");
  return typeof v === "string" ? v : "";
}
// カンマ区切り → トリム済み配列（空要素は除去）。
export function csvToArray(s: string): string[] {
  return s
    .split(",")
    .map((x) => x.trim())
    .filter((x) => x !== "");
}

// customSessionId（単一文字列）⇔ prefix/suffix（v1 互換・最初の ':' で分割）。
//   "U-x:abc" → {prefix:"U-x", suffix:"abc"} / ':' 無し → 全体を suffix。
export function splitCustomSessionId(v: unknown): { prefix: string; suffix: string } {
  if (typeof v !== "string" || v === "") return { prefix: "", suffix: "" };
  const i = v.indexOf(":");
  if (i >= 0) return { prefix: v.slice(0, i), suffix: v.slice(i + 1) };
  return { prefix: "", suffix: v };
}
// prefix/suffix → customSessionId。両方あれば "p:s"、suffix のみなら "s"、空なら ""。
export function joinCustomSessionId(prefix: string, suffix: string): string {
  const p = prefix.trim();
  const s = suffix.trim();
  if (p && s) return `${p}:${s}`;
  return s;
}
