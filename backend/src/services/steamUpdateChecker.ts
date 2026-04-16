import { spawn, type ChildProcess } from 'node:child_process';
import { EventEmitter } from 'node:events';
import { promises as fs } from 'node:fs';
import path from 'node:path';
import type { SteamConfig } from './steamConfig.js';
import type { SteamUpdateCheckResult } from '../../../shared/src/index.js';
import {
  parseKeyValues,
  getByPath,
  type KeyValuesNode
} from './steamManifestReader.js';
import { steamUpdateBus } from './steamUpdateBus.js';

export type { SteamUpdateCheckResult };

/**
 * SteamCMD `+app_status <appId> +app_info_update 1 +app_info_print <appId> +quit` を実行して、
 * ローカルにインストール済みの buildid（app_status）と Steam 側最新の buildid（app_info_print）
 * を取得・比較するサービス。
 *
 * 発火するイベント:
 *   'result' (result: SteamUpdateCheckResult)  1 回のチェックが終わったとき（成功・失敗問わず）
 *   'log'    (text: string)                    デバッグ用（現状は使っていないが将来用）
 *
 * 使い方:
 *   const checker = new SteamUpdateChecker(() => loadSteamConfig());
 *   checker.on('result', (r) => steamUpdateBus.emitCheckResult(r));
 *   await checker.check();          // 同期的に1回実行
 *   checker.startPeriodic({ initialDelayMs: 60_000, intervalMs: 30 * 60_000 });
 *
 * 同時実行は常に 1 本にまとめる（dedupe）。
 * `app_update` と同じ installDir を触るため、`steamUpdateBus.isUpdateActive()` が
 * true の場合はチェックをスキップする。逆に app_update が新規に開始される直前には
 * `abort()` を呼んで check を中断させる想定（steamRoutes から）。
 */
export class SteamUpdateChecker extends EventEmitter {
  private readonly loadConfig: () => Promise<SteamConfig>;

  /** 直前のチェック結果（成功・失敗問わず最新のもの） */
  private lastResult: SteamUpdateCheckResult | null = null;
  /** 直前の成功チェック結果（エラー時のキャッシュ復元用） */
  private lastSuccessResult: SteamUpdateCheckResult | null = null;

  /** check() 実行中の in-flight Promise（dedupe 用） */
  private currentCheck: Promise<SteamUpdateCheckResult> | null = null;
  /** check() 実行中の child process（abort 用） */
  private currentChild: ChildProcess | null = null;
  /** abort 要求フラグ */
  private aborted = false;

  /** 定期実行タイマー */
  private initialTimer: NodeJS.Timeout | null = null;
  private periodicTimer: NodeJS.Timeout | null = null;

  /** レート制限に引っかかった場合の cooldown（Unix ms） */
  private cooldownUntil = 0;

  constructor(loadConfig: () => Promise<SteamConfig>) {
    super();
    this.loadConfig = loadConfig;
  }

  getCached(): SteamUpdateCheckResult | null {
    return this.lastResult;
  }

  isChecking(): boolean {
    return this.currentCheck !== null;
  }

