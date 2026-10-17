// 実行: cd web && npm run build、その後ルートで node scripts/check-mod-upload.mjs
// Playwrightを既存環境（またはNODE_PATH）から使用。APIは全て模擬応答し、実際のDLLは送信しない。
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { preview } from '../web/node_modules/vite/dist/node/index.js';
const { chromium } = createRequire(import.meta.url)('playwright');
const server = await preview({ root: fileURLToPath(new URL('../web', import.meta.url)), preview: { host: '127.0.0.1', port: 0, open: false } });
let browser;
try {
  browser = await chromium.launch({ channel: 'msedge', headless: true });
  const page = await browser.newPage({ locale: 'ja-JP', viewport: { width: 1100, height: 780 } });
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const entry = { id: 'a'.repeat(32), enabled: true, current: { files: [{ name: 'Existing.dll', hash: 'b'.repeat(64) }], addedAt: '2026-10-18T00:00:00Z' } };
  let uploads = [], pendingUpload, finishUpload;
  await page.addInitScript(() => {
    // SSEの現在状態だけを制御して、ダイアログ表示中の稼働開始も検査する。
    window.EventSource = class extends EventTarget {
      constructor() {
        super(); window.checkStatus = state => this.dispatchEvent(new MessageEvent('status', { data: JSON.stringify({ state, ready: false }) }));
        setTimeout(() => window.checkStatus('stopped'), 0);
      }
      close() {}
    };
  });
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url());
    let data = null;
    if (url.pathname.endsWith('/status')) data = { state: 'stopped', ready: false };
    if (url.pathname.endsWith('/configs')) data = [];
    if (url.pathname.endsWith('/headless-credentials')) data = { username: 'test', hasPassword: true };
    if (url.pathname.endsWith('/mods')) data = { entries: [entry], canModify: true, busy: false };
    if (url.pathname.endsWith('/mods/upload')) {
      uploads.push({ name: url.searchParams.get('name'), id: url.searchParams.get('id'), body: route.request().postDataBuffer() });
      if (pendingUpload) await pendingUpload;
      data = { entries: [entry] };
    }
    await route.fulfill({ json: { ok: true, data } });
  });
  await page.goto(server.resolvedUrls.local[0]);
  await page.getByText('Mod', { exact: true }).click();
  await page.getByRole('button', { name: 'DLLをアップロード', exact: true }).click();
  const dialog = page.getByRole('dialog');
  const zone = dialog.getByRole('region', { name: 'DLLファイル' });
  const add = dialog.getByRole('button', { name: '追加', exact: true });
  const file = { name: 'Example.DLL', mimeType: 'application/octet-stream', buffer: Buffer.from('test-dll') };
  async function drop(files, type = 'drop') {
    const dataTransfer = await page.evaluateHandle(files => {
      const dt = new DataTransfer();
      for (const f of files) dt.items.add(new File([new Uint8Array(f.size ?? 8)], f.name));
      return dt;
    }, files);
    await zone.dispatchEvent(type, { dataTransfer });
    await dataTransfer.dispose();
  }
  assert.equal(await dialog.getByRole('textbox').count(), 0);
  assert.equal(await add.isDisabled(), true);
  // 見えているボタンがOSのファイル選択を開く。
  const chooser = page.waitForEvent('filechooser');
  await dialog.getByRole('button', { name: 'ファイルを選択', exact: true }).click();
  await (await chooser).setFiles(file);
  await dialog.getByText('Example.DLL (0.00 MiB)', { exact: true }).waitFor();
  assert.equal(uploads.length, 0);
  assert.equal(await add.isEnabled(), true);
  for (const files of [[{ name: 'a.dll' }, { name: 'b.dll' }], [{ name: 'a.txt' }], [{ name: 'a.dll', size: 0 }], [{ name: 'a.dll', size: 32 * 1024 * 1024 + 1 }]]) {
    await drop(files);
    await dialog.getByRole('alert').waitFor();
    assert.equal(await add.isDisabled(), true);
  }
  await drop([{ name: 'Drop.dll' }], 'dragenter');
  await dialog.getByText('ここにドロップ', { exact: true }).waitFor();
  await drop([{ name: 'Drop.dll' }]);
  await dialog.getByText('Drop.dll (0.00 MiB)', { exact: true }).waitFor();
  assert.equal(await dialog.getByRole('alert').count(), 0);
  await page.evaluate(() => window.checkStatus('running'));
  await page.waitForFunction(() => document.querySelector('[role="region"]')?.getAttribute('aria-disabled') === 'true');
  await drop([{ name: 'Blocked.dll' }]);
  assert.equal(await add.isDisabled(), true);
  await dialog.getByText('Drop.dll (0.00 MiB)', { exact: true }).waitFor();
  await page.evaluate(() => window.checkStatus('stopped'));
  await page.waitForFunction(() => document.querySelector('[role="region"]')?.getAttribute('aria-disabled') === 'false');
  await page.setViewportSize({ width: 360, height: 780 });
  assert.equal(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth), true);
  if (process.env.MOD_UPLOAD_SCREENSHOT) await page.screenshot({ path: process.env.MOD_UPLOAD_SCREENSHOT });
  pendingUpload = new Promise(resolve => { finishUpload = resolve; });
  await add.click();
  await page.waitForFunction(() => document.querySelector('[role="region"]')?.getAttribute('aria-disabled') === 'true');
  await drop([{ name: 'Busy.dll' }]);
  assert.equal(uploads.length, 1);
  assert.equal(uploads[0].name, 'Drop.dll');
  assert.equal(uploads[0].id, '');
  assert.equal(uploads[0].body.length, 8);
  finishUpload(); pendingUpload = null;
  await dialog.waitFor({ state: 'hidden' });
  await page.setViewportSize({ width: 1100, height: 780 });
  await page.getByRole('button', { name: 'Existing.dllの操作', exact: true }).click();
  await page.getByRole('menuitem', { name: 'DLLを差し替え', exact: true }).click();
  await dialog.locator('input[type=file]').setInputFiles(file);
  await dialog.getByRole('button', { name: '適用', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(uploads.length, 2);
  assert.equal(uploads[1].id, entry.id);
  assert.deepEqual(uploads[1].body, file.buffer);
  assert.deepEqual(errors, []);
  console.log('DLL選択・ドロップ・入力検査・稼働中/送信中の拒否・追加/差し替え・狭幅表示: OK');
} finally {
  await browser?.close();
  await new Promise(resolve => server.httpServer.close(resolve));
}
