import { promises as fs } from 'node:fs';
import path from 'node:path';
import { PROJECT_ROOT } from '../config/index.js';

const CONFIG_DIR = path.join(PROJECT_ROOT, 'config');
const STEAM_CONFIG_FILE = path.join(CONFIG_DIR, 'steam.json');

export interface SteamAccountConfig {
  username: string;
  password: string;
  useSteamGuardFile: boolean;
  steamGuardFile: string;
}

export interface SteamCmdConfig {
  path: string;
  autoDetect: boolean;
}

export interface ResoniteSteamConfig {
  appId: string;
  installDir: string;
  autoDetectFromExecutable: boolean;
}

export interface SteamConfig {
  steamCmd: SteamCmdConfig;
  resonite: ResoniteSteamConfig;
  account: SteamAccountConfig;
}

const DEFAULT_STEAM_CONFIG: SteamConfig = {
  steamCmd: {
    path: 'C:/steamcmd/steamcmd.exe',
    autoDetect: true
  },
  resonite: {
    appId: '2519830',
    installDir: 'C:/Program Files (x86)/Steam/steamapps/common/Resonite',
    autoDetectFromExecutable: true
  },
  account: {
    username: '',
    password: '',
    useSteamGuardFile: false,
    steamGuardFile: ''
  }
};

/**
 * Steam設定を読み込む（環境変数で一部を上書き）
 */
export async function loadSteamConfig(): Promise<SteamConfig> {
  try {
    const data = await fs.readFile(STEAM_CONFIG_FILE, 'utf-8');
    const fileConfig = JSON.parse(data) as SteamConfig;

    const autoDetect =
      fileConfig.steamCmd?.autoDetect ??
      DEFAULT_STEAM_CONFIG.steamCmd.autoDetect;

    // SteamCMDパスの自動検出（存在確認付き）
    const configuredPath =
      process.env.STEAMCMD_PATH ||
      fileConfig.steamCmd?.path ||
      DEFAULT_STEAM_CONFIG.steamCmd.path;

    let resolvedSteamCmdPath = configuredPath;

    if (autoDetect) {
      const candidates = [
        configuredPath,
        'C:/steamcmd/steamcmd.exe',
        'C:/Program Files (x86)/Steam/steamcmd/steamcmd.exe',
        'C:/Program Files/Steam/steamcmd/steamcmd.exe'
      ].filter((p): p is string => Boolean(p));

      for (const candidate of candidates) {
        try {
          await fs.access(candidate);
          resolvedSteamCmdPath = candidate;
          break;
        } catch {
          // 存在しない場合は次の候補を試す
        }
      }
    }

    const merged: SteamConfig = {
      steamCmd: {
        path: resolvedSteamCmdPath,
        autoDetect
      },
      resonite: {
        appId:
          fileConfig.resonite?.appId ||
          DEFAULT_STEAM_CONFIG.resonite.appId,
        installDir:
          fileConfig.resonite?.installDir ||
          DEFAULT_STEAM_CONFIG.resonite.installDir,
        autoDetectFromExecutable:
          fileConfig.resonite?.autoDetectFromExecutable ??
          DEFAULT_STEAM_CONFIG.resonite.autoDetectFromExecutable
      },
      account: {
        username:
          process.env.STEAM_USERNAME ||
          fileConfig.account?.username ||
          DEFAULT_STEAM_CONFIG.account.username,
        password:
          process.env.STEAM_PASSWORD ||
          fileConfig.account?.password ||
          DEFAULT_STEAM_CONFIG.account.password,
        useSteamGuardFile:
          fileConfig.account?.useSteamGuardFile ??
          DEFAULT_STEAM_CONFIG.account.useSteamGuardFile,
        steamGuardFile:
          fileConfig.account?.steamGuardFile ||
          DEFAULT_STEAM_CONFIG.account.steamGuardFile
      }
    };

    return merged;
  } catch (error: any) {
    if (error.code === 'ENOENT') {
      // ファイルが存在しない場合はデフォルト設定を作成
      await saveSteamConfig(DEFAULT_STEAM_CONFIG);
      return DEFAULT_STEAM_CONFIG;
    }

    console.error('[SteamConfig] Failed to load config:', error);
    // エラー時はデフォルト設定を返す
    return DEFAULT_STEAM_CONFIG;
  }
}

/**
 * Steam設定を保存する（環境変数で上書きされる項目も含めてそのまま保存）
 */
export async function saveSteamConfig(config: SteamConfig): Promise<void> {
  try {
    await fs.mkdir(CONFIG_DIR, { recursive: true });

    // JSON形式で保存（整形付き）
    await fs.writeFile(
      STEAM_CONFIG_FILE,
      JSON.stringify(config, null, 2),
      'utf-8'
    );

    console.log('[SteamConfig] Config saved successfully');
  } catch (error) {
    console.error('[SteamConfig] Failed to save config:', error);
    throw new Error('Steam設定の保存に失敗しました');
  }
}

/**
 * APIレスポンス用にパスワードをマスクした設定を返す
 */
export function maskSensitiveSteamConfig(config: SteamConfig): Omit<SteamConfig, 'account'> & {
  account: Omit<SteamAccountConfig, 'password'> & { hasPassword: boolean };
} {
  return {
    steamCmd: config.steamCmd,
    resonite: config.resonite,
    account: {
      username: config.account.username,
      useSteamGuardFile: config.account.useSteamGuardFile,
      steamGuardFile: config.account.steamGuardFile,
      hasPassword: Boolean(config.account.password)
    }
  };
}