  /**
   * 最新 buildid のチェックを実行する。
   *
   * - force=false で、キャッシュが TTL 内ならキャッシュを返す
   * - 既に check が走っているなら同じ Promise を返す（dedupe）
   * - app_update が走っているならスキップし、直前のキャッシュを返す
   */
  async check(force: boolean = false): Promise<SteamUpdateCheckResult> {
    // 既に別の check が走っていれば、それに相乗りする
    if (this.currentCheck) {
      return this.currentCheck;
    }

    // app_update 実行中は SteamCMD を新規に立ち上げない（ロック競合回避）
    if (steamUpdateBus.isUpdateActive()) {
      if (this.lastResult) {
        return this.lastResult;
      }
      // まだ一度もチェックが走っていない場合は、pending な結果を返す
      return this.buildPendingResult('アップデート実行中のためチェックをスキップしました');
    }

    // レート制限 cooldown 中はスキップ。
    // force=true でもバイパスしない。cooldown は Steam 側のレート制限を踏んだ結果として
    // 設定されているので、強引に再試行しても即 "Rate Limit Exceeded" で失敗するだけ。
    // 定期実行は check(true) で呼ばれるため、ここを force で貫通させると
    // 定期タイマーが cooldown を無視してレート制限を再度踏みに行ってしまう。
    if (Date.now() < this.cooldownUntil) {
      if (this.lastResult) return this.lastResult;
      return this.buildPendingResult('Steam ログインのレート制限のため一時的にチェックをスキップしています');
    }

    // キャッシュ TTL 判定（force=false のみ）
    if (!force && this.lastSuccessResult) {
      const ageMs = Date.now() - new Date(this.lastSuccessResult.checkedAt).getTime();
      const ttlMs = 15 * 60 * 1000; // 15 分以内なら再実行しない
      if (ageMs < ttlMs) {
        return this.lastSuccessResult;
      }
    }

    this.aborted = false;
    this.currentCheck = this.runCheck();
    try {
      const result = await this.currentCheck;
      return result;
    } finally {
      this.currentCheck = null;
      this.currentChild = null;
    }
  }

  /**
   * 現在進行中の check を中断する。
   * steamRoutes の `POST /api/steam/update` 開始前に呼ぶことで、
   * app_update と app_info_print の同時実行を防ぐ。
   */
  async abort(): Promise<void> {
    if (!this.currentCheck) return;
    this.aborted = true;
    if (this.currentChild) {
      try {
        this.currentChild.kill();
      } catch {
        // ignore
      }
    }
    try {
      await this.currentCheck;
    } catch {
      // ignore
    }
  }

  /**
   * 定期実行を開始する。
   * `initialDelayMs` 後に 1 回、その後 `intervalMs` ごとに check() を呼ぶ。
   * 既に実行中なら再設定する（古いタイマーは破棄）。
   */
  startPeriodic(opts: { initialDelayMs: number; intervalMs: number }): void {
    this.stopPeriodic();

    const MIN_INTERVAL_MS = 10 * 60 * 1000; // 10 分未満は警告
    if (opts.intervalMs < MIN_INTERVAL_MS) {
      console.warn(
        `[SteamUpdateChecker] interval=${opts.intervalMs}ms は短すぎます（Steam のレート制限に注意）。` +
          `推奨は 30 分以上です。`
      );
    }

    console.log(
      `[SteamUpdateChecker] periodic check scheduled: initialDelay=${opts.initialDelayMs}ms interval=${opts.intervalMs}ms`
    );

    this.initialTimer = setTimeout(() => {
      this.initialTimer = null;
      this.check(true).catch((err) => {
        console.warn('[SteamUpdateChecker] initial check failed:', err?.message ?? err);
      });

      this.periodicTimer = setInterval(() => {
        this.check(true).catch((err) => {
          console.warn('[SteamUpdateChecker] periodic check failed:', err?.message ?? err);
        });
      }, opts.intervalMs);
    }, opts.initialDelayMs);
  }

  stopPeriodic(): void {
    if (this.initialTimer) {
      clearTimeout(this.initialTimer);
      this.initialTimer = null;
    }
    if (this.periodicTimer) {
      clearInterval(this.periodicTimer);
      this.periodicTimer = null;
    }
  }

  // ========================================================================
  // 内部実装
  // ========================================================================

  private buildPendingResult(reason: string): SteamUpdateCheckResult {
    return {
      branch: '',
      installedBuildId: null,
      latestBuildId: null,
      latestTimeUpdated: null,
      updateAvailable: false,
      checkedAt: new Date().toISOString(),
      error: reason
    };
  }

