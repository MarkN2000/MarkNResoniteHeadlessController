import { spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import path from 'node:path';
import type { SteamConfig } from './steamConfig.js';
import type { SteamUpdateState, SteamUpdateProgress } from '../../../shared/src/index.js';

// 既存の import 経路を維持するため、shared から取り込んだ型を再エクスポート
// （後方互換: 既に `from './steamUpdateManager.js'` で参照しているコードを壊さない）
export type { SteamUpdateState, SteamUpdateProgress };

export interface SteamUpdateResult {
  success: boolean;
  updated: boolean;
  message: string;
  rawLog?: string;
  /**
   * Steam Guardコードが必要と判断された場合に true
   * （初回ガード無し試行時のみ想定）
   */
  requiresGuardCode?: boolean;
}

/**
 * SteamCMD出力1行から進捗を抽出する純粋関数
 *
 * 代表的な出力例:
 *   Update state (0x61) downloading, progress: 12.34 (1234567 / 9999999)
 *   Update state (0x81) verifying update, progress: 50.00 (12345 / 24690)
 */
const PROGRESS_RE =
  /Update state \(0x[0-9a-f]+\)\s+([^,]+),\s+progress:\s+([\d.]+)\s*(?:\((\d+)\s*\/\s*(\d+)\))?/i;

export const parseSteamcmdLine = (line: string): SteamUpdateProgress | null => {
  const match = line.match(PROGRESS_RE);
  if (!match) return null;

  const stateRaw = (match[1] ?? '').toLowerCase().trim();
  const percent = Number.parseFloat(match[2] ?? '0');
  const downloaded = match[3] ? Number.parseInt(match[3], 10) : undefined;
  const total = match[4] ? Number.parseInt(match[4], 10) : undefined;

  let state: SteamUpdateState = 'downloading';
  if (stateRaw.includes('verifying') || stateRaw.includes('validating')) {
    state = 'verifying';
  } else if (stateRaw.includes('preallocating') || stateRaw.includes('committing')) {
    state = 'finalizing';
  } else if (stateRaw.includes('downloading')) {
    state = 'downloading';
  }

  return {
    percent: Number.isFinite(percent) ? percent : 0,
    state,
    downloaded,
    total,
    raw: line.trim()
  };
};

/**
 * SteamCMDの汎用ログ行から状態キーワードを検出する純粋関数
 * progress 行以外の状態遷移検出に使用
 */
export const detectStateFromLog = (line: string): SteamUpdateState | null => {
  const lower = line.toLowerCase();
  // 認証フェーズ
  if (lower.includes('logging in') || lower.includes('waiting for user info')) {
    return 'authenticating';
  }
  // 接続フェーズ
  if (lower.includes('connecting anonymously') || lower.includes("connecting to steam")) {
    return 'connecting';
  }
  // 検証/ダウンロードはprogress行で更新するためここでは扱わない
  return null;
};

/**
 * SteamCMD を使って Resonite をアップデートするためのシンプルなマネージャー
 *
 * 発火するイベント:
 *   'log'      (text: string)                stdout/stderr の各チャンク
 *   'status'   (state: SteamUpdateState)     状態遷移（差分のみ）
 *   'progress' (progress: SteamUpdateProgress) 進捗パーセント
 */
export class SteamUpdateManager extends EventEmitter {
  private config: SteamConfig;

  constructor(config: SteamConfig) {
    super();
    this.config = config;
  }

  /**
   * Resonite をアップデート（アップデートがなければ何もせずメッセージのみ返す）
   *
   * 実装を簡潔にするため、現時点では SteamCMD の出力メッセージから
   * 「already up to date」かどうかを判定する。
   */
  async updateResonite(guardCode?: string): Promise<SteamUpdateResult> {
    const steamcmdPath = this.config.steamCmd.path;
    const { username, password, useSteamGuardFile, steamGuardFile } = this.config.account;
    const appId = this.config.resonite.appId || '2519830';

    if (!steamcmdPath) {
      return {
        success: false,
        updated: false,
        message: 'SteamCMDのパスが設定されていません（config/steam.json または STEAMCMD_PATH を確認してください）'
      };
    }

    if (!username || !password) {
      return {
        success: false,
        updated: false,
        message: 'Steamアカウントのユーザー名またはパスワードが設定されていません（config/steam.json または STEAM_USERNAME/STEAM_PASSWORD を確認してください）'
      };
    }

    const installDir = this.config.resonite.installDir;
    if (!installDir) {
      return {
        success: false,
        updated: false,
        message: 'Resoniteのインストールディレクトリが設定されていません（config/steam.json の resonite.installDir を確認してください）'
      };
    }

    const args: string[] = ['+login', username, password];

    // 2ステップ方式の2回目: フロントエンドから guardCode が渡された場合
    // steamcmd の +login ヘルプに従い、第3引数としてガードコードを渡す
    if (guardCode) {
      args.push(guardCode);
    }

    // Steam Guardファイルによる2要素認証に対応
    if (useSteamGuardFile && steamGuardFile) {
      args.push('+set_steam_guard_file', steamGuardFile);
    }

    // インストール先ディレクトリを明示的に指定（SteamCMDデフォルトではなく設定されたパスへ）
    args.push('+force_install_dir', installDir);

    args.push(
      '+app_update',
      appId,
      'validate',
      '+quit'
    );

    return new Promise<SteamUpdateResult>((resolve) => {
      let stdout = '';
      let stderr = '';
      let steamGuardPrompted = false;
      let lastState: SteamUpdateState | null = null;

      // 状態遷移を差分のみ emit するヘルパー
      const emitState = (state: SteamUpdateState): void => {
        if (state === lastState) return;
        lastState = state;
        this.emit('status', state);
      };

      // 開始状態を即時通知
      emitState('starting');

      // タイムアウト（Steam Guardコード待ちなどで止まり続けるのを防ぐ）
      // デフォルト5分（環境変数 STEAMCMD_TIMEOUT_MS で上書き可能）
      const TIMEOUT_MS = Number(process.env.STEAMCMD_TIMEOUT_MS || 5 * 60 * 1000);
      let timeout: NodeJS.Timeout | null = null;

      // カレントディレクトリを SteamCMD 実行ファイルのディレクトリに設定
      const cwd = path.dirname(steamcmdPath);

      this.emit('log', `[SteamUpdate] Starting SteamCMD: ${steamcmdPath}`);
      this.emit('log', `[SteamUpdate] Using account: ${username} (passwordはログに出力しません)`);

      // 行末バッファ（stdout/stderr 個別）
      // SteamCMD は data チャンクが行の途中で切れることがあるため、
      // 改行を含むまでバッファして完全な行だけを processLine に渡す
      const stdoutBufferRef = { value: '' };
      const stderrBufferRef = { value: '' };

      // 1行から状態と進捗を抽出して emit する純粋ヘルパー
      const processLine = (line: string): void => {
        if (!line) return;

        // 進捗パース（成功した場合は対応する状態にも遷移）
        const progress = parseSteamcmdLine(line);
        if (progress) {
          emitState(progress.state);
          this.emit('progress', progress);
          return;
        }

        // それ以外の状態キーワード検出
        const detected = detectStateFromLog(line);
        if (detected) {
          emitState(detected);
        }
      };

      // チャンクをバッファに蓄積し、改行で区切られた完全な行のみを処理
      const processChunk = (text: string, bufferRef: { value: string }): void => {
        bufferRef.value += text;
        const parts = bufferRef.value.split(/\r?\n/);
        // 末尾の不完全な行（改行が来ていない）はバッファに残す
        bufferRef.value = parts.pop() ?? '';
        for (const line of parts) {
          processLine(line);
        }
      };

      // プロセス終了時にバッファに残った最終行を処理する
      const flushBuffers = (): void => {
        if (stdoutBufferRef.value) {
          processLine(stdoutBufferRef.value);
          stdoutBufferRef.value = '';
        }
        if (stderrBufferRef.value) {
          processLine(stderrBufferRef.value);
          stderrBufferRef.value = '';
        }
      };

      const child = spawn(steamcmdPath, args, {
        cwd,
        windowsHide: true
      });

      timeout = setTimeout(() => {
        this.emit('log', `[SteamUpdate] Timeout after ${TIMEOUT_MS}ms. Killing SteamCMD process...`);
        try {
          child.kill();
        } catch {
          // ignore
        }

        flushBuffers();
        emitState('timeout');

        const rawLog = stdout + stderr;
        const baseMessage =
          'SteamCMDの実行がタイムアウトしました。ネットワーク接続やSteamサーバーの状態を確認してください。';

        // stdout で Steam Guard プロンプトを検知した場合のみ、ガードコード入力を促す
        resolve({
          success: false,
          updated: false,
          message: baseMessage,
          rawLog,
          requiresGuardCode: steamGuardPrompted && !guardCode
        });
      }, TIMEOUT_MS);

      child.stdout.on('data', (data: Buffer) => {
        const text = data.toString('utf-8');
        stdout += text;
        this.emit('log', text);

        const lower = text.toLowerCase();
        if (
          lower.includes('steam guard') ||
          lower.includes('two-factor code') ||
          lower.includes('two factor code')
        ) {
          steamGuardPrompted = true;
          emitState('guard-required');
        }

        processChunk(text, stdoutBufferRef);
      });

      child.stderr.on('data', (data: Buffer) => {
        const text = data.toString('utf-8');
        stderr += text;
        this.emit('log', text);
        processChunk(text, stderrBufferRef);
      });

      child.on('error', (error) => {
        if (timeout) {
          clearTimeout(timeout);
          timeout = null;
        }
        flushBuffers();
        this.emit('log', `[SteamUpdate] Failed to start SteamCMD: ${String(error)}`);
        emitState('failed');
        resolve({
          success: false,
          updated: false,
          message: `SteamCMDの起動に失敗しました: ${error instanceof Error ? error.message : String(error)}`,
          rawLog: stdout + stderr
        });
      });

      child.on('close', (code) => {
        if (timeout) {
          clearTimeout(timeout);
          timeout = null;
        }
        flushBuffers();
        const rawLog = stdout + stderr;

        if (code !== 0) {
          this.emit('log', `[SteamUpdate] SteamCMD exited with code ${code}`);

          const baseMessage = `SteamCMDの実行中にエラーが発生しました（終了コード: ${code}）。ログを確認してください。`;

          // stdout で Steam Guard プロンプトを検知した場合のみ、ガードコード入力を促す
          if (steamGuardPrompted && !guardCode) {
            emitState('guard-required');
            resolve({
              success: false,
              updated: false,
              message:
                baseMessage +
                ' Steam Guardコードが必要です。ブラウザ側でSteam Guardコードを入力して再試行してください。',
              rawLog,
              requiresGuardCode: true
            });
            return;
          }

          emitState('failed');
          resolve({
            success: false,
            updated: false,
            message: baseMessage,
            rawLog
          });
          return;
        }

        // 代表的なメッセージでアップデート有無を判定
        const lowerLog = rawLog.toLowerCase();
        const alreadyUpToDate =
          lowerLog.includes('already up to date') ||
          lowerLog.includes('fully installed, but no update was necessary');

        // アップデートが実際に走った場合のみ進捗100%を発火する。
        // 既に最新の場合は途中の進捗が一切流れていないため、
        // ここで100%にジャンプさせるとUIに「DLしたかのような誤解」を与える。
        if (!alreadyUpToDate) {
          this.emit('progress', { percent: 100, state: 'completed' });
        }
        emitState('completed');

        if (alreadyUpToDate) {
          resolve({
            success: true,
            updated: false,
            message: 'Resonite は既に最新バージョンです（アップデートはありませんでした）。',
            rawLog
          });
        } else {
          resolve({
            success: true,
            updated: true,
            message: 'Resonite のアップデートが完了しました。',
            rawLog
          });
        }
      });
    });
  }
}


