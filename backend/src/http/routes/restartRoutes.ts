import { Router } from 'express';
import type { Request, Response, NextFunction } from 'express';
import { RestartManager } from '../../services/restartManager.js';
import { loadRestartConfig, saveRestartConfig, getDefaultRestartConfig } from '../../services/restartConfig.js';
import type { RestartConfig } from '../../../../shared/src/index.js';

export function createRestartRoutes(restartManager: RestartManager): Router {
  const router = Router();

  /**
   * GET /api/restart/config
   * 再起動設定を取得
   */
  router.get('/config', async (req: Request, res: Response, next: NextFunction) => {
    try {
      const config = await loadRestartConfig();
      res.json(config);
    } catch (error) {
      console.error('[RestartRoutes] Failed to get config:', error);
      next(error);
    }
  });

  /**
   * POST /api/restart/config
   * 再起動設定を保存
   */
  router.post('/config', async (req: Request, res: Response, next: NextFunction) => {
    try {
      const config = req.body as RestartConfig;
      
      // バリデーションはrestartConfig.tsのsaveRestartConfigで実施
      await saveRestartConfig(config);
      
      // RestartManagerの設定も更新
      await restartManager.reloadConfig();
      
      // 次回の予定再起動を更新
      restartManager.updateNextScheduledRestart();
      
      res.json({ success: true, message: '設定を保存しました' });
    } catch (error: any) {
      console.error('[RestartRoutes] Failed to save config:', error);
      res.status(400).json({ 
        success: false, 
        error: error.message || '設定の保存に失敗しました' 
      });
    }
  });

  /**
   * GET /api/restart/status
   * 再起動ステータスを取得
   */
  router.get('/status', (req: Request, res: Response, next: NextFunction) => {
    try {
      const status = restartManager.getStatus();
      res.json(status);
    } catch (error) {
      console.error('[RestartRoutes] Failed to get status:', error);
      next(error);
    }
  });

  /**
   * POST /api/restart/trigger
   * 手動で再起動をトリガー
   */
  router.post('/trigger', async (req: Request, res: Response, next: NextFunction) => {
    try {
      const { type } = req.body;
      
      if (type !== 'manual' && type !== 'forced') {
        return res.status(400).json({ 
          success: false, 
          error: 'Invalid trigger type. Use "manual" or "forced".' 
        });
      }
      
      // 非同期で再起動を実行（レスポンスは即座に返す）
      restartManager.triggerRestart(type).catch((error) => {
        console.error('[RestartRoutes] Manual restart failed:', error);
      });
      
      res.json({ 
        success: true, 
        message: type === 'manual' ? '手動再起動を開始しました' : '強制再起動を開始しました' 
      });
    } catch (error: any) {
      console.error('[RestartRoutes] Failed to trigger restart:', error);
      res.status(500).json({ 
        success: false, 
        error: error.message || '再起動のトリガーに失敗しました' 
      });
    }
  });

  /**
   * POST /api/restart/config/reset
   * 再起動設定をデフォルトにリセット
   */
  router.post('/config/reset', async (req: Request, res: Response, next: NextFunction) => {
    try {
      // 現在の設定を読み込んでスケジュールを保持
      const currentConfig = await loadRestartConfig();
      const defaultConfig = getDefaultRestartConfig();
      
      // スケジュールを保持（デフォルト設定のschedulesを現在のものに置き換え）
      defaultConfig.triggers.scheduled.schedules = currentConfig.triggers.scheduled.schedules;
      
      // デフォルト設定を保存
      await saveRestartConfig(defaultConfig);
      
      // RestartManagerの設定も更新
      await restartManager.reloadConfig();
      
      // 次回の予定再起動を更新
      restartManager.updateNextScheduledRestart();
      
      res.json({ 
        success: true, 
        message: '設定をデフォルトにリセットしました',
        config: defaultConfig
      });
    } catch (error: any) {
      console.error('[RestartRoutes] Failed to reset config:', error);
      res.status(500).json({ 
        success: false, 
        error: error.message || '設定のリセットに失敗しました' 
      });
    }
  });

  return router;
}

