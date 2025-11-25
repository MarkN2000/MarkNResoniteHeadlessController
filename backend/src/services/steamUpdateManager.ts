import { spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import path from 'node:path';
import type { SteamConfig } from './steamConfig.js';

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
 * SteamCMD を使って Resonite をアップデートするためのシンプルなマネージャー
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

      // タイムアウト（Steam Guardコード待ちなどで止まり続けるのを防ぐ）
      const TIMEOUT_MS = Number(process.env.STEAMCMD_TIMEOUT_MS || 5 * 60 * 1000); // デフォルト5分
      let timeout: NodeJS.Timeout | null = null;

      // カレントディレクトリを SteamCMD 実行ファイルのディレクトリに設定
      const cwd = path.dirname(steamcmdPath);

      this.emit('log', `[SteamUpdate] Starting SteamCMD: ${steamcmdPath}`);
      this.emit('log', `[SteamUpdate] Using account: ${username} (passwordはログに出力しません)`);

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

        const rawLog = stdout + stderr;
        const baseMessage =
          'SteamCMDの実行がタイムアウトしました。ログインやSteam Guardコード待ちで止まっている可能性があります。';

        // guardCode 未指定でガードプロンプトが見えている場合は、ガードコード要求とみなす
        if (!guardCode && steamGuardPrompted) {
          resolve({
            success: false,
            updated: false,
            message: baseMessage,
            rawLog,
            requiresGuardCode: true
          });
        } else {
          resolve({
            success: false,
            updated: false,
            message: baseMessage,
            rawLog
          });
        }
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
        }
      });

      child.stderr.on('data', (data: Buffer) => {
        const text = data.toString('utf-8');
        stderr += text;
        this.emit('log', text);
      });

      child.on('error', (error) => {
        if (timeout) {
          clearTimeout(timeout);
          timeout = null;
        }
        this.emit('log', `[SteamUpdate] Failed to start SteamCMD: ${String(error)}`);
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
        const rawLog = stdout + stderr;

        if (code !== 0) {
          this.emit('log', `[SteamUpdate] SteamCMD exited with code ${code}`);

          const baseMessage = `SteamCMDの実行中にエラーが発生しました（終了コード: ${code}）。ログを確認してください。`;

          // guardCode 未指定で Steam Guard プロンプトが確認された場合のみ、
          // フロントエンドからの再試行を促すフラグを立てる
          if (!guardCode && steamGuardPrompted) {
            resolve({
              success: false,
              updated: false,
              message:
                baseMessage +
                ' Steam Guardコードが必要と思われます。ブラウザ側でSteam Guardコードを入力して再試行してください。',
              rawLog,
              requiresGuardCode: true
            });
            return;
          }

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