  private async runCheck(): Promise<SteamUpdateCheckResult> {
    const config = await this.loadConfig();

    const steamcmdPath = config.steamCmd.path;
    const appId = config.resonite.appId;
    const installDir = config.resonite.installDir;
    const branch = (config.resonite.branch ?? '').trim() || 'public';
    const username = config.account.username;
    const password = config.account.password;

    if (!steamcmdPath || !username || !password || !appId || !installDir) {
      const result: SteamUpdateCheckResult = {
        branch,
        installedBuildId: null,
        latestBuildId: null,
        latestTimeUpdated: null,
        updateAvailable: false,
        checkedAt: new Date().toISOString(),
        error: 'Steam 設定が不完全です（steam.json の steamCmd.path / account / resonite を確認してください）'
      };
      this.recordResult(result);
      return result;
    }

    // SteamCMD の appcache/appinfo.vdf を削除して、app_info_print がキャッシュ済みの
    // 古いデータを返すのを防ぐ（+app_info_update 1 だけでは不十分なケースがある）
    try {
      const appcachePath = path.resolve(path.dirname(steamcmdPath), 'appcache', 'appinfo.vdf');
      await fs.unlink(appcachePath);
    } catch (err: any) {
      if (err?.code !== 'ENOENT') {
        console.warn('[SteamUpdateChecker] Failed to delete appinfo.vdf:', err?.message ?? err);
      }
    }

    // SteamCMD を起動して app_status + app_info_print の出力を取得する
    const spawnResult = await this.runSteamCmd(steamcmdPath, {
      installDir,
      username,
      password,
      appId
    });

    // app_status の出力からインストール済み BuildID を抽出
    const installedBuildId = this.extractInstalledBuildId(spawnResult.stdout);

    if (spawnResult.error) {
      // SteamCMD 実行が失敗してもログイン成功済みのキャッシュがあれば保持する
      const result: SteamUpdateCheckResult = {
        branch,
        installedBuildId,
        latestBuildId: this.lastSuccessResult?.latestBuildId ?? null,
        latestTimeUpdated: this.lastSuccessResult?.latestTimeUpdated ?? null,
        updateAvailable: this.lastSuccessResult?.updateAvailable ?? false,
        checkedAt: new Date().toISOString(),
        error: spawnResult.error
      };
      this.recordResult(result);
      return result;
    }

    // stdout をパースして branches.<branch>.buildid を抽出
    const { latestBuildId, latestTimeUpdated, parseError } = this.extractLatestBuild(
      spawnResult.stdout,
      appId,
      branch
    );

    if (parseError) {
      console.warn(
        `[SteamUpdateChecker] parse failed for branch="${branch}": ${parseError}. rawOutput size=${spawnResult.stdout.length}`
      );
      const result: SteamUpdateCheckResult = {
        branch,
        installedBuildId,
        latestBuildId: this.lastSuccessResult?.latestBuildId ?? null,
        latestTimeUpdated: this.lastSuccessResult?.latestTimeUpdated ?? null,
        updateAvailable: this.lastSuccessResult?.updateAvailable ?? false,
        checkedAt: new Date().toISOString(),
        error: parseError
      };
      this.recordResult(result);
      return result;
    }

    const updateAvailable =
      latestBuildId !== null && (installedBuildId === null || latestBuildId !== installedBuildId);

    const result: SteamUpdateCheckResult = {
      branch,
      installedBuildId,
      latestBuildId,
      latestTimeUpdated,
      updateAvailable,
      checkedAt: new Date().toISOString(),
      error: null
    };

    console.log(
      `[SteamUpdateChecker] check result: branch=${branch} installed=${installedBuildId ?? '(none)'} ` +
        `latest=${latestBuildId ?? '(unknown)'} updateAvailable=${updateAvailable}`
    );

    this.recordResult(result);
    this.lastSuccessResult = result;
    return result;
  }

