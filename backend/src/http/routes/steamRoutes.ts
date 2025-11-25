import { Router } from 'express';
import type { Request, Response } from 'express';
import { loadSteamConfig, saveSteamConfig, maskSensitiveSteamConfig } from '../../services/steamConfig.js';
import { SteamUpdateManager } from '../../services/steamUpdateManager.js';

export const steamRoutes = Router();

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
 */
steamRoutes.post('/update', async (_req: Request, res: Response) => {
  try {
    const config = await loadSteamConfig();
    const updater = new SteamUpdateManager(config);

    // 必要であればログをどこかに流すためのイベント
    updater.on('log', (message: string) => {
      console.log(message);
    });

    const result = await updater.updateResonite();

    if (!result.success) {
      return res.status(500).json({
        success: false,
        updated: false,
        message: result.message
      });
    }

    res.json({
      success: true,
      updated: result.updated,
      message: result.message
    });
  } catch (error: any) {
    console.error('[SteamRoutes] Failed to update Resonite:', error);
    res.status(500).json({
      success: false,
      updated: false,
      message: error?.message || 'Resoniteのアップデートに失敗しました'
    });
  }
});


