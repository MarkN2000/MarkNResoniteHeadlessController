// 実行: web で npm run build 後、node scripts/check-ui-state.mjs
// Playwright は既存環境（または NODE_PATH）を使用。全通信を模擬し、実設定には触れない。
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import ts from '../web/node_modules/typescript/lib/typescript.js';
const { chromium } = createRequire(import.meta.url)('playwright');

// 実際のフックを使い、取得待ちの非表示→再表示でも取得が並行しないことを確認する。
const hook = await readFile(new URL('../web/src/hooks/useVisiblePolling.ts', import.meta.url), 'utf8');
const hookJS = ts.transpileModule(hook.replace(/^import .*;\r?\n/, '').replace('export function', 'function'), {}).outputText;
const timers = new Map(), listeners = new Map(), doc = { hidden: false,
  addEventListener: (name, fn) => listeners.set(name, fn), removeEventListener: name => listeners.delete(name) };
let timerId = 0, cleanup, releasePoll, active = 0, maxActive = 0;
const usePolling = new Function('useEffect', 'useRef', 'document', 'setTimeout', 'clearTimeout', hookJS + '; return useVisiblePolling;')(
  effect => { cleanup = effect(); }, value => ({ current: value }), doc,
  fn => { timers.set(++timerId, fn); return timerId; }, id => timers.delete(id));
usePolling(async () => {
  active++; maxActive = Math.max(maxActive, active);
  await new Promise(resolve => { releasePoll = resolve; });
  active--;
}, 1000);
timers.values().next().value();
doc.hidden = true; listeners.get('visibilitychange')();
doc.hidden = false; listeners.get('visibilitychange')();
assert.equal(maxActive, 1);
releasePoll();
await new Promise(resolve => setImmediate(resolve));
assert.equal(timers.size, 1);
cleanup();
assert.equal(timers.size, 0);

