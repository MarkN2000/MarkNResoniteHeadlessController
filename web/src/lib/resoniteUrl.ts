// Resonite の record/world URL スキーム判定（res:// / resrec:// / res-steam:// などで始まる）。
// \w はハイフンを含まないため [-\w] でハイフンを明示的に許可する。
// セッションタブのスポーン（SpawnImpulseCard）と新規セッションの起動（StartPanel）で共有（L3）。
// 方針A 上、不正 URL でも backend は HTTP 200 を返し得る（＝無音失敗）ため、空振りを UI で減らす用途。
export const RESONITE_URL_SCHEME = /^res[-\w]*:\/\//i;

// 前後空白を除いて判定する（呼び出し側は trim を意識しなくてよい）。
export const isResoniteUrl = (s: string): boolean => RESONITE_URL_SCHEME.test(s.trim());

// お気に入り登録可能な resrec URL（resrec:///U|G-xxx/R-xxx）だけを owner/record に分解する。
// backend favorites の検証（favorites.go の favoriteResoniteURLRe）と同一の厳密形。
// 一致しない（空・他スキーム・余分な要素）なら null＝お気に入り不可。
const RESREC_RE = /^resrec:\/\/\/((?:U|G)-[A-Za-z0-9_-]+)\/(R-[A-Za-z0-9_-]+)$/;
export const parseResrecUrl = (s: string): { ownerId: string; recordId: string } | null => {
  const m = RESREC_RE.exec(s.trim());
  return m ? { ownerId: m[1], recordId: m[2] } : null;
};
