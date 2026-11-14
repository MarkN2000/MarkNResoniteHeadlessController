// 実行: node scripts/check-config-import.mjs（web の既存依存のみ使用）
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from '../web/node_modules/vite/dist/node/index.js';

const server = await createServer({
  root: fileURLToPath(new URL('../web', import.meta.url)),
  configFile: false,
  optimizeDeps: { noDiscovery: true },
  server: { middlewareMode: true },
});
try {
  const { importConfig, getDuplicateUDPPorts } = await server.ssrLoadModule('/src/tabs/config/configModel.ts');
  const preserved = {
    dataFolder: 'C:\\current-data', cacheFolder: '/current-cache', logsFolder: null,
    loginCredential: 'CurrentAccount', loginPassword: '', usernameOverride: 'CurrentHost',
  };
  const current = Object.freeze({
    ...preserved, $schema: 'current-schema', comment: 'Keep this comment', tickRate: 60,
    maxConcurrentAssetTransfers: 128, allowedUrlHosts: ['https://example.com/'],
    autoSpawnItems: ['resrec:///current-item'], ExtraConfig: { KeepThisCase: ['current'] },
    startWorlds: [{ sessionName: 'Before' }],
  });
  const before = structuredClone(current);
  const source = {
    DataFolder: '/other-data', CacheFolder: '/other-cache', LogsFolder: '/other-logs',
    LoginCredential: 'OtherAccount', LoginPassword: 'other-secret', UsernameOverride: 'OtherHost',
    LOGINPASSWORD: 'another-secret', loginPassword: 'lowercase-secret',
    TickRate: 30, UniverseID: 'custom', AutoSpawnItems: ['resrec:///item'],
    StartWorlds: [{
      SessionName: 'Imported', LoadWorldURL: 'resrec:///world', Tags: ['new'],
      DefaultUserRoles: { MixedCaseUser: 'Admin' },
      ForcePorts: { LNL: 4000, QUIC: 4001, CustomProtocol: 4002 },
      ExtraWorld: { KeepThisCase: true },
    }, { IsEnabled: false, SessionName: 'Disabled' }],
    ExtraConfig: { KeepThisCase: ['value'] },
  };
  const imported = importConfig('\uFEFF' + JSON.stringify(source), current);
  assert.deepEqual(imported, { ...current, startWorlds: [{
    sessionName: 'Imported', loadWorldURL: 'resrec:///world', tags: ['new'],
    defaultUserRoles: { MixedCaseUser: 'Admin' },
    forcePorts: { lnl: 4000, quic: 4001, CustomProtocol: 4002 },
    ExtraWorld: { KeepThisCase: true },
  }, { isEnabled: false, sessionName: 'Disabled' }] });
  assert.equal(Object.hasOwn(imported, 'TickRate'), false);
  assert.equal(Object.hasOwn(imported, 'LoginPassword'), false);
  assert.equal(Object.hasOwn(imported, 'LOGINPASSWORD'), false);
  assert.deepEqual(current, before);

  // 現在未設定の項目を取り込み元の値で埋めず、null・空文字・編集途中のパスワードも維持する。
  const withoutSettings = importConfig(JSON.stringify(source), { startWorlds: [] });
  assert.deepEqual(withoutSettings, { startWorlds: imported.startWorlds });
  const edited = { ...current, dataFolder: null, cacheFolder: '', loginPassword: 'pending-secret' };
  const kept = importConfig('{"startWorlds":[]}', edited);
  assert.deepEqual(kept, { ...edited, startWorlds: [] });

  const sessionOnly = {
    $schema: 'https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json',
    startWorlds: [{
      isEnabled: true, maxUsers: 24, accessLevel: 'Private', hideFromPublicListing: true,
      sessionName: '読み込みテスト 予備',
      loadWorldURL: 'resrec:///U-Example/R-example',
      autoInviteUsernames: ['Sample Admin', 'SecondAdmin', 'guest-one', 'guest-two'],
      defaultUserRoles: { 'Sample Admin': 'Admin', SecondAdmin: 'Admin', 'guest-one': 'Admin' },
    }],
  };
  assert.deepEqual(importConfig(JSON.stringify(sessionOnly), current), { ...current, startWorlds: sessionOnly.startWorlds });

  const sample = JSON.parse(await readFile(new URL('../sample/default.json', import.meta.url), 'utf8'));
  assert.deepEqual(importConfig(JSON.stringify(sample), current).startWorlds, sample.startWorlds);
  const legacy = importConfig('{"startWorlds":[{"forcePort":4000},{"forcePorts":{"quic":4000}}]}', current);
  assert.deepEqual([...getDuplicateUDPPorts(legacy)], [4000]);

  for (const text of [
    '', '{', 'null', '[]', 'true', '{}', '{"startWorlds":null}', '{"startWorlds":{}}',
    '{"startWorlds":[null]}', '{"startWorlds":[[]]}', '{"startWorlds":[1]}',
  ]) assert.throws(() => importConfig(text, current), undefined, text);
  assert.deepEqual(current, before);
  console.log('JSON取り込み: セッションのみ反映、全体設定維持、省略項目、表記差、複数ワールド、未知項目、BOM、既存ポート検証、不正入力: OK');
} finally {
  await server.close();
}
