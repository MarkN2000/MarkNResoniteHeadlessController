// Resonite の record/world URL スキーム判定（res:// / resrec:// / res-steam:// などで始まる）。
// \w はハイフンを含まないため [-\w] でハイフンを明示的に許可する。
// セッションタブのスポーン（SpawnImpulseCard）と新規セッションの起動（StartPanel）で共有（L3）。
// 方針A 上、不正 URL でも backend は HTTP 200 を返し得る（＝無音失敗）ため、空振りを UI で減らす用途。
export const RESONITE_URL_SCHEME = /^res[-\w]*:\/\//i;

// 前後空白を除いて判定する（呼び出し側は trim を意識しなくてよい）。
export const isResoniteUrl = (s: string): boolean => RESONITE_URL_SCHEME.test(s.trim());
