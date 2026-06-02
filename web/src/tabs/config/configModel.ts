// コンフィグタブのデータモデルとヘルパ（§3.14）。
// config は不透明な JSON map。フォームは特定キーのビューに過ぎず、map を単一の真実として
// 持つ（編集は clone への in-place）。これにより未知/レアフィールドはキーを落とさず自動温存される。
// $schema は保存時に backend が付与するため雛形には含めない。

export type ConfigMap = Record<string, unknown>;
export type WorldMap = Record<string, unknown>;

// 1ワールドの雛形（accessLevel=Private＝安全側・loadWorldPresetName=Grid）。
export function defaultWorld(): WorldMap {
  return {
    isEnabled: true,
    sessionName: "",
    description: "",
    maxUsers: 16,
    accessLevel: "Private",
    loadWorldPresetName: "Grid",
  };
}

// 同梱デフォルト config の雛形（hlconfig.defaultConfigJSON 相当・creds 空＝中央注入）。
export function defaultConfig(): ConfigMap {
  return {
    comment: "",
    tickRate: 60,
    maxConcurrentAssetTransfers: 128,
    loginCredential: "",
    loginPassword: "",
    startWorlds: [defaultWorld()],
  };
}

// startWorlds を配列として安全に取り出す（不在/非配列は空配列）。
export function getWorlds(cfg: ConfigMap): WorldMap[] {
  return Array.isArray(cfg.startWorlds) ? (cfg.startWorlds as WorldMap[]) : [];
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
