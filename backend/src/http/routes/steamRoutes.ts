import { Router } from 'express';
import type { Request, Response } from 'express';
import { loadSteamConfig, saveSteamConfig, maskSensitiveSteamConfig } from '../../services/steamConfig.js';
import { SteamUpdateManager } from '../../services/steamUpdateManager.js';
import type { SteamUpdateProgress, SteamUpdateState } from '../../services/steamUpdateManager.js';
import { steamUpdateBus } from '../../services/steamUpdateBus.js';
import type { ProcessManager } from '../../services/processManager.js';

export function createSteamRoutes(processManager: ProcessManager): Router {
  const steamRoutes = Router();

  // 同時に複数のアップデートが走らないようにするためのプロセス内ロック
  // （複数の管理者が同時にボタンを押した場合のSteamCMDロック衝突や、
  //   フロント側の進捗表示混線を防ぐ）
  let updateInProgress = false;

  /**
   * GET /api/steam/config
   * Steam設定を取得（パスワードは含めない）
   */
  steamRoutes.get('/config', async (_req: Request, res: Response) => {
    try {
      const config = await loadSteamConfig();
      const masked = maskSensitiveSteamConfig(config);
      res.json(masked);
    } catch (error: any) {
      console.error('[SteamRoutes] Failed to get config:', error);
      res.status(500).json({
        success: false,
        error: error?.message || 'Steam設定の取得に失敗しました'
      });
    }
  });

  /**
   * POST /api/steam/config
   * Steam設定を保存（パスワード以外）
   */
  steamRoutes.post('/config', async (req: Request, res: Response) => {
    try {
      const incoming = req.body;
      const current = await loadSteamConfig();

      // パスワードはこのエンドポイントでは更新しない
      const nextConfig = {
        steamCmd: {
          path: incoming.steamCmd?.path ?? current.steamCmd.path,
          autoDetect:
            typeof incoming.steamCmd?.autoDetect === 'boolean'
              ? incoming.steamCmd.autoDetect
              : current.steamCmd.autoDetect
        },
        resonite: {
          appId: incoming.resonite?.appId ?? current.resonite.appId,
          installDir: incoming.resonite?.installDir ?? current.resonite.installDir,
          autoDetectFromExecutable:
            typeof incoming.resonite?.autoDetectFromExecutable === 'boolean'
              ? incoming.resonite.autoDetectFromExecutable
              : current.resonite.autoDetectFromExecutable
        },
        account: {
          username: incoming.account?.username ?? current.account.username,
          password: current.account.password,
          useSteamGuardFile:
            typeof incoming.account?.useSteamGuardFile === 'boolean'
              ? incoming.account.useSteamGuardFile
              : current.account.useSteamGuardFile,
          steamGuardFile: incoming.account?.steamGuardFile ?? current.account.steamGuardFile
        }
      };

      await saveSteamConfig(nextConfig);
      res.json({ success: true, message: 'Steam設定を保存しました' });
    } catch (error: any) {
      console.error('[SteamRoutes] Failed to save config:', error);
      res.status(500).json({
        success: false,
        error: error?.message || 'Steam設定の保存に失敗しました'
      });
    }
  });

  /**
   * POST /api/steam/config/password
   * Steamアカウントのパスワードのみを更新
   */
  steamRoutes.post('/config/password', async (req: Request, res: Response) => {
    try {
      const { password } = req.body as { password?: string };

      if (typeof password !== 'string') {
        return res.status(400).json({
          success: false,
          error: 'password は文字列で指定してください'
        });
      }

      const current = await loadSteamConfig();
      const nextConfig = {
        ...current,
        account: {
          ...current.account,
          password
        }
      };

      await saveSteamConfig(nextConfig);
      res.json({ success: true, message: 'Steamパスワードを更新しました' });
    } catch (error: any) {
      console.error('[SteamRoutes] Failed to update password:', error);
      res.status(500).json({
        success: false,
        error: error?.message || 'Steamパスワードの更新に失敗しました'
      });
    }
  });

  /**
   * POST /api/steam/update
   * SteamCMD を使って Resonite をアップデート
   * body: { guardCode?: string }
   */
  steamRoutes.post('/update', async (req: Request, res: Response) => {
    // 同時実行ガード: 既に走っているアップデートがあれば 409 Conflict を返す
    if (updateInProgress) {
      return res.status(409).json({
        success: false,
        updated: false,
        conflict: true,
        error: '別のリクエストでResoniteアップデートが既に実行中です。完了までお待ちください。'
      });
    }
    updateInProgress = true;

    try {
      const { guardCode } = (req.body ?? {}) as { guardCode?: string };

      const config = await loadSteamConfig();
      const updater = new SteamUpdateManager(config);

      // updater のイベントを WebSocket バスへ転送
      // （フロント側はバスを購読してリアルタイムに進捗を表示する）
      const onLog = (message: string): void => {
        console.log(message);
        steamUpdateBus.emitLog(message);
      };
      const onStatus = (state: SteamUpdateState): void => {
        steamUpdateBus.emitStatus(state);
      };
      const onProgress = (progress: SteamUpdateProgress): void => {
        steamUpdateBus.emitProgress(progress);
      };
      updater.on('log', onLog);
      updater.on('status', onStatus);
      updater.on('progress', onProgress);

      try {
        // サーバーが起動中の場合は停止する（ファイルロック回避）
        const status = processManager.getStatus();
        const wasRunning = status.running;
        const configPath = status.configPath;

        if (wasRunning) {
          console.log('[SteamRoutes] Resoniteヘッドレスサーバーを停止しています...');
          await processManager.stop();
        }

        const result = await updater.updateResonite(guardCode);

        // サーバーを停止した場合は、アップデート結果に関わらず再起動を試みる
        // （Guardコード要求で失敗した場合もサーバーは復帰させる）
        if (wasRunning && !result.requiresGuardCode) {
          try {
            console.log('[SteamRoutes] Resoniteヘッドレスサーバーを再起動しています...');
            await processManager.start(configPath);
          } catch (restartError: any) {
            console.error('[SteamRoutes] サーバーの再起動に失敗:', restartError);
            result.message += '（注意: サーバーの自動再起動に失敗しました。手動で起動してください）';
          }
        }

        if (!result.success) {
          // Steam Guardコードが必要な場合は、2ステップ目の入力を促すためのフラグを返す
          if (result.requiresGuardCode) {
            return res.json({
              success: false,
              updated: false,
              requiresGuardCode: true,
              message: result.message
            });
          }

          return res.status(500).json({
            success: false,
            updated: false,
            error: result.message
          });
        }

        res.json({
          success: true,
          updated: result.updated,
          message: result.message
        });
      } finally {
        // メモリリーク防止: 登録した3つのリスナーを必ず解除
        updater.off('log', onLog);
        updater.off('status', onStatus);
        updater.off('progress', onProgress);
      }
    } catch (error: any) {
      console.error('[SteamRoutes] Failed to update Resonite:', error);
      res.status(500).json({
        success: false,
        updated: false,
        error: error?.message || 'Resoniteのアップデートに失敗しました'
      });
    } finally {
      // 同時実行ロックを必ず解除
      updateInProgress = false;
    }
  });

  return steamRoutes;
}
