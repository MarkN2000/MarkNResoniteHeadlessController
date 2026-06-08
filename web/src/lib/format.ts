// 表示用の整形ヘルパ（純関数・UI 非依存）。

// バイト数を人間可読（B/KB/MB/GB）に整形する。ログのファイルサイズ表示・
// キャッシュサイズ表示で共用（旧 LogsTab/CacheSection のローカル重複を一本化）。
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
