// 実行: web で npm run build 後、ルートで node scripts/check-app-restart.mjs
// Playwright は既存環境（または NODE_PATH）を使用。API は模擬応答し、実際の MRHC は再起動しない。
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { preview } from '../web/node_modules/vite/dist/node/index.js';
const { chromium } = createRequire(import.meta.url)('playwright');
const server = await preview({ root: fileURLToPath(new URL('../web', import.meta.url)), preview: { host: '127.0.0.1', port: 0, open: false } });
let browser;
try {
  browser = await chromium.launch({ channel: 'msedge', headless: true });
  const page = await browser.newPage({ locale: 'ja-JP', viewport: { width: 1100, height: 900 } });
  const errors = [], writes = [];
  let boot = 'old', pingCount = 0, loads = 0, fail = true, info = null;
  page.on('pageerror', error => errors.push(error.message));
  page.on('framenavigated', frame => { if (frame === page.mainFrame()) loads++; });
  await page.addInitScript(() => {
    window.EventSource = class extends EventTarget { close() {} };
  });
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    if (route.request().method() !== 'GET') writes.push(path);
    if (path === '/restart' && fail) {
      await route.fulfill({ status: 503, json: { ok: false, error: { code: 'restart_unavailable', message: '再起動テスト失敗' } } });
      return;
    }
    if (path === '/ping') pingCount++;
    const data = {
      '/status': { state: 'stopped', ready: false },
      '/configs': [],
      '/headless-credentials': { username: 'test', hasPassword: true },
      '/app-settings': { port: 8080, headlessConfigDir: '' },
      '/update/check': info,
      '/ping': { boot },
      '/restart': { accepted: true },
    }[path] ?? null;
    await route.fulfill({ json: { ok: true, data } });
  });
  await page.goto(server.resolvedUrls.local[0]);
  await page.getByText('設定', { exact: true }).click();
  const restart = page.getByRole('button', { name: 'MRHC を再起動', exact: true });
  const dialog = page.getByRole('dialog');
  await restart.click();
  await dialog.getByText('ヘッドレスも停止します。再起動後のヘッドレス起動は手動です。', { exact: true }).waitFor();
  await dialog.getByRole('button', { name: 'キャンセル', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(writes, []);
  assert.equal(pingCount, 0);

  await restart.click();
  await dialog.getByRole('button', { name: '実行', exact: true }).click();
  await page.getByText('再起動テスト失敗', { exact: true }).waitFor();
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(await restart.isEnabled(), true);

  fail = false;
  await restart.click();
  await dialog.getByRole('button', { name: '実行', exact: true }).click();
  await page.getByRole('heading', { name: 'MRHC を再起動しています', exact: true }).waitFor();
  assert.equal(await restart.count(), 0);
  // 旧プロセスが応答しても通常画面へ戻らず、boot が変化して初めて再読込する。
  await page.waitForResponse(response => response.url().endsWith('/ping'));
  assert.equal(loads, 1);
  const reloaded = page.waitForEvent('framenavigated', { predicate: frame => frame === page.mainFrame() });
  boot = 'new';
  await reloaded;
  await page.getByText('設定', { exact: true }).waitFor();
  assert.equal(loads, 2);
  assert.deepEqual(writes, ['/restart', '/restart']);

  // 自己更新後の既存の再起動導線と版表示も維持する。
  info = { current: 'v1.0.0', latest: 'v2.0.0', staged: 'v2.0.0', updateAvailable: true, currentIsRelease: true, includePrereleases: false, goos: 'windows' };
  await page.reload();
  await page.getByRole('button', { name: 'menu', exact: true }).click();
  await page.getByRole('menuitem', { name: /再起動待ち/ }).click();
  await dialog.getByRole('button', { name: '今すぐ再起動する', exact: true }).click();
  await page.getByText('再起動後は v2.0.0 になります。', { exact: true }).waitFor();
  assert.deepEqual(writes, ['/restart', '/restart', '/restart']);
  assert.deepEqual(errors, []);
  console.log('再起動の確認・キャンセル・失敗・更新なしの成功・boot 変化での再接続・自己更新の既存導線: OK');
} finally {
  await browser?.close();
  await new Promise(resolve => server.httpServer.close(resolve));
}
