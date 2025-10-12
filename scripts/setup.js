/**
 * 初回セットアップスクリプト
 * 設定ファイルのサンプルから実際の設定ファイルをコピーします
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT_DIR = path.resolve(__dirname, '..');

const CONFIG_FILES = [
  {
    example: path.join(ROOT_DIR, 'env.example'),
    target: path.join(ROOT_DIR, '.env'),
    name: '環境変数設定',
  },
  {
    example: path.join(ROOT_DIR, 'config', 'auth.json.example'),
    target: path.join(ROOT_DIR, 'config', 'auth.json'),
    name: '認証設定',
  },
  {
    example: path.join(ROOT_DIR, 'config', 'security.json.example'),
    target: path.join(ROOT_DIR, 'config', 'security.json'),
    name: 'セキュリティ設定',
  },
  {
    example: path.join(ROOT_DIR, 'config', 'runtime-state.json.example'),
    target: path.join(ROOT_DIR, 'config', 'runtime-state.json'),
    name: 'ランタイム状態',
  },
  {
    example: path.join(ROOT_DIR, 'backend', 'config', 'restart.json.example'),
    target: path.join(ROOT_DIR, 'backend', 'config', 'restart.json'),
    name: '再起動設定',
  },
  {
    example: path.join(ROOT_DIR, 'backend', 'config', 'restart-status.json.example'),
    target: path.join(ROOT_DIR, 'backend', 'config', 'restart-status.json'),
    name: '再起動ステータス',
  },
];

console.log('🚀 MarkN Resonite Headless Controller - 初回セットアップ\n');

let copiedCount = 0;
let skippedCount = 0;

for (const { example, target, name } of CONFIG_FILES) {
  if (!fs.existsSync(example)) {
    console.log(`⚠️  サンプルファイルが見つかりません: ${name} (${example})`);
    continue;
  }

  if (fs.existsSync(target)) {
    console.log(`⏭️  スキップ: ${name} (既に存在します)`);
    skippedCount++;
    continue;
  }

  try {
    // ディレクトリが存在しない場合は作成
    const targetDir = path.dirname(target);
    if (!fs.existsSync(targetDir)) {
      fs.mkdirSync(targetDir, { recursive: true });
    }

    // ファイルをコピー
    fs.copyFileSync(example, target);
    console.log(`✅ コピー完了: ${name}`);
    copiedCount++;
  } catch (error) {
    console.error(`❌ エラー: ${name} のコピーに失敗しました:`, error.message);
  }
}

console.log(`\n📊 結果: ${copiedCount}個のファイルをコピー, ${skippedCount}個をスキップ\n`);

if (copiedCount > 0) {
  console.log('⚠️  重要: 本番環境で使用する前に、以下の設定を必ず変更してください：');
  console.log('  1. .env ファイルの AUTH_SHARED_SECRET');
  console.log('  2. .env ファイルの MOD_API_KEY');
  console.log('  3. .env ファイルの RESONITE_HEADLESS_PATH');
  console.log('  4. config/auth.json の jwtSecret と password');
  console.log('  5. config/security.json の allowedCidrs（必要に応じて）\n');
}

console.log('✨ セットアップが完了しました！\n');
console.log('次のステップ:');
console.log('  1. npm install を実行して依存関係をインストール');
console.log('  2. 設定ファイルを編集して環境に合わせる');
console.log('  3. npm run dev で開発サーバーを起動\n');