  /**
   * SteamCMD を spawn し、`+app_status <appId> +app_info_update 1 +app_info_print <appId> +quit`
   * の stdout を全て回収して返す。タイムアウト・aborted・起動失敗時はエラー文字列を返す。
   */
  private runSteamCmd(
    steamcmdPath: string,
    params: { installDir: string; username: string; password: string; appId: string }
  ): Promise<{ stdout: string; error: string | null }> {
    return new Promise((resolve) => {
      // `+force_install_dir` は必ず `+login` より前に置く（app_update と同じ制約）
      const args = [
        '+force_install_dir',
        params.installDir,
        '+login',
        params.username,
        params.password,
        '+app_status',
        params.appId,
        '+app_info_update',
        '1',
        '+app_info_print',
        params.appId,
        '+quit'
      ];

      const TIMEOUT_MS = Number(process.env.STEAM_UPDATE_CHECK_TIMEOUT_MS || 90 * 1000);
      let timeout: NodeJS.Timeout | null = null;

      let stdout = '';
      let stderr = '';
      let finished = false;

      const cwd = path.dirname(steamcmdPath);

      const child = spawn(steamcmdPath, args, {
        cwd,
        windowsHide: true
      });
      this.currentChild = child;

      const finalize = (error: string | null): void => {
        if (finished) return;
        finished = true;
        if (timeout) {
          clearTimeout(timeout);
          timeout = null;
        }
        resolve({ stdout, error });
      };

      timeout = setTimeout(() => {
        try {
          child.kill();
        } catch {
          // ignore
        }
        finalize(`SteamCMD の実行がタイムアウトしました（${TIMEOUT_MS}ms）`);
      }, TIMEOUT_MS);

      child.stdout.on('data', (data: Buffer) => {
        stdout += data.toString('utf-8');
      });
      child.stderr.on('data', (data: Buffer) => {
        stderr += data.toString('utf-8');
      });

      child.on('error', (error) => {
        console.warn('[SteamUpdateChecker] SteamCMD spawn error:', error);
        finalize(`SteamCMD の起動に失敗しました: ${error.message}`);
      });

      child.on('close', (code) => {
        if (this.aborted) {
          finalize('アップデート実行のためチェックを中断しました');
          return;
        }

        const combined = (stdout + stderr).toLowerCase();

        // レート制限検出（次回以降の cooldown を設定）
        if (combined.includes('rate limit exceeded')) {
          this.cooldownUntil = Date.now() + 30 * 60 * 1000;
          console.warn('[SteamUpdateChecker] Steam rate limit detected; cooldown for 30 minutes');
          finalize('Steam ログインのレート制限に到達しました。しばらく時間を置いてから再試行してください。');
          return;
        }

        // Steam Guard プロンプト検出（自動チェックでは Guard 入力は求めない）
        if (
          combined.includes('steam guard') ||
          combined.includes('two-factor code') ||
          combined.includes('two factor code')
        ) {
          finalize('Steam Guard コードが要求されました（自動チェックでは対応しません）');
          return;
        }

        // ログイン失敗系のエラーキーワード
        const loginErrorKeywords = [
          'invalid password',
          'no subscription',
          'no licenses',
          'failed login',
          'account logon denied'
        ];
        const loginError = loginErrorKeywords.find((k) => combined.includes(k));
        if (loginError) {
          finalize(`SteamCMD ログインに失敗しました: "${loginError}"`);
          return;
        }

        if (code !== 0) {
          finalize(`SteamCMD が異常終了しました（exit=${code}）`);
          return;
        }

        finalize(null);
      });
    });
  }

  /**
   * `+app_status` の stdout から、インストール済みの BuildID を抽出する。
   *
   * app_status の出力例:
   *   - size on disk: 3367596428 bytes, BuildID 22787313
   *
   * `force_install_dir` 使用時でも正しい BuildID を返す（appmanifest とは異なる）。
   */
  private extractInstalledBuildId(rawOutput: string): string | null {
    const match = rawOutput.match(/BuildID\s+(\d+)/);
    if (!match?.[1]) {
      console.warn('[SteamUpdateChecker] app_status output did not contain BuildID');
      return null;
    }
    const buildId = match[1];
    // "0" は無効値（未インストール等）
    if (buildId === '0') {
      console.warn('[SteamUpdateChecker] app_status returned BuildID 0 (not installed?)');
      return null;
    }
    return buildId;
  }