const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage({ locale: 'ja-JP', viewport: { width: 1300, height: 950 } });
  const errors = [], writes = [];
  const store = {
    alpha: { comment: 'Alpha', startWorlds: [
      { sessionName: 'World A', isEnabled: true, tags: ['tag-a'], defaultUserRoles: { 'User-A': 'Admin' }, autoInviteUsernames: ['Invite-A'] },
      { sessionName: 'World B', isEnabled: true, tags: ['tag-b'], defaultUserRoles: { 'User-B': 'Guest' }, autoInviteUsernames: ['Invite-B'] },
    ] },
    beta: { comment: 'Beta', startWorlds: [] },
    gamma: { comment: 'Gamma', startWorlds: [] },
  };
  let rc = {
    scheduled: ['a', 'b'].map((id, hour) => ({ id, enabled: true, type: 'daily', hour, minute: 0, configName: '' })),
    waitControl: { quietWaitMin: 58, announceWaitMin: 2 },
    preActions: { announce: { enabled: false, templateId: '', message: '', speakerId: 0 }, sessionChanges: { setPrivate: false, setMaxUsersOne: true, renameEnabled: false, renameTo: '' } },
    crashRecovery: { enabled: true, maxCrashes: 3, windowMinutes: 10 },
    updateOnScheduledRestart: false, updateBeforeManualStart: false,
  };
  let delayBeta = false, releaseBeta, delaySave = false, releaseSave, loginMode = false, failLogin = true;
  let delayConfigSave = false, releaseConfigSave;
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/*', route => {
    const path = new URL(route.request().url()).pathname;
    return route.fulfill({ path: fileURLToPath(new URL('../web/dist/' + (path === '/' ? 'index.html' : path.slice(1)), import.meta.url)) });
  });
  await page.addInitScript(() => {
    window.EventSource = class extends EventTarget {
      constructor() { super(); window.events = this; }
      close() {}
    };
  });
  await page.route('**/api/v1/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname.replace('/api/v1', '');
    if (path === '/status' && loginMode) return route.fulfill({ status: 401, json: { ok: false } });
    if (path === '/login' && failLogin) return route.abort('failed');
    if (path === '/headless-configs/beta' && delayBeta) await new Promise(resolve => { releaseBeta = resolve; });
    if (request.method() === 'PUT') {
      const from = new URL(request.url()).searchParams.get('from');
      writes.push({ path, from, body: request.postDataJSON() });
      if (path === '/restart-config') {
        if (delaySave) await new Promise(resolve => { releaseSave = resolve; });
        rc = request.postDataJSON();
      } else if (path.startsWith('/headless-configs/')) {
        if (delayConfigSave) await new Promise(resolve => { releaseConfigSave = resolve; });
        if (from && !store[from]) return route.fulfill({ status: 404, json: { ok: false } });
        store[path.split('/').at(-1)] = request.postDataJSON();
        if (from) delete store[from];
      }
    }
    const data = {
      '/status': { state: 'running', ready: true }, '/sessions': [],
      '/headless-configs': Object.entries(store).map(([name, cfg]) => ({ name, comment: cfg.comment, worldCount: cfg.startWorlds.length })),
      '/headless-configs/last-used': { lastUsed: 'alpha' },
      '/headless-credentials': { username: 'Example', hasPassword: true },
      '/headless-config-defaults': { dataFolder: '/data', cacheFolder: '/cache' },
      '/restart-config': rc, '/restart-status': { running: true, inProgress: false, uptimeSeconds: 10 },
    }[path] ?? (path.startsWith('/headless-configs/') ? store[path.split('/').at(-1)] : null);
    return route.fulfill({ json: { ok: true, data } });
  });
  const save = () => page.getByRole('button', { name: '保存', exact: true }).first();
  const dialog = page.getByRole('dialog');
  await page.goto('http://mrhc.test/');
  await page.getByText('コンフィグ', { exact: true }).first().click();
  await page.locator('input[value="User-A"]').waitFor();
  await page.getByRole('button', { name: 'このワールドを削除', exact: true }).first().click();
  await dialog.getByRole('button', { name: '実行', exact: true }).click();
  await page.locator('input[value="User-B"]').waitFor();
  assert.equal(await page.locator('input[value="User-A"]').count(), 0);
  assert.equal(await page.locator('input[value="Invite-B"]').count(), 1);
  assert.equal(await page.locator('input[value="tag-b"]').count(), 1);
  await page.locator('input[value="User-B"]').fill('Edited-B');
  await save().click();
  await page.waitForFunction(() => document.querySelector('input[placeholder="my-config"]')?.value === 'alpha' && [...document.querySelectorAll('button')].some(b => b.textContent === '保存' && b.disabled));
  assert.deepEqual(store.alpha.startWorlds[0].defaultUserRoles, { 'Edited-B': 'Guest' });
  assert.equal(store.alpha.startWorlds[0].sessionName, 'World B');

  delayBeta = true;
  await page.getByRole('button', { name: 'beta', exact: true }).click();
  await page.getByRole('button', { name: 'gamma', exact: true }).click();
  await page.locator('input[value="gamma"]').waitFor();
  await page.locator('textarea').first().fill('Gamma edited');
  releaseBeta();
  await page.waitForResponse(r => r.url().endsWith('/headless-configs/beta'));
  assert.equal(await page.locator('input[placeholder="my-config"]').inputValue(), 'gamma');
  assert.equal(await page.locator('textarea').first().inputValue(), 'Gamma edited');

  // 保存応答待ちの追加入力を維持し、改名成功後の次回保存も新しい名前を使う。
  await page.locator('input[placeholder="my-config"]').fill('renamed');
  delayConfigSave = true;
  await save().click();
  await page.locator('textarea').first().fill('Gamma latest edit');
  delayConfigSave = false; releaseConfigSave();
  await page.waitForResponse(r => r.url().includes('/headless-configs/renamed') && r.request().method() === 'PUT');
  await page.waitForFunction(() => [...document.querySelectorAll('button')].some(b => b.textContent === '保存' && !b.disabled));
  assert.equal(await page.locator('textarea').first().inputValue(), 'Gamma latest edit');
  await save().click();
  await page.waitForFunction(() => [...document.querySelectorAll('button')].some(b => b.textContent === '保存' && b.disabled));
  assert.equal(writes.at(-1).from, null);
  assert.equal(store.renamed.comment, 'Gamma latest edit');

  await page.getByText('スケジュール', { exact: true }).first().click();
  const toggles = page.getByRole('switch');
  await toggles.first().waitFor({ state: 'attached' });
  const clickToggle = async index => page.locator(`label[for="${await toggles.nth(index).getAttribute('id')}"]`).click();
  await page.locator('input[value="58"]').fill('59');
  const saveSettings = page.getByRole('button', { name: '設定を保存', exact: true });
  delaySave = true;
  await clickToggle(0);
  await page.waitForFunction(() => document.querySelector('input[role="switch"]')?.disabled);
  assert.equal(await toggles.nth(1).isDisabled(), true);
  assert.equal(await saveSettings.isDisabled(), true);
  assert.equal(writes.filter(w => w.path === '/restart-config').length, 1);
  delaySave = false; releaseSave();
  await page.waitForFunction(() => !document.querySelector('input[role="switch"]')?.disabled);
  await clickToggle(1);
  await page.waitForFunction(() => !document.querySelector('input[role="switch"]')?.disabled);
  assert.deepEqual(rc.scheduled.map(s => s.enabled), [false, false]);
  assert.equal(rc.waitControl.quietWaitMin, 58);
  assert.equal(await page.locator('input[value="59"]').count(), 1);
  delaySave = true;
  await saveSettings.click();
  await page.waitForFunction(() => document.querySelector('input[role="switch"]')?.disabled);
  assert.equal(await toggles.nth(0).isDisabled(), true);
  delaySave = false; releaseSave();
  await page.waitForFunction(() => !document.querySelector('input[role="switch"]')?.disabled);
  assert.equal(rc.waitControl.quietWaitMin, 59);
  assert.deepEqual(rc.scheduled.map(s => s.enabled), [false, false]);

  const emit = (type, data) => page.evaluate(({ type, data }) => window.events.dispatchEvent(new MessageEvent(type, { data: JSON.stringify(data) })), { type, data });
  await emit('status', { state: 'running', ready: true });
  await page.getByText('コマンド', { exact: true }).first().click();
  await emit('boot', 'old');
  await emit('log', { seq: 100, kind: 'out', text: 'Old process log' });
  await page.getByText('Old process log', { exact: true }).waitFor();
  await emit('boot', 'old');
  await emit('log', { seq: 100, kind: 'out', text: 'Duplicate log' });
  assert.equal(await page.getByText('Duplicate log', { exact: true }).count(), 0);
  await emit('boot', 'new');
  await emit('log', { seq: 1, kind: 'out', text: 'New process log' });
  await page.getByText('New process log', { exact: true }).waitFor();
  assert.equal(await page.getByText('Old process log', { exact: true }).count(), 0);

  loginMode = true;
  await page.reload();
  await page.getByPlaceholder('管理パスワード').fill('test-only');
  const login = page.getByRole('button', { name: 'ログイン', exact: true });
  await login.click();
  await page.getByText('サーバーに接続できません', { exact: true }).waitFor();
  assert.equal(await login.isEnabled(), true);
  failLogin = false;
  await login.click();
  await page.getByText('コンフィグ', { exact: true }).first().waitFor();
  assert.deepEqual(errors, []);
  console.log('UI状態: ワールド削除・予定の排他保存・古い取得応答・ポーリング重複・ログ世代・ログイン再試行: OK');
} finally {
  await browser.close();
}