  /**
   * `+app_info_print` の stdout から、指定ブランチの buildid / timeupdated を抽出する。
   *
   * SteamCMD の出力は前後にプロンプトログが混じる形で以下のような KeyValues ブロックを含む:
   *
   *   "2519830"
   *   {
   *       "common" { ... }
   *       "depots"
   *       {
   *           "branches"
   *           {
   *               "headless"
   *               {
   *                   "buildid"     "21155186"
   *                   "timeupdated" "1765497533"
   *               }
   *           }
   *       }
   *   }
   */
  private extractLatestBuild(
    rawOutput: string,
    appId: string,
    branch: string
  ): { latestBuildId: string | null; latestTimeUpdated: number | null; parseError: string | null } {
    // app_info_print のブロック先頭（"2519830" の行）を探し、その後の { } を括って切り出す
    const marker = `"${appId}"`;
    const markerIdx = rawOutput.indexOf(marker);
    if (markerIdx < 0) {
      return {
        latestBuildId: null,
        latestTimeUpdated: null,
        parseError: `app_info_print の出力内に "${appId}" ブロックが見つかりませんでした`
      };
    }

    // marker の直後の最初の '{' を探す
    const blockStart = rawOutput.indexOf('{', markerIdx + marker.length);
    if (blockStart < 0) {
      return {
        latestBuildId: null,
        latestTimeUpdated: null,
        parseError: 'app_info_print 出力のブロック開始 `{` が見つかりませんでした'
      };
    }

    // 対応する閉じ `}` を深さカウントで探す
    let depth = 0;
    let blockEnd = -1;
    let inString = false;
    for (let i = blockStart; i < rawOutput.length; i++) {
      const ch = rawOutput[i];
      if (ch === '"') {
        // 非常に簡易な文字列スキップ（エスケープは気にしない）
        inString = !inString;
        continue;
      }
      if (inString) continue;
      if (ch === '{') depth++;
      else if (ch === '}') {
        depth--;
        if (depth === 0) {
          blockEnd = i;
          break;
        }
      }
    }

    if (blockEnd < 0) {
      return {
        latestBuildId: null,
        latestTimeUpdated: null,
        parseError: 'app_info_print 出力の閉じ `}` が見つかりませんでした'
      };
    }

    // "2519830" { ... } 相当を一括で KeyValues として投げるために、先頭に marker を付け直す
    const kvText = `${marker}\n${rawOutput.substring(blockStart, blockEnd + 1)}`;

    let parsed: KeyValuesNode;
    try {
      parsed = parseKeyValues(kvText);
    } catch (err: any) {
      return {
        latestBuildId: null,
        latestTimeUpdated: null,
        parseError: `KeyValues のパースに失敗しました: ${err?.message ?? err}`
      };
    }

    const branchNode = getByPath(parsed, `${appId}.depots.branches.${branch}`);
    if (!branchNode || typeof branchNode !== 'object') {
      return {
        latestBuildId: null,
        latestTimeUpdated: null,
        parseError: `ブランチ "${branch}" の情報が app_info_print に含まれていませんでした`
      };
    }

    const buildidRaw = getByPath(branchNode, 'buildid');
    const buildid = typeof buildidRaw === 'string' && buildidRaw !== '' ? buildidRaw : null;

    const timeupdatedRaw = getByPath(branchNode, 'timeupdated');
    const timeupdated =
      typeof timeupdatedRaw === 'string' && timeupdatedRaw !== ''
        ? Number.parseInt(timeupdatedRaw, 10)
        : null;

    if (buildid === null) {
      return {
        latestBuildId: null,
        latestTimeUpdated: null,
        parseError: `ブランチ "${branch}" に buildid が含まれていませんでした`
      };
    }

    return {
      latestBuildId: buildid,
      latestTimeUpdated: Number.isFinite(timeupdated) ? timeupdated : null,
      parseError: null
    };
  }

  private recordResult(result: SteamUpdateCheckResult): void {
    this.lastResult = result;
    this.emit('result', result);
  }
}
