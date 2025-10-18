<script lang="ts">
  import { onMount, afterUpdate } from 'svelte';
  import { derived, writable } from 'svelte/store';
  import {
    createServerStores,
    getStatus,
    getLogs,
    getConfigs,
    generateConfig,
    getRuntimeStatus,
    getRuntimeUsers,
    getFriendRequests,
    getBans,
    getRuntimeWorlds,
    postCommand,
    postFocusWorld,
    postFocusWorldRefresh,
    startServer,
    stopServer,
    getWorldSearch,
    login,
    verifyAuth,
    logout,
    setAuthToken,
    getAuthToken,
    getSecurityConfig,
    getClientInfo,
    getResoniteUserFull,
    searchResoniteUsers,
    getRestartConfig,
    saveRestartConfig,
    getRestartStatus,
    triggerRestart,
    resetRestartConfig,
    type WorldSearchItem,
    type RuntimeStatusData,
    type RuntimeUsersData,
    type FriendRequestsData,
    type BansData,
    type BanEntry,
    type RuntimeWorldsData,
    type RuntimeWorldEntry,
    type ConfigEntry,
    type LogEntry,
    type ResoniteUserFull,
    type RestartConfig,
    type RestartStatus
  } from '$lib';

  const { status, logs, configs, metrics, setConfigs, setStatus, setLogs, clearLogs } = createServerStores();

  // Resonite画像URLを変換する関数
  const convertResoniteImageUrl = (url: string | null): string | null => {
    if (!url) return null;
    
    // resdb:// プロトコルをHTTPSに変換
    if (url.startsWith('resdb:///')) {
      // resdb:///を除去してIDを取得
      let id = url.replace('resdb:///', '');
      
      // 拡張子を除去（.webp, .png, .jpg など）
      id = id.replace(/\.(webp|png|jpg|jpeg|gif)$/i, '');
      
      // Resoniteのアセットサーバーに変換
      return `https://assets.resonite.com/${id}`;
    }
    
    // 既にHTTPまたはHTTPSの場合はそのまま返す
    if (url.startsWith('http://') || url.startsWith('https://')) {
      return url;
    }
    
    return null;
  };

  const tabs = [
    { id: 'dashboard', label: 'ダッシュボード' },
    { id: 'newWorld', label: '新規セッション' },
    { id: 'friends', label: 'フレンド管理' },
    { id: 'settings', label: 'コンフィグ作成' },
    { id: 'restart', label: '自動再起動設定' }
  ];

  let activeTab: (typeof tabs)[number]['id'] = 'dashboard';

  // 認証状態
  let isAuthenticated = false;
  let loginPassword = '';
  let loginLoading = false;
  let showLogin = true; // 初期状態でログイン画面を表示

  // セキュリティ情報
  let clientInfo: any = null;
  let securityConfig: any = null;
  let rateLimitInfo: any = null;

  let initialLoading = true;
  let selectedConfig: string | undefined;
  let appMessage: { type: 'error' | 'warning' | 'info'; text: string } | null = null;
  const notificationsStore = writable<Array<{ id: number; message: string; type: 'success' | 'error' | 'info' }>>([]);
  const notifications = derived(notificationsStore, value => value);
  const pushToast = (message: string, type: 'success' | 'error' | 'info' = 'info', duration = 4200) => {
    const id = Date.now() + Math.random();
    notificationsStore.update(items => [...items, { id, message, type }]);
    setTimeout(() => {
      notificationsStore.update(items => items.filter((item: { id: number }) => item.id !== id));
    }, duration);
  };

  // 認証関数
  const handleLogin = async () => {
    if (!loginPassword.trim()) {
      pushToast('パスワードを入力してください', 'error');
      return;
    }

    loginLoading = true;
    try {
      const response = await login(loginPassword);
      setAuthToken(response.token);
      isAuthenticated = true;
      showLogin = false;
      loginPassword = '';
      pushToast('ログインしました', 'success');
      
      // 認証後に初期データを読み込み
      await loadInitialData();
      
      // セキュリティ情報を読み込み
      await loadSecurityInfo();
    } catch (error) {
      pushToast(error instanceof Error ? error.message : 'ログインに失敗しました', 'error');
    } finally {
      loginLoading = false;
    }
  };

  const handleLogout = async () => {
    try {
      await logout();
    } catch (error) {
      console.error('Logout error:', error);
    } finally {
      setAuthToken(null);
      isAuthenticated = false;
      pushToast('ログアウトしました', 'info');
    }
  };

  const checkAuth = async () => {
    // ブラウザ環境でのみ実行
    if (typeof window === 'undefined') {
      showLogin = true;
      return;
    }

    const token = getAuthToken();
    if (!token) {
      showLogin = true;
      return;
    }

    try {
      await verifyAuth();
      isAuthenticated = true;
      showLogin = false;
    } catch (error) {
      setAuthToken(null);
      showLogin = true;
    }
  };

  const loadSecurityInfo = async () => {
    try {
      const [clientInfoData, securityConfigData] = await Promise.all([
        getClientInfo(),
        getSecurityConfig()
      ]);
      clientInfo = clientInfoData;
      securityConfig = securityConfigData;
    } catch (error) {
      console.error('Failed to load security info:', error);
    }
  };

  const copyLogsToClipboard = async () => {
    try {
      const text = $logs.slice(-LOG_DISPLAY_LIMIT).map(entry => entry.message).join('\n');
      if (!text) {
        pushToast('コピーするログがありません', 'info');
        return;
      }
      if (typeof navigator !== 'undefined' && navigator.clipboard) {
        await navigator.clipboard.writeText(text);
        pushToast('ログをコピーしました', 'success');
      } else {
        const ta = document.createElement('textarea');
        ta.value = text;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        pushToast('ログをコピーしました', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コピーに失敗しました';
      pushToast(message, 'error');
    }
  };
  const copyToClipboard = async (text: string) => {
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard) {
        await navigator.clipboard.writeText(text);
        pushToast('コピーしました', 'success');
      } else {
        const ta = document.createElement('textarea');
        ta.value = text;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        pushToast('コピーしました', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コピーに失敗しました';
      pushToast(message, 'error');
    }
  };

  let actionInProgress = false;
  const LOG_DISPLAY_LIMIT = 1000;
  const INITIAL_RETRY_DELAY = 3000;
  let backendReachable = false;
  let logContainer: HTMLDivElement | null = null;
  let runtimeStatus: RuntimeStatusData | null = null;
  let runtimeUsers: RuntimeUsersData | null = null;
  let statusLoading = false;
  let usersLoading = false;
  let commandText = '';
  let commandResult = '';
  let commandLoading = false;
  let wasRunning = false;
  let runtimeWorlds: RuntimeWorldsData | null = null;
  let worldsLoading = false;
  let worldsError = '';
  let selectedWorldId: string | null = null;
  let currentWorld: RuntimeWorldEntry | null = null;
  let pendingStartup = false;
  let logsInitialized = false;
  let lastProcessedLogId = 0;
  let startupFinalizeTimer: ReturnType<typeof setTimeout> | null = null;
  let lastWorldRunningAt = 0;
  let showStartupMessage = false;
  let startupWorldsReady = false;
  let startupUsersReady = false;
  let headlessUserName: string | null = null;
  let headlessUserId: string | null = null;
  let startupRetryCount = 0;
  const STARTUP_RETRY_INTERVAL = 3800;
  const STARTUP_MAX_RETRIES = 4;

  const STORAGE_KEY = 'mrhc:selectedConfig';
  const DRAFT_STORAGE_KEY = 'mrhc:configDraftV1';
  const WORLD_STORAGE_KEY = 'mrhc:selectedWorldId';

  let templateName = 'Grid';
  let templateLoading = false;
  const templateSuggestions = ['Grid', 'Platform', 'Blank'];

  let worldUrl = '';
  let worldUrlLoading = false;

  let friendTargetName = '';
  let friendMessageText = '';
  let friendMessageLoading = false;
  let friendRequests: FriendRequestsData | null = null;
  let friendRequestsLoading = false;
  let friendRequestsError = '';

  // フレンド管理タブ - 新機能
  let friendSearchUsername = ''; // ユーザー名検索
  let friendSearchUserId = ''; // ユーザーID検索
  let friendSearchUsernameLoading = false;
  let friendSearchUserIdLoading = false;
  let friendSearchResults: ResoniteUserFull[] = []; // 検索結果とリスト表示用
  let selectedFriendUser: ResoniteUserFull | null = null; // 選択されたユーザー
  let friendSendLoading = false;
  let friendAcceptLoading = false;
  let friendRemoveLoading = false;
  let friendInviteLoading = false;
  
  // BAN一覧
  let bansList: BanEntry[] = [];
  let bansLoading = false;
  let selectedBanEntry: BanEntry | null = null;
  
  // フォーカスセッション内ユーザー一覧
  let focusedSessionUsersLoading = false;

  // World search state
  let worldSearchTerm = '';
  let worldSearchLoading = false;
  let worldSearchError = '';
  let worldSearchResults: WorldSearchItem[] = [];
  let selectedResoniteUrl: string | null = null;

  // Restart management state
  let restartConfig: RestartConfig | null = null;
  let restartStatus: RestartStatus | null = null;
  let restartConfigLoading = false;
  let restartStatusLoading = false;
  let restartSaveLoading = false;
  let forceRestartLoading = false;
  let manualRestartLoading = false;
  let restartConfigDebounceTimer: NodeJS.Timeout | null = null;
  let restartConfigInitialized = false; // 初回読み込み完了フラグ

  // Scheduled restart edit state
  let editingScheduleId: string | null = null;
  let editingSchedule: any = null;
  let scheduledRestartModalOpen = false;

  // Config generation state
  let configName = '';
  let configUsername = '';
  let configPassword = '';
  let showPassword = false;
  let configGenerationLoading = false;
  let isFormClearing = false; // フォームクリア中フラグ
  
  // Advanced config settings
  let configComment = '';
  // Universe ID は UI から削除（生成時はデフォルト固定）
  let configUniverseId = '';
  let configTickRate = 60.0;
  let configMaxConcurrentAssetTransfers = 128;
  let configUsernameOverride = '';
  let configDataFolder = '';
  let configCacheFolder = '';
  let configLogsFolder = '';
  let configAllowedUrlHosts = 'https://ttsapi.markn2000.com/';
  let configAutoSpawnItems = '';

  // リセット用デフォルト値（default.json 準拠）
  const DEFAULT_CONFIG = {
    name: 'default',
    username: '',
    password: '',
    comment: '',
    universeId: '',
    tickRate: 60.0,
    maxConcurrentAssetTransfers: 128,
    usernameOverride: '',
    dataFolder: '',
    cacheFolder: '',
    logsFolder: '',
    allowedUrlHosts: 'https://ttsapi.markn2000.com/',
    autoSpawnItems: ''
  } as const;

  const DEFAULT_SESSION_FIELDS = {
    sessionName: '',
    customSessionId: '',
    customSessionIdSuffix: '',
    description: '',
    tags: '',
    mobileFriendly: false,
    loadWorldURL: '',
    loadWorldPresetName: 'Grid',
    overrideCorrespondingWorldId: '',
    forcePort: null as number | null,
    keepOriginalRoles: false,
    defaultUserRoles: '',
    roleCloudVariable: '',
    allowUserCloudVariable: '',
    denyUserCloudVariable: '',
    requiredUserJoinCloudVariable: '',
    requiredUserJoinCloudVariableDenyMessage: '',
    awayKickMinutes: 5,
    parentSessionIds: '',
    autoInviteUsernames: '',
    autoInviteMessage: '',
    saveAsOwner: '',
    autoRecover: true,
    idleRestartInterval: 1800,
    forcedRestartInterval: -1.0,
    saveOnExit: false,
    autosaveInterval: -1.0,
    autoSleep: true,
    isEnabled: true,
    useCustomJoinVerifier: false,
    accessLevel: 'Anyone',
    maxUsers: 16,
    hideFromPublicListing: false,
    showWorldSearch: false,
    worldSearchTerm: '',
    worldSearchLoading: false,
    worldSearchError: '',
    worldSearchResults: [] as WorldSearchItem[]
  } as const;

  // リセットヘルパー
  const resetBasicField = (key: keyof typeof DEFAULT_CONFIG) => {
    if (key === 'name') configName = DEFAULT_CONFIG.name;
    if (key === 'username') configUsername = DEFAULT_CONFIG.username;
    if (key === 'password') configPassword = DEFAULT_CONFIG.password;
    if (key === 'comment') configComment = DEFAULT_CONFIG.comment;
    if (key === 'universeId') configUniverseId = DEFAULT_CONFIG.universeId;
    if (key === 'tickRate') configTickRate = DEFAULT_CONFIG.tickRate;
    if (key === 'maxConcurrentAssetTransfers') configMaxConcurrentAssetTransfers = DEFAULT_CONFIG.maxConcurrentAssetTransfers;
    if (key === 'usernameOverride') configUsernameOverride = DEFAULT_CONFIG.usernameOverride;
    if (key === 'dataFolder') configDataFolder = DEFAULT_CONFIG.dataFolder;
    if (key === 'cacheFolder') configCacheFolder = DEFAULT_CONFIG.cacheFolder;
    if (key === 'logsFolder') configLogsFolder = DEFAULT_CONFIG.logsFolder;
    if (key === 'allowedUrlHosts') configAllowedUrlHosts = DEFAULT_CONFIG.allowedUrlHosts;
    if (key === 'autoSpawnItems') configAutoSpawnItems = DEFAULT_CONFIG.autoSpawnItems;
  };

  const resetCurrentSessionField = (field: keyof typeof DEFAULT_SESSION_FIELDS | 'customSessionIdSuffix') => {
    const cur = getCurrentSession();
    if (field === 'customSessionIdSuffix') {
      (cur as any).customSessionIdSuffix = '';
    } else if (field === 'customSessionId') {
      // カスタムセッションIDのリセット時はプレフィックスとサフィックス両方をリセット
      (cur as any).customSessionIdPrefix = '';
      (cur as any).customSessionIdSuffix = '';
    } else {
      (cur as any)[field] = (DEFAULT_SESSION_FIELDS as any)[field];
    }
    // sessions 配列を更新して再描画
    sessions = sessions.map(s => (s.id === cur.id ? cur : s));
  };

  // usernameからuseridを取得するAPI関数（デバウンス・キャッシュ機能付き）
  const fetchUseridFromUsername = async (username: string): Promise<string | null> => {
    const trimmedUsername = username.trim();
    if (!trimmedUsername) return null;
    
    // キャッシュをチェック
    if (useridCache.has(trimmedUsername)) {
      return useridCache.get(trimmedUsername)!;
    }
    
    // 同じユーザー名の場合は重複リクエストを避ける
    if (lastFetchedUsername === trimmedUsername && useridLoading) {
      return null;
    }
    
    try {
      useridLoading = true;
      lastFetchedUsername = trimmedUsername;
      
      // バックエンド経由でAPIを呼び出し（CORS問題を回避）
      const response = await fetch(`/api/server/resonite-user/${encodeURIComponent(trimmedUsername)}`);
      
      if (!response.ok) {
        let errorMessage = `HTTP ${response.status}`;
        try {
          const errorData = await response.json();
          errorMessage = errorData.error || errorMessage;
          if (errorData.details) {
            errorMessage += ` (詳細: ${errorData.details})`;
          }
        } catch {
          errorMessage += ' (レスポンスの解析に失敗)';
        }
        throw new Error(errorMessage);
      }
      
      const data = await response.json();
      
      if (!data.userid) {
        throw new Error('ユーザーIDが取得できませんでした（APIレスポンスにuseridが含まれていません）');
      }
      
      // キャッシュに保存
      useridCache.set(trimmedUsername, data.userid);
      
      return data.userid;
      
    } catch (error) {
      const message = error instanceof Error ? error.message : 'ユーザーIDの取得に失敗しました';
      pushToast(`ユーザーID取得に失敗: ${message}`, 'error');
      return null;
    } finally {
      useridLoading = false;
    }
  };

  // 手動でUserIDを取得してプレフィックスに設定
  const autoFillUserid = async () => {
    const username = configUsername.trim();
    
    if (!username) {
      pushToast('ユーザー名を入力してください', 'error');
      return;
    }
    
    if (useridLoading) {
      return; // 既に実行中の場合は何もしない
    }
    
    try {
      // キャッシュをチェック
      if (useridCache.has(username)) {
        const userid = useridCache.get(username)!;
        const cur = getCurrentSession();
        (cur as any).customSessionIdPrefix = userid;
        sessions = sessions.map(s => (s.id === cur.id ? cur : s));
        pushToast('キャッシュから自動入力しました', 'success');
        return;
      }
      
      // lastFetchedUsernameをリセットして常に実行できるようにする
      lastFetchedUsername = '';
      
      const userid = await fetchUseridFromUsername(username);
      
      if (userid) {
        // 現在のセッションのプレフィックスに設定
        const cur = getCurrentSession();
        (cur as any).customSessionIdPrefix = userid;
        sessions = sessions.map(s => (s.id === cur.id ? cur : s));
        
        pushToast('ユーザーIDを自動入力しました', 'success');
      }
    } catch (error) {
      // 予期しないエラーが発生した場合
      const message = error instanceof Error ? error.message : '不明なエラーが発生しました';
      pushToast(`予期しないエラー: ${message}`, 'error');
    }
  };

  // カスタムセッションIDの接頭辞
  const getCustomIdPrefix = () => {
    // カスタムプレフィックスが設定されている場合はそれを使用、そうでなければデフォルト
    if (customSessionIdPrefix.trim()) {
      return customSessionIdPrefix.trim();
    }
    return headlessUserId ? `S-${headlessUserId}:` : 'S-<UserID>:';
  };
  
  // Session management (default.json と同値になるよう初期値設定)
  let sessions: any[] = [
    {
      id: 1,
      isEnabled: true,
      sessionName: '',
      customSessionId: '',
      customSessionIdPrefix: '',
      customSessionIdSuffix: '',
      description: '',
      maxUsers: 16,
      accessLevel: 'Anyone',
      useCustomJoinVerifier: false,
      hideFromPublicListing: null,
      tags: '',
      mobileFriendly: false,
      loadWorldURL: '',
      loadWorldPresetName: 'Grid',
      overrideCorrespondingWorldId: '',
      forcePort: null,
      keepOriginalRoles: false,
      defaultUserRoles: '',
      roleCloudVariable: '',
      allowUserCloudVariable: '',
      denyUserCloudVariable: '',
      requiredUserJoinCloudVariable: '',
      requiredUserJoinCloudVariableDenyMessage: '',
      awayKickMinutes: null,
      parentSessionIds: '',
      autoInviteUsernames: '',
      autoInviteMessage: '',
      saveAsOwner: '',
      autoRecover: true,
      idleRestartInterval: 1800,
      forcedRestartInterval: -1.0,
      saveOnExit: false,
      autosaveInterval: -1.0,
      autoSleep: true,
      // ワールド検索関連のプロパティを追加
      showWorldSearch: false,
      worldSearchTerm: '',
      worldSearchLoading: false,
      worldSearchError: '',
      worldSearchResults: []
    }
  ];
  let activeSessionTab = 1;
  let nextSessionId = 2;
  let showConfigPreview = false;
  let configPreviewText = '';
  
  // コンフィグ読み込み用の変数
  let selectedConfigToLoad = '';
  let configLoadLoading = false;
  let configDeleteLoading = false;
  
  // プレビュー編集用の変数
  let isPreviewEditing = false;
  let editedPreviewText = '';
  let previewEditError = '';
  let previewTextarea: HTMLTextAreaElement | null = null;
  
  // カスタムセッションIDプレフィックス用の変数
  let customSessionIdPrefix = '';
  
  // ユーザーID取得用の変数
  let useridLoading = false;
  let lastFetchedUsername = '';
  let useridCache = new Map<string, string>();

  // アイテムスポーン機能
  let itemSpawnUrl = '';
  let itemSpawnLoading = false;

  // ダイナミックインパルスstring機能
  let dynamicImpulseTag = '';
  let dynamicImpulseText = '';
  let dynamicImpulseLoading = false;
  
  // 下書き保存/復元: コンフィグ作成タブの編集中データをLocalStorageに自動保存
  const loadDraft = () => {
    try {
      const raw = localStorage.getItem(DRAFT_STORAGE_KEY);
      if (!raw) return false;
      const draft = JSON.parse(raw);
      
      // 期限切れチェック（30分）
      if (draft._expires && Date.now() > draft._expires) {
        localStorage.removeItem(DRAFT_STORAGE_KEY);
        return false;
      }
      
      configName = draft.configName ?? '';
      configUsername = draft.configUsername ?? '';
      // パスワードは一時保存から除外（セキュリティ対策）
      configPassword = '';
      configComment = draft.configComment ?? '';
      configUniverseId = draft.configUniverseId ?? '';
      configTickRate = typeof draft.configTickRate === 'number' ? draft.configTickRate : 60.0;
      configMaxConcurrentAssetTransfers = typeof draft.configMaxConcurrentAssetTransfers === 'number' ? draft.configMaxConcurrentAssetTransfers : 128;
      configUsernameOverride = draft.configUsernameOverride ?? '';
      configDataFolder = draft.configDataFolder ?? '';
      configCacheFolder = draft.configCacheFolder ?? '';
      configLogsFolder = draft.configLogsFolder ?? '';
      configAllowedUrlHosts = Array.isArray(draft.configAllowedUrlHosts) ? draft.configAllowedUrlHosts.join(',') : (draft.configAllowedUrlHosts ?? '');
      configAutoSpawnItems = Array.isArray(draft.configAutoSpawnItems) ? draft.configAutoSpawnItems.join(',') : (draft.configAutoSpawnItems ?? '');
      if (Array.isArray(draft.sessions) && draft.sessions.length) {
        sessions = draft.sessions;
        activeSessionTab = sessions[0]?.id ?? 1;
        nextSessionId = Math.max(...sessions.map(s => s.id)) + 1;
      }
      return true;
    } catch {
      return false;
    }
  };

  const saveDraft = () => {
    try {
      // 検索用の一時フィールドは保存しないようにセッションを整形
      const draftSessions = sessions.map((s) => {
        const { worldSearchResults, worldSearchLoading, worldSearchError, showWorldSearch, worldSearchTerm, ...rest } = s as any;
        return {
          ...rest,
          // 軽量化のため、UI状態は保存しない
        };
      });

      const draft = {
        configName,
        configUsername,
        // パスワードは一時保存から除外（セキュリティ対策）
        // configPassword を削除
        configComment,
        configUniverseId,
        configTickRate,
        configMaxConcurrentAssetTransfers,
        configUsernameOverride,
        configDataFolder,
        configCacheFolder,
        configLogsFolder,
        configAllowedUrlHosts: configAllowedUrlHosts.split(',').map(h => h.trim()).filter(Boolean),
        configAutoSpawnItems: configAutoSpawnItems.split(',').map(i => i.trim()).filter(Boolean),
        sessions: draftSessions,
        _expires: Date.now() + (30 * 60 * 1000) // 30分で期限切れ
      };
      localStorage.setItem(DRAFT_STORAGE_KEY, JSON.stringify(draft));
    } catch {
      // ignore
    }
  };

  const clearDraft = () => {
    localStorage.removeItem(DRAFT_STORAGE_KEY);
  };

  // コンフィグファイル読み込み機能
  const loadConfigFile = async () => {
    if (!selectedConfigToLoad || configLoadLoading) return;
    
    configLoadLoading = true;
    try {
      // パスからファイル名だけを抽出
      const fileName = selectedConfigToLoad.split(/[/\\]/).pop() || selectedConfigToLoad;
      
      // バックエンドからコンフィグファイルの内容を取得
      const response = await fetch(`/api/server/configs/${encodeURIComponent(fileName)}`);
      if (!response.ok) {
        if (response.status === 404) {
          throw new Error(`コンフィグファイル "${fileName}" が見つかりません`);
        }
        const errorText = await response.text();
        throw new Error(`コンフィグファイルの読み込みに失敗しました (${response.status}): ${errorText}`);
      }
      
      const configData = await response.json();
      
      // フォーム更新中フラグを立てる
      isFormClearing = true;
      
      // ファイル名から拡張子を除いた部分を取得
      const nameWithoutExt = fileName.replace(/\.json$/i, '');
      
      // 基本設定に反映（ヘルパー関数を使用）
      configName = nameWithoutExt || 'default';
      configUsername = configData.loginCredential || '';
      configPassword = ''; // セキュリティのため、読み込み時はパスワードを空にする
      configComment = configData.comment || '';
      configUniverseId = configData.universeId || '';
      configTickRate = configData.tickRate ?? 60.0;
      configMaxConcurrentAssetTransfers = configData.maxConcurrentAssetTransfers ?? 128;
      configUsernameOverride = configData.usernameOverride || '';
      configDataFolder = configData.dataFolder || '';
      configCacheFolder = configData.cacheFolder || '';
      configLogsFolder = configData.logsFolder || '';
      configAllowedUrlHosts = arrayToString(configData.allowedUrlHosts);
      configAutoSpawnItems = arrayToString(configData.autoSpawnItems);
      
      // セッション設定に反映（ヘルパー関数を使用）
      if (configData.startWorlds && Array.isArray(configData.startWorlds)) {
        sessions = configData.startWorlds.map((world: any, index: number) => {
          const { prefix, suffix } = splitCustomSessionId(world.customSessionId);
          
          return {
            id: index + 1,
            isEnabled: world.isEnabled !== false,
            sessionName: world.sessionName || '',
            customSessionId: world.customSessionId || '',
            customSessionIdPrefix: prefix,
            customSessionIdSuffix: suffix,
            description: world.description || '',
            maxUsers: world.maxUsers ?? 16,
            accessLevel: world.accessLevel || 'Anyone',
            useCustomJoinVerifier: world.useCustomJoinVerifier || false,
            hideFromPublicListing: world.hideFromPublicListing ?? null,
            tags: arrayToString(world.tags),
            mobileFriendly: world.mobileFriendly || false,
            loadWorldURL: world.loadWorldURL || '',
            loadWorldPresetName: world.loadWorldPresetName || 'Grid',
            overrideCorrespondingWorldId: world.overrideCorrespondingWorldId || '',
            forcePort: world.forcePort ?? null,
            keepOriginalRoles: world.keepOriginalRoles || false,
            defaultUserRoles: objectToJsonString(world.defaultUserRoles),
            roleCloudVariable: world.roleCloudVariable || '',
            allowUserCloudVariable: world.allowUserCloudVariable || '',
            denyUserCloudVariable: world.denyUserCloudVariable || '',
            requiredUserJoinCloudVariable: world.requiredUserJoinCloudVariable || '',
            requiredUserJoinCloudVariableDenyMessage: world.requiredUserJoinCloudVariableDenyMessage || '',
            awayKickMinutes: world.awayKickMinutes ?? null,
            parentSessionIds: arrayToString(world.parentSessionIds),
            autoInviteUsernames: arrayToString(world.autoInviteUsernames),
            autoInviteMessage: world.autoInviteMessage || '',
            saveAsOwner: world.saveAsOwner || '',
            autoRecover: world.autoRecover !== false,
            idleRestartInterval: world.idleRestartInterval ?? 1800,
            forcedRestartInterval: world.forcedRestartInterval ?? -1.0,
            saveOnExit: world.saveOnExit || false,
            autosaveInterval: world.autosaveInterval ?? -1.0,
            autoSleep: world.autoSleep !== false,
            showWorldSearch: false,
            worldSearchTerm: '',
            worldSearchLoading: false,
            worldSearchError: '',
            worldSearchResults: []
          };
        });
        
        activeSessionTab = 1;
        nextSessionId = sessions.length + 1;
      }
      
      pushToast('コンフィグファイルを読み込みました', 'success');
      
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コンフィグファイルの読み込みに失敗しました';
      pushToast(message, 'error');
    } finally {
      configLoadLoading = false;
      // フラグをリセット
      setTimeout(() => {
        isFormClearing = false;
      }, 0);
    }
  };

  // コンフィグファイル削除機能
  const deleteConfigFile = async () => {
    if (!selectedConfigToLoad || configDeleteLoading) return;
    
    const fileName = selectedConfigToLoad.split(/[/\\]/).pop() || selectedConfigToLoad;
    
    if (!confirm(`コンフィグファイル "${fileName}" を削除してもよろしいですか？\nこの操作は取り消せません。`)) {
      return;
    }
    
    configDeleteLoading = true;
    try {
      const response = await fetch(`/api/server/configs/${encodeURIComponent(fileName)}`, {
        method: 'DELETE'
      });
      
      if (!response.ok) {
        if (response.status === 404) {
          throw new Error(`コンフィグファイル "${fileName}" が見つかりません`);
        }
        const errorText = await response.text();
        throw new Error(`コンフィグファイルの削除に失敗しました (${response.status}): ${errorText}`);
      }
      
      pushToast('コンフィグファイルを削除しました', 'success');
      
      // 選択をクリア
      selectedConfigToLoad = '';
      
      // コンフィグファイル一覧を再取得
      const configsResponse = await getConfigs();
      setConfigs(configsResponse);
      
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コンフィグファイルの削除に失敗しました';
      pushToast(message, 'error');
    } finally {
      configDeleteLoading = false;
    }
  };

  // パスワードマスク機能
  const maskPasswordInJson = (jsonString: string): string => {
    try {
      const obj = JSON.parse(jsonString);
      const maskPasswords = (obj: any): any => {
        if (typeof obj === 'object' && obj !== null) {
          const masked = { ...obj };
          for (const key in masked) {
            if ((key.toLowerCase().includes('password') || key === 'loginPassword') && typeof masked[key] === 'string') {
              masked[key] = '****';
            } else if (typeof masked[key] === 'object') {
              masked[key] = maskPasswords(masked[key]);
            }
          }
          return masked;
        }
        return obj;
      };
      return JSON.stringify(maskPasswords(obj), null, 2);
    } catch {
      return jsonString;
    }
  };

  // ヘルパー関数: 配列⇔文字列の相互変換
  const arrayToString = (value: any): string => {
    if (Array.isArray(value)) {
      return value.map(v => String(v).trim()).filter(Boolean).join(', ');
    }
    if (typeof value === 'string') {
      return value;
    }
    return '';
  };

  const stringToArray = (value: string): string[] | null => {
    const trimmed = value.trim();
    if (!trimmed) return null;
    return trimmed.split(',').map(v => v.trim()).filter(Boolean);
  };

  // ヘルパー関数: customSessionIdの分割と結合
  const splitCustomSessionId = (customSessionId: string | null | undefined): { prefix: string; suffix: string } => {
    if (!customSessionId || typeof customSessionId !== 'string') {
      return { prefix: '', suffix: '' };
    }
    const parts = customSessionId.split(':');
    if (parts.length >= 2) {
      return {
        prefix: parts[0],
        suffix: parts.slice(1).join(':') // コロンが複数ある場合に対応
      };
    }
    // コロンがない場合は全体をサフィックスとして扱う
    return { prefix: '', suffix: customSessionId };
  };

  const joinCustomSessionId = (prefix: string, suffix: string): string | null => {
    const trimmedPrefix = prefix.trim();
    const trimmedSuffix = suffix.trim();
    if (trimmedPrefix && trimmedSuffix) {
      return `${trimmedPrefix}:${trimmedSuffix}`;
    }
    return null;
  };

  // ヘルパー関数: defaultUserRolesの型変換
  const objectToJsonString = (value: any): string => {
    if (!value) return '';
    if (typeof value === 'string') return value;
    if (typeof value === 'object') {
      try {
        return JSON.stringify(value);
      } catch {
        return '';
      }
    }
    return '';
  };

  const jsonStringToObject = (value: string): any => {
    if (!value || !value.trim()) return null;
    try {
      return JSON.parse(value);
    } catch {
      return null;
    }
  };

  // プレビュー編集機能
  const startPreviewEdit = () => {
    isPreviewEditing = true;
    // 元のパスワードを含むJSONを編集用に設定
    // configPreviewTextは既に元のパスワードを含んでいる
    editedPreviewText = configPreviewText;
    previewEditError = '';
    
  };

  const cancelPreviewEdit = () => {
    isPreviewEditing = false;
    editedPreviewText = '';
    previewEditError = '';
  };

  const savePreviewEdit = () => {
    try {
      // JSON構文チェック
      const parsed = JSON.parse(editedPreviewText);
      
      // 必須フィールドの検証
      const username = parsed.loginCredential;
      const password = parsed.loginPassword;
      
      if (!username || !password) {
        previewEditError = 'エラー: loginCredentialとloginPasswordは必須です';
        return;
      }

      // データ型の検証
      if (parsed.tickRate !== undefined && (typeof parsed.tickRate !== 'number' || parsed.tickRate <= 0)) {
        previewEditError = 'エラー: tickRateは正の数値である必要があります';
        return;
      }

      if (parsed.maxConcurrentAssetTransfers !== undefined && (typeof parsed.maxConcurrentAssetTransfers !== 'number' || parsed.maxConcurrentAssetTransfers <= 0)) {
        previewEditError = 'エラー: maxConcurrentAssetTransfersは正の数値である必要があります';
        return;
      }

      // startWorldsの検証
      if (parsed.startWorlds !== undefined && !Array.isArray(parsed.startWorlds)) {
        previewEditError = 'エラー: startWorldsは配列である必要があります';
        return;
      }

      if (parsed.startWorlds && parsed.startWorlds.length === 0) {
        previewEditError = 'エラー: 最低1つのセッション設定が必要です';
        return;
      }

      // セッション設定の検証
      if (parsed.startWorlds) {
        for (let i = 0; i < parsed.startWorlds.length; i++) {
          const world = parsed.startWorlds[i];
          
          if (world.maxUsers !== undefined && (typeof world.maxUsers !== 'number' || world.maxUsers < 0)) {
            previewEditError = `エラー: セッション${i + 1}のmaxUsersは0以上の数値である必要があります`;
            return;
          }

          if (world.defaultUserRoles !== undefined && world.defaultUserRoles !== null && typeof world.defaultUserRoles !== 'object') {
            previewEditError = `エラー: セッション${i + 1}のdefaultUserRolesはオブジェクトまたはnullである必要があります`;
            return;
          }
        }
      }

      // 編集内容を各フィールドに反映
      applyPreviewEditToFields(parsed);
      
      isPreviewEditing = false;
      editedPreviewText = '';
      previewEditError = '';
      
      pushToast('プレビューの編集を保存しました', 'success');
      
    } catch (error) {
      if (error instanceof SyntaxError) {
        previewEditError = `JSONの構文エラー: ${error.message}`;
      } else {
        previewEditError = `エラー: ${error instanceof Error ? error.message : '不明なエラーが発生しました'}`;
      }
    }
  };

  const applyPreviewEditToFields = (parsed: any) => {
    // 基本設定の反映
    if (parsed.comment) {
      configName = parsed.comment.replace('の設定ファイル', '');
      configComment = parsed.comment;
    }
    
    // ログイン情報の取得
    if (parsed.loginCredential) configUsername = parsed.loginCredential;
    if (parsed.loginPassword) configPassword = parsed.loginPassword;
    
    // 詳細設定の反映
    if (parsed.universeId !== undefined) configUniverseId = parsed.universeId || '';
    if (parsed.tickRate !== undefined) configTickRate = parsed.tickRate;
    if (parsed.maxConcurrentAssetTransfers !== undefined) configMaxConcurrentAssetTransfers = parsed.maxConcurrentAssetTransfers;
    if (parsed.usernameOverride !== undefined) configUsernameOverride = parsed.usernameOverride || '';
    if (parsed.dataFolder !== undefined) configDataFolder = parsed.dataFolder || '';
    if (parsed.cacheFolder !== undefined) configCacheFolder = parsed.cacheFolder || '';
    if (parsed.logsFolder !== undefined) configLogsFolder = parsed.logsFolder || '';
    
    // 配列フィールドの変換
    if (parsed.allowedUrlHosts !== undefined) {
      configAllowedUrlHosts = arrayToString(parsed.allowedUrlHosts);
    }
    if (parsed.autoSpawnItems !== undefined) {
      configAutoSpawnItems = arrayToString(parsed.autoSpawnItems);
    }
    
    // セッション設定の反映
    if (parsed.startWorlds && Array.isArray(parsed.startWorlds)) {
      sessions = parsed.startWorlds.map((world: any, index: number) => {
        // customSessionIdの分割処理
        const { prefix, suffix } = splitCustomSessionId(world.customSessionId);
        
        return {
          id: index + 1,
          isEnabled: world.isEnabled !== false,
          sessionName: world.sessionName || '',
          customSessionId: world.customSessionId || '',
          customSessionIdPrefix: prefix,
          customSessionIdSuffix: suffix,
          description: world.description || '',
          maxUsers: world.maxUsers ?? 16,
          accessLevel: world.accessLevel || 'Anyone',
          useCustomJoinVerifier: world.useCustomJoinVerifier || false,
          hideFromPublicListing: world.hideFromPublicListing ?? null,
          tags: arrayToString(world.tags),
          mobileFriendly: world.mobileFriendly || false,
          loadWorldURL: world.loadWorldURL || '',
          loadWorldPresetName: world.loadWorldPresetName || 'Grid',
          overrideCorrespondingWorldId: world.overrideCorrespondingWorldId || '',
          forcePort: world.forcePort ?? null,
          keepOriginalRoles: world.keepOriginalRoles || false,
          defaultUserRoles: objectToJsonString(world.defaultUserRoles),
          roleCloudVariable: world.roleCloudVariable || '',
          allowUserCloudVariable: world.allowUserCloudVariable || '',
          denyUserCloudVariable: world.denyUserCloudVariable || '',
          requiredUserJoinCloudVariable: world.requiredUserJoinCloudVariable || '',
          requiredUserJoinCloudVariableDenyMessage: world.requiredUserJoinCloudVariableDenyMessage || '',
          awayKickMinutes: world.awayKickMinutes ?? null,
          parentSessionIds: arrayToString(world.parentSessionIds),
          autoInviteUsernames: arrayToString(world.autoInviteUsernames),
          autoInviteMessage: world.autoInviteMessage || '',
          saveAsOwner: world.saveAsOwner || '',
          autoRecover: world.autoRecover !== false,
          idleRestartInterval: world.idleRestartInterval ?? 1800,
          forcedRestartInterval: world.forcedRestartInterval ?? -1.0,
          saveOnExit: world.saveOnExit || false,
          autosaveInterval: world.autosaveInterval ?? -1.0,
          autoSleep: world.autoSleep !== false,
          showWorldSearch: false,
          worldSearchTerm: '',
          worldSearchLoading: false,
          worldSearchError: '',
          worldSearchResults: []
        };
      });
      activeSessionTab = 1;
      nextSessionId = sessions.length + 1;
    }
  };

  const ROLE_OPTIONS = ['Admin', 'Builder', 'Moderator', 'Guest', 'Spectator'];
  const USER_ACTIONS = [
    { key: 'silence', label: 'ミュート' },
    { key: 'unsilence', label: 'ボイス許可' },
    { key: 'respawn', label: 'リスポーン' },
    { key: 'kick', label: 'キック' },
    { key: 'ban', label: 'BAN' }
  ] as const;

  const isCommandLog = (entry: LogEntry) => {
    if (entry.level !== 'stdout') return false;
    const trimmed = entry.message.trimStart();
    if (!trimmed.startsWith('>')) return false;
    return Boolean(trimmed.slice(1).trim());
  };

  const getActionLabel = (key: UserActionDefinition['key']) =>
    USER_ACTIONS.find(action => action.key === key)?.label ?? key;

  const DEFAULT_ACCESS_LEVELS = ['Private', 'LAN', 'Contacts', 'ContactsPlus', 'RegisteredUsers', 'Anyone'];

  // システムメトリクスの表示用にリアクティブに変換
  $: resourceMetrics = [
    { 
      label: 'CPU', 
      value: $metrics ? `${$metrics.cpu.usage.toFixed(1)} %` : '--- %' 
    },
    { 
      label: 'メモリ', 
      value: $metrics ? `${($metrics.memory.used / 1024 / 1024 / 1024).toFixed(2)} GB` : '--- GB' 
    }
  ];

  const ENGINE_READY_REGEX = /Engine Ready!?/i;
  const START_LOG_REGEX = /(Initializing App|Starting running world)/i;
  const WORLD_RUNNING_REGEX = /World running\.\.\./i;
  const STARTUP_WORLD_DELAY = 4000;

  let accessLevelOptions = [...DEFAULT_ACCESS_LEVELS];
  let sessionNameInput = '';
  let sessionDescriptionInput = '';
  let maxUsersInput: number | null = null;
  let awayKickMinutesInput: number | null = null;
  let accessLevelInput = '';
  let hiddenFromListingInput = false;
  let statusActionLoading: Record<string, boolean> = {};

  type UserActionDefinition = (typeof USER_ACTIONS)[number];

  let userRoleSelections: Record<string, string> = {};
  let userActionLoading: Record<string, boolean> = {};
  const setStatusLoading = (key: string, value: boolean) => {
    statusActionLoading = { ...statusActionLoading, [key]: value };
  };

  afterUpdate(() => {
    if (logContainer) {
      logContainer.scrollTop = logContainer.scrollHeight;
    }
  });

  $: if (!$status.running) {
    pendingStartup = false;
    runtimeWorlds = null;
    selectedWorldId = null;
    if (startupFinalizeTimer) {
      clearTimeout(startupFinalizeTimer);
      startupFinalizeTimer = null;
    }
    showStartupMessage = false;
  }

  $: if ($status.running && !wasRunning && !pendingStartup) {
    refreshWorlds(true);
    refreshRuntimeInfo(true);
  }

  $: wasRunning = $status.running;

  $: currentWorld = runtimeWorlds?.data.find(world => world.sessionId === selectedWorldId) ?? null;

  const refreshWorlds = async (suppressError = false): Promise<number | null> => {
    worldsLoading = true;
    if (!suppressError) worldsError = '';
    let count: number | null = null;
    try {
      const worlds = await getRuntimeWorlds();
      const unique: RuntimeWorldEntry[] = [];
      const seen = new Set<string>();
      for (const world of worlds.data) {
        const key = world.sessionId || world.focusTarget || world.name;
        if (key && seen.has(key)) continue;
        if (key) seen.add(key);
        unique.push(world);
      }
      runtimeWorlds = { ...worlds, data: unique };
      count = unique.length;

      let preferred: RuntimeWorldEntry | undefined;
      if (worlds.focusedSessionId) {
        preferred = unique.find(entry => entry.sessionId === worlds.focusedSessionId);
      }
      if (!preferred && worlds.focusedFocusTarget) {
        preferred = unique.find(entry => entry.focusTarget === worlds.focusedFocusTarget);
      }
      if (!preferred) {
        preferred = unique.find(entry => entry.focused);
      }
      if (!preferred && selectedWorldId) {
        preferred = unique.find(entry => entry.sessionId === selectedWorldId);
      }
      if (!preferred) {
        preferred = unique[0];
      }

      selectedWorldId = preferred?.sessionId ?? null;

      if (selectedWorldId) {
        localStorage.setItem(WORLD_STORAGE_KEY, selectedWorldId);
      } else {
        localStorage.removeItem(WORLD_STORAGE_KEY);
      }
    } catch (error) {
      runtimeWorlds = null;
      if (!suppressError) {
        worldsError = error instanceof Error ? error.message : 'セッションを取得できませんでした';
      }
      count = null;
    } finally {
      worldsLoading = false;
    }
    return count;
  };

  const refreshRuntimeStatus = async (suppressError = false) => {
    statusLoading = true;
    try {
      runtimeStatus = await getRuntimeStatus();
      if (runtimeStatus?.data) {
        sessionNameInput = runtimeStatus.data.name ?? '';
        sessionDescriptionInput = runtimeStatus.data.description ?? '';
        maxUsersInput = runtimeStatus.data.maxUsers ?? null;
        awayKickMinutesInput = (runtimeStatus.data as any).awayKickMinutes ?? null;
        accessLevelInput = runtimeStatus.data.accessLevel ?? '';
        hiddenFromListingInput = runtimeStatus.data.hiddenFromListing ?? false;
        const combinedLevels = new Set([...DEFAULT_ACCESS_LEVELS]);
        if (runtimeStatus.data.accessLevel) {
          combinedLevels.add(runtimeStatus.data.accessLevel);
        }
        accessLevelOptions = Array.from(combinedLevels);
      } else {
        sessionNameInput = '';
        sessionDescriptionInput = '';
        maxUsersInput = null;
        awayKickMinutesInput = null;
        accessLevelInput = '';
        hiddenFromListingInput = false;
        accessLevelOptions = [...DEFAULT_ACCESS_LEVELS];
      }
    } catch (error) {
      runtimeStatus = null;
      if (!suppressError) {
        const message = error instanceof Error ? error.message : 'ステータス情報を取得できませんでした';
        appMessage = { type: 'error', text: message };
      }
    } finally {
      statusLoading = false;
    }
  };

  const refreshRuntimeUsers = async (suppressError = false): Promise<number | null> => {
    usersLoading = true;
    let userCount: number | null = null;
    try {
      runtimeUsers = await getRuntimeUsers();
      if (runtimeUsers?.data) {
        const nextSelections: Record<string, string> = {};
        runtimeUsers.data.forEach(user => {
          nextSelections[user.name] = userRoleSelections[user.name] ?? user.role;
        });
        userRoleSelections = nextSelections;
        userCount = runtimeUsers.data.length;
      }
    } catch (error) {
      runtimeUsers = null;
      if (!suppressError) {
        const message = error instanceof Error ? error.message : 'ユーザー情報を取得できませんでした';
        appMessage = { type: 'error', text: message };
      }
      userCount = null;
    } finally {
      usersLoading = false;
    }
    return userCount;
  };

  const refreshRuntimeInfo = async (suppressError = false): Promise<number | null> => {
    const [, userCount] = await Promise.all([
      refreshRuntimeStatus(suppressError),
      refreshRuntimeUsers(suppressError)
    ]);
    return userCount;
  };

  const refreshConfigsOnly = async () => {
    const configList = await getConfigs();
    setConfigs(configList);
    if (configList.length && !configList.some(item => item.path === selectedConfig)) {
      const stored = localStorage.getItem(STORAGE_KEY);
      const match = configList.find(item => item.path === stored) ?? configList[0];
      selectedConfig = match.path;
      localStorage.setItem(STORAGE_KEY, selectedConfig);
    }
  };

  const addSession = () => {
    const newSession = {
      id: nextSessionId++,
      isEnabled: true,
      sessionName: '',
      customSessionId: '',
      customSessionIdPrefix: '',
      customSessionIdSuffix: '',
      description: '',
      maxUsers: 16,
      accessLevel: 'Anyone',
      useCustomJoinVerifier: false,
      hideFromPublicListing: null,
      tags: '',
      mobileFriendly: false,
      loadWorldURL: '',
      loadWorldPresetName: 'Grid',
      overrideCorrespondingWorldId: '',
      forcePort: null,
      keepOriginalRoles: false,
      defaultUserRoles: '',
      roleCloudVariable: '',
      allowUserCloudVariable: '',
      denyUserCloudVariable: '',
      requiredUserJoinCloudVariable: '',
      requiredUserJoinCloudVariableDenyMessage: '',
      awayKickMinutes: null,
      parentSessionIds: '',
      autoInviteUsernames: '',
      autoInviteMessage: '',
      saveAsOwner: '',
      autoRecover: true,
      idleRestartInterval: 1800,
      forcedRestartInterval: -1.0,
      saveOnExit: false,
      autosaveInterval: -1.0,
      autoSleep: true,
      // ワールド検索関連のプロパティを追加
      showWorldSearch: false,
      worldSearchTerm: '',
      worldSearchLoading: false,
      worldSearchError: '',
      worldSearchResults: []
    };
    sessions = [...sessions, newSession];
    activeSessionTab = newSession.id;
  };

  const removeSession = (sessionId: number) => {
    if (sessions.length <= 1) {
      pushToast('最低1つのセッションが必要です', 'error');
      return;
    }
    sessions = sessions.filter(s => s.id !== sessionId);
    if (activeSessionTab === sessionId) {
      activeSessionTab = sessions[0].id;
    }
  };

  const getCurrentSession = () => {
    return sessions.find(s => s.id === activeSessionTab) || sessions[0];
  };

  const generateConfigFile = async () => {
    if (configGenerationLoading) return;
    configGenerationLoading = true;
    try {
      const trimmedName = configName.trim();
      const trimmedUsername = configUsername.trim();
      const trimmedPassword = configPassword.trim();
      
      // バリデーション: 設定名、ユーザー名、パスワードは必須
      if (!trimmedName) {
        pushToast('設定名を入力してください', 'error');
        return;
      }
      
      if (!trimmedUsername) {
        pushToast('ユーザー名を入力してください', 'error');
        return;
      }
      
      if (!trimmedPassword) {
        pushToast('パスワードを入力してください', 'error');
        return;
      }

      // セッションデータを処理（ヘルパー関数を使用）
      const processedSessions = sessions.map(session => {
        const processedSession: any = {
          isEnabled: session.isEnabled,
          maxUsers: session.maxUsers,
          accessLevel: session.accessLevel,
          useCustomJoinVerifier: session.useCustomJoinVerifier,
          mobileFriendly: session.mobileFriendly,
          keepOriginalRoles: session.keepOriginalRoles,
          awayKickMinutes: session.awayKickMinutes,
          autoRecover: session.autoRecover,
          idleRestartInterval: session.idleRestartInterval,
          forcedRestartInterval: session.forcedRestartInterval,
          saveOnExit: session.saveOnExit,
          autosaveInterval: session.autosaveInterval,
          autoSleep: session.autoSleep
        };

        // 文字列フィールド: 空は null
        processedSession.sessionName = session.sessionName.trim() || null;
        
        // customSessionId: ヘルパー関数を使用して結合
        processedSession.customSessionId = joinCustomSessionId(
          (session as any).customSessionIdPrefix, 
          (session as any).customSessionIdSuffix
        );
        
        processedSession.description = session.description.trim() || null;
        processedSession.tags = stringToArray(session.tags);
        processedSession.loadWorldURL = session.loadWorldURL.trim() || null;
        processedSession.loadWorldPresetName = session.loadWorldPresetName.trim() || 'Grid';
        processedSession.overrideCorrespondingWorldId = session.overrideCorrespondingWorldId.trim() || null;
        processedSession.forcePort = (session.forcePort !== null && session.forcePort !== '') ? Number(session.forcePort) : null;
        
        // defaultUserRoles: ヘルパー関数を使用
        processedSession.defaultUserRoles = jsonStringToObject(session.defaultUserRoles);
        
        processedSession.roleCloudVariable = session.roleCloudVariable.trim() || null;
        processedSession.allowUserCloudVariable = session.allowUserCloudVariable.trim() || null;
        processedSession.denyUserCloudVariable = session.denyUserCloudVariable.trim() || null;
        processedSession.requiredUserJoinCloudVariable = session.requiredUserJoinCloudVariable.trim() || null;
        processedSession.requiredUserJoinCloudVariableDenyMessage = session.requiredUserJoinCloudVariableDenyMessage.trim() || null;
        
        // 配列フィールド: ヘルパー関数を使用
        processedSession.parentSessionIds = stringToArray(session.parentSessionIds);
        processedSession.autoInviteUsernames = stringToArray(session.autoInviteUsernames);
        
        processedSession.autoInviteMessage = session.autoInviteMessage.trim() || null;
        processedSession.saveAsOwner = session.saveAsOwner.trim() || null;
        
        // hideFromPublicListing: 常に含める（デフォルトはfalse）
        processedSession.hideFromPublicListing = !!session.hideFromPublicListing;

        return processedSession;
      });

      const configData = {
        comment: configComment.trim() || null,
        universeId: configUniverseId.trim() || null,
        tickRate: configTickRate,
        maxConcurrentAssetTransfers: configMaxConcurrentAssetTransfers,
        usernameOverride: configUsernameOverride.trim() || null,
        dataFolder: configDataFolder.trim() || null,
        cacheFolder: configCacheFolder.trim() || null,
        logsFolder: configLogsFolder.trim() || null,
        allowedUrlHosts: stringToArray(configAllowedUrlHosts),
        autoSpawnItems: stringToArray(configAutoSpawnItems),
        startWorlds: processedSessions
      };

      // 既存ファイル名との衝突チェック
      const currentList = $configs as ConfigEntry[];
      const exists = currentList.some(item => item.name === `${trimmedName}.json`);
      let overwrite = false;
      if (exists) {
        // 上書き確認ダイアログ
        overwrite = window.confirm(`${trimmedName}.json は既に存在します。上書きしますか？`);
        if (!overwrite) {
          pushToast('作成をキャンセルしました', 'info');
          return;
        }
      }

      // ここで初めてバックエンドへ送信（"作成"ボタン押下時）
      await generateConfig(trimmedName, trimmedUsername, trimmedPassword, configData, overwrite);
      pushToast('コンフィグファイルを作成しました', 'success');
      
      // 設定ファイル一覧を更新
      await refreshConfigsOnly();
      
      // フォームをクリア（リアクティブステートメントをスキップ）
      isFormClearing = true;
      configName = '';
      configUsername = '';
      configPassword = '';
      configComment = '';
      configUniverseId = '';
      configTickRate = 60.0;
      configMaxConcurrentAssetTransfers = 128;
      configUsernameOverride = '';
      configDataFolder = '';
      configCacheFolder = '';
      configLogsFolder = '';
      configAllowedUrlHosts = 'https://ttsapi.markn2000.com/';
      configAutoSpawnItems = '';
      sessions = [{
        id: 1,
        isEnabled: true,
        sessionName: '',
        customSessionId: '',
        customSessionIdPrefix: '',
        customSessionIdSuffix: '',
        description: '',
        maxUsers: 16,
        accessLevel: 'Anyone',
        useCustomJoinVerifier: false,
        hideFromPublicListing: null,
        tags: '',
        mobileFriendly: false,
        loadWorldURL: '',
        loadWorldPresetName: 'Grid',
        overrideCorrespondingWorldId: '',
        forcePort: null,
        keepOriginalRoles: false,
        defaultUserRoles: '',
        roleCloudVariable: '',
        allowUserCloudVariable: '',
        denyUserCloudVariable: '',
        requiredUserJoinCloudVariable: '',
        requiredUserJoinCloudVariableDenyMessage: '',
        awayKickMinutes: null,
        parentSessionIds: '',
        autoInviteUsernames: '',
        autoInviteMessage: '',
        saveAsOwner: '',
        autoRecover: true,
        idleRestartInterval: 1800,
        forcedRestartInterval: -1.0,
        saveOnExit: false,
        autosaveInterval: -1.0,
        autoSleep: true
      }];
      activeSessionTab = 1;
      nextSessionId = 2;
      clearDraft();
    } catch (error) {
      const message = error instanceof Error ? error.message : '設定ファイルの生成に失敗しました';
      pushToast(message, 'error');
    } finally {
      // フラグをリセット（必ず実行される）
      configGenerationLoading = false;
      // 次のティックでクリアフラグをリセット
      setTimeout(() => {
        isFormClearing = false;
      }, 0);
    }
  };


  // プレビューをリアクティブに更新
  $: {
    if (activeTab === 'settings') {
      const trimmedName = configName.trim() || 'Config';
      const trimmedUsername = configUsername.trim();
      const trimmedPassword = configPassword.trim();

      // sessionsを確実に配列として扱う
      const sessionsList = [...sessions];
      
      const processedSessions = Array.from(sessionsList.map(session => {
        const processedSession: any = {
          isEnabled: session.isEnabled,
          maxUsers: session.maxUsers,
          accessLevel: session.accessLevel,
          useCustomJoinVerifier: session.useCustomJoinVerifier,
          mobileFriendly: session.mobileFriendly,
          keepOriginalRoles: session.keepOriginalRoles,
          awayKickMinutes: session.awayKickMinutes,
          autoRecover: session.autoRecover,
          idleRestartInterval: session.idleRestartInterval,
          forcedRestartInterval: session.forcedRestartInterval,
          saveOnExit: session.saveOnExit,
          autosaveInterval: session.autosaveInterval,
          autoSleep: session.autoSleep
        };
        
        // 文字列フィールド: 空は null
        processedSession.sessionName = session.sessionName.trim() || null;
        
        // customSessionId: ヘルパー関数を使用して結合
        processedSession.customSessionId = joinCustomSessionId(
          (session as any).customSessionIdPrefix, 
          (session as any).customSessionIdSuffix
        );
        
        processedSession.description = session.description.trim() || null;
        processedSession.tags = stringToArray(session.tags);
        processedSession.loadWorldURL = session.loadWorldURL.trim() || null;
        processedSession.loadWorldPresetName = session.loadWorldPresetName.trim() || 'Grid';
        processedSession.overrideCorrespondingWorldId = session.overrideCorrespondingWorldId.trim() || null;
        processedSession.forcePort = (session.forcePort !== null && session.forcePort !== '') ? Number(session.forcePort) : null;
        
        // defaultUserRoles: ヘルパー関数を使用
        processedSession.defaultUserRoles = jsonStringToObject(session.defaultUserRoles);
        
        processedSession.roleCloudVariable = session.roleCloudVariable.trim() || null;
        processedSession.allowUserCloudVariable = session.allowUserCloudVariable.trim() || null;
        processedSession.denyUserCloudVariable = session.denyUserCloudVariable.trim() || null;
        processedSession.requiredUserJoinCloudVariable = session.requiredUserJoinCloudVariable.trim() || null;
        processedSession.requiredUserJoinCloudVariableDenyMessage = session.requiredUserJoinCloudVariableDenyMessage.trim() || null;
        
        // 配列フィールド: ヘルパー関数を使用
        processedSession.parentSessionIds = stringToArray(session.parentSessionIds);
        processedSession.autoInviteUsernames = stringToArray(session.autoInviteUsernames);
        
        processedSession.autoInviteMessage = session.autoInviteMessage.trim() || null;
        processedSession.saveAsOwner = session.saveAsOwner.trim() || null;
        
        // hideFromPublicListing: 値がnull以外の場合のみ含める
        if (session.hideFromPublicListing !== null && session.hideFromPublicListing !== undefined) {
          processedSession.hideFromPublicListing = session.hideFromPublicListing;
        }
        
        return processedSession;
      }));

      const configObject = {
        "$schema": "https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json",
        "comment": configComment.trim() || null,
        "universeId": configUniverseId.trim() || null,
        "tickRate": configTickRate,
        "maxConcurrentAssetTransfers": configMaxConcurrentAssetTransfers,
        "usernameOverride": configUsernameOverride.trim() || null,
        "loginCredential": trimmedUsername || null,
        "loginPassword": trimmedPassword || null,
        "startWorlds": JSON.parse(JSON.stringify(processedSessions)),
        "dataFolder": configDataFolder.trim() || null,
        "cacheFolder": configCacheFolder.trim() || null,
        "logsFolder": configLogsFolder.trim() || null,
        "allowedUrlHosts": stringToArray(configAllowedUrlHosts),
        "autoSpawnItems": stringToArray(configAutoSpawnItems)
      };

      configPreviewText = JSON.stringify(configObject, null, 2);
    }
  };

  const applyConfigList = (configList: ConfigEntry[]) => {
    const storedConfig = localStorage.getItem(STORAGE_KEY);
    const defaultConfig = configList.find(item => item.path === storedConfig) ?? configList[0];
    selectedConfig = defaultConfig?.path;
    if (selectedConfig) {
      localStorage.setItem(STORAGE_KEY, selectedConfig);
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  };

  const loadInitialData = async () => {
    initialLoading = true;
    try {
      const configList = await getConfigs();
      backendReachable = true;
      appMessage = null;
      if (configList.length === 0) {
        throw Object.assign(new Error('設定ファイルが見つかりません。設定ファイルを作成してください。'), { retryable: true, configMissing: true });
      }
      setConfigs(configList);
      applyConfigList(configList);

      const statusValue = await getStatus();
      setStatus(statusValue);
      setLogs(await getLogs(LOG_DISPLAY_LIMIT));

      if (statusValue.running) {
        selectedWorldId = localStorage.getItem(WORLD_STORAGE_KEY);
        await Promise.all([refreshWorlds(true), refreshRuntimeInfo(true)]);
      } else {
        userRoleSelections = {};
        sessionNameInput = '';
        sessionDescriptionInput = '';
        maxUsersInput = null;
        awayKickMinutesInput = null;
        accessLevelInput = '';
        hiddenFromListingInput = false;
        accessLevelOptions = [...DEFAULT_ACCESS_LEVELS];
      }
    } catch (error) {
      const isConfigMissing = error instanceof Error && (error as { configMissing?: boolean }).configMissing;
      if (isConfigMissing) {
        backendReachable = true; // バックエンドは接続できている
        appMessage = { type: 'warning', text: '設定ファイルが見つかりません。設定タブで設定ファイルを作成してください。' };
      } else {
        backendReachable = false;
        const message = error instanceof Error ? error.message : 'バックエンドに接続できません。バックエンドが起動しているか確認してください。';
        appMessage = { type: 'error', text: message };
        if (error instanceof Error && (error as { retryable?: boolean }).retryable) {
          setTimeout(() => {
            if (!initialLoading) {
              initialLoading = true;
            }
            loadInitialData();
          }, INITIAL_RETRY_DELAY);
        }
      }
    } finally {
      initialLoading = false;
    }
  };

  // リアクティブステートメント: 自動再起動タブ切り替え時の処理
  $: if (activeTab === 'restart' && isAuthenticated && !restartConfig) {
    loadRestartData();
  }

  // リアクティブステートメント: restartConfigの自動保存（デバウンス5秒）
  $: if (restartConfig && restartConfigInitialized && activeTab === 'restart') {
    // 既存のタイマーをクリア
    if (restartConfigDebounceTimer) {
      clearTimeout(restartConfigDebounceTimer);
    }
    
    // 5秒後に自動保存
    restartConfigDebounceTimer = setTimeout(() => {
      autoSaveRestartConfig();
    }, 5000);
  }

  onMount(() => {
    // 認証チェック（ブラウザ環境でのみ）
    if (typeof window !== 'undefined') {
      checkAuth();
    }
    
    // 起動時に下書き復元を試行
    const restored = loadDraft();
    if (restored) {
      pushToast('編集中の下書きを復元しました', 'info');
    }
    
    // ページ離脱時に一時保存データを自動削除
    const handleBeforeUnload = () => {
      clearDraft();
    };
    
    window.addEventListener('beforeunload', handleBeforeUnload);
    
    // クリーンアップ関数
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  });

  onMount(() => {
    const unsubscribeConfigs = configs.subscribe(current => {
      if (!initialLoading) {
        if (current.length === 0) {
          backendReachable = false;
          refreshConfigsOnly().catch(err => {
            console.error('Failed to refresh configs after reconnect:', err);
          });
        } else {
          backendReachable = true;
          appMessage = null;
          applyConfigList(current);
        }
      }
    });

    const unsubscribeLogs = logs.subscribe(entries => {
      if (!entries.length) return;
      if (!logsInitialized) {
        logsInitialized = true;
        lastProcessedLogId = entries[entries.length - 1].id;
        return;
      }

      for (const entry of entries) {
        if (entry.id <= lastProcessedLogId) continue;
        lastProcessedLogId = entry.id;
        const message = entry.message;

        if (START_LOG_REGEX.test(message)) {
          pendingStartup = true;
          showStartupMessage = true;
          if (startupFinalizeTimer) {
            clearTimeout(startupFinalizeTimer);
            startupFinalizeTimer = null;
          }
          startupRetryCount = 0;
          continue;
        }

        if (pendingStartup && ENGINE_READY_REGEX.test(message)) {
          continue;
        }

        if (pendingStartup && WORLD_RUNNING_REGEX.test(message)) {
        lastWorldRunningAt = Date.now();
        pendingStartup = true;
        startupWorldsReady = false;
        startupUsersReady = false;
        startupRetryCount = 0;
        scheduleStartupFinalize();
          continue;
        }
      }
    });

    loadInitialData();

    return () => {
      unsubscribeConfigs();
      unsubscribeLogs();
    };
  });

  // 入力変化を監視して下書きを保存（高頻度対策で簡易スロットル）
  let draftSaveTimer: ReturnType<typeof setTimeout> | null = null;
  const triggerSaveDraft = () => {
    if (draftSaveTimer) clearTimeout(draftSaveTimer);
    draftSaveTimer = setTimeout(() => {
      saveDraft();
      draftSaveTimer = null;
    }, 500);
  };

  // 設定タブの関連変数が変化したときのみトリガー（ログ等の外部更新では発火しない）
  $: if (activeTab === 'settings' && !isFormClearing) {
    // 依存として触れておくことで、これらの値が変わった時だけ反応
    configName; configUsername; configPassword; configComment; configUniverseId;
    configTickRate; configMaxConcurrentAssetTransfers; configUsernameOverride;
    configDataFolder; configCacheFolder; configLogsFolder; configAllowedUrlHosts;
    configAutoSpawnItems; sessions;
    triggerSaveDraft();
  }

  const sendUserAction = async (username: string, action: UserActionDefinition['key']) => {
    if (!username) {
      pushToast('ユーザー名が取得できませんでした', 'error');
      return;
    }
    userActionLoading = { ...userActionLoading, [`${username}-${action}`]: true };
    try {
      await postCommand(`${action} "${username}"`);
      pushToast(`${getActionLabel(action)} を送信しました`, 'success');
      if ($status.running) {
        await Promise.all([refreshRuntimeInfo(true), refreshWorlds(true)]);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
      pushToast(message, 'error');
    } finally {
      userActionLoading = { ...userActionLoading, [`${username}-${action}`]: false };
    }
  };

  const updateUserRole = async (username: string, role: string) => {
    if (!username) {
      pushToast('ユーザー名が取得できませんでした', 'error');
      return;
    }
    if (!role) {
      pushToast('ロールを選択してください', 'error');
      return;
    }
    userActionLoading = { ...userActionLoading, [`${username}-role`]: true };
    try {
      await postCommand(`role "${username}" "${role}"`);
      pushToast(`ロールを ${role} に変更しました`, 'success');
      if ($status.running) {
        await Promise.all([refreshRuntimeInfo(true), refreshWorlds(true)]);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'ロール変更に失敗しました';
      pushToast(message, 'error');
    } finally {
      userActionLoading = { ...userActionLoading, [`${username}-role`]: false };
    }
  };

  const sendStatusCommand = async (key: string, command: string, successMessage: string) => {
    if (!$status.running) {
      pushToast('サーバーが起動していません', 'error');
      return;
    }
    setStatusLoading(key, true);
    try {
      await ensureSelectedWorldFocused();
      await postCommand(command);
      pushToast(successMessage, 'success');
      await Promise.all([refreshRuntimeInfo(true), refreshWorlds(true)]);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
      pushToast(message, 'error');
    } finally {
      setStatusLoading(key, false);
    }
  };

  const applySessionName = async () => {
    const value = sessionNameInput.trim();
    if (!value) {
      pushToast('セッション名を入力してください', 'error');
      return;
    }
    await sendStatusCommand('name', `name ${JSON.stringify(value)}`, 'セッション名を更新しました');
  };

  const copySessionId = async () => {
    const sessionId = runtimeStatus?.data?.sessionId;
    if (!sessionId) return;
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard) {
        await navigator.clipboard.writeText(sessionId);
        pushToast('SessionIDをコピーしました', 'success');
      } else {
        pushToast('クリップボードに対応していません', 'error');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コピーに失敗しました';
      pushToast(message, 'error');
    }
  };

  const copyPasteSessionUrl = async () => {
    const sessionId = runtimeStatus?.data?.sessionId;
    if (!sessionId) return;
    const pasteUrl = `ressession:///${sessionId}`;
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard) {
        await navigator.clipboard.writeText(pasteUrl);
        pushToast('貼付け用URLをコピーしました', 'success');
      } else {
        pushToast('クリップボードに対応していません', 'error');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コピーに失敗しました';
      pushToast(message, 'error');
    }
  };

  const applyMaxUsers = async () => {
    if (maxUsersInput === null || Number.isNaN(maxUsersInput)) {
      pushToast('最大人数を入力してください', 'error');
      return;
    }
    if (!Number.isFinite(maxUsersInput) || maxUsersInput < 0) {
      pushToast('0以上の数値を入力してください', 'error');
      return;
    }
    await sendStatusCommand('maxUsers', `maxusers ${Math.floor(maxUsersInput)}`, '最大人数を更新しました');
  };

  const applyAwayKickInterval = async () => {
    if (awayKickMinutesInput === null || Number.isNaN(awayKickMinutesInput)) {
      pushToast('最大AFK時間を入力してください', 'error');
      return;
    }
    if (!Number.isFinite(awayKickMinutesInput) || awayKickMinutesInput < 0) {
      pushToast('0以上の数値を入力してください', 'error');
      return;
    }
    await sendStatusCommand('awayKickMinutes', `awaykickinterval ${Math.floor(awayKickMinutesInput)}`, '最大AFK時間を更新しました');
  };

  const applyAccessLevel = async (value?: string) => {
    const nextLevel = value ?? accessLevelInput;
    if (!nextLevel) {
      pushToast('アクセスレベルを選択してください', 'error');
      return;
    }
    accessLevelInput = nextLevel;
    await sendStatusCommand('accessLevel', `accesslevel ${nextLevel}`, `アクセスレベルを${nextLevel}に設定しました`);
  };

  const handleHiddenFromListingChange = async (checked: boolean) => {
    hiddenFromListingInput = checked;
    await sendStatusCommand(
      'hidden',
      `hidefromlisting ${checked ? 'true' : 'false'}`,
      checked ? 'リスト非表示に設定しました' : 'リスト表示に設定しました'
    );
  };

  const applyDescription = async () => {
    await sendStatusCommand('description', `description ${JSON.stringify(sessionDescriptionInput)}`, '説明を更新しました');
  };

  const executeSessionCommand = async (command: 'save' | 'close' | 'restart') => {
    let successMessage = '';
    switch (command) {
      case 'save':
        successMessage = 'ワールドを保存しました';
        break;
      case 'close':
        successMessage = 'セッションを閉じるコマンドを送信しました';
        break;
      case 'restart':
        successMessage = 'セッション再起動コマンドを送信しました';
        break;
      default:
        successMessage = 'コマンドを送信しました';
        break;
    }
    await sendStatusCommand(command, command, successMessage);
  };

  const handleStart = async () => {
    actionInProgress = true;
    pendingStartup = true;
    showStartupMessage = true;
    startupRetryCount = 0;
    try {
      await startServer(selectedConfig);
      appMessage = { type: 'info', text: 'サーバー起動コマンドを送信しました' };
      pendingStartup = true;
      lastWorldRunningAt = Date.now();
      scheduleStartupFinalize();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'サーバーを起動できませんでした';
      appMessage = { type: 'error', text: message };
      pendingStartup = false;
    } finally {
      actionInProgress = false;
    }
  };

  const handleStop = async () => {
    actionInProgress = true;
    try {
      await stopServer();
      await refreshRuntimeInfo(true);
      appMessage = { type: 'info', text: 'サーバー停止コマンドを送信しました' };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'サーバーを停止できませんでした';
      appMessage = { type: 'error', text: message };
    } finally {
      actionInProgress = false;
    }
  };

  const submitTemplate = async () => {
    if (templateLoading) return;
    templateLoading = true;
    try {
      const trimmed = templateName.trim();
      if (!trimmed) {
        pushToast('テンプレート名を入力してください。', 'error');
      } else {
        await postCommand(`startworldtemplate ${JSON.stringify(trimmed)}`);
        pushToast('テンプレートを起動しました。', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'テンプレートを起動できませんでした';
      pushToast(message, 'error');
    } finally {
      templateLoading = false;
    }
  };

  const submitWorldUrl = async () => {
    if (worldUrlLoading) return;
    worldUrlLoading = true;
    try {
      const trimmed = worldUrl.trim();
      if (!trimmed) {
        pushToast('URLを入力してください。', 'error');
      } else {
        await postCommand(`startworldurl ${JSON.stringify(trimmed)}`);
        pushToast('URLから起動しました。', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'URLから起動できませんでした';
      pushToast(message, 'error');
    } finally {
      worldUrlLoading = false;
    }
  };

  const submitWorldSearch = async () => {
    if (worldSearchLoading) return;
    worldSearchLoading = true;
    worldSearchError = '';
    selectedResoniteUrl = null;
    try {
      const term = worldSearchTerm.trim();
      if (!term) {
        worldSearchResults = [];
      } else {
        const resp = await getWorldSearch(term);
        worldSearchResults = resp.items ?? [];
      }
    } catch (error) {
      worldSearchError = error instanceof Error ? error.message : '検索に失敗しました';
      worldSearchResults = [];
    } finally {
      worldSearchLoading = false;
    }
  };

  const launchSelectedWorld = async () => {
    if (!selectedResoniteUrl) return;
    try {
      await postCommand(`startworldurl ${JSON.stringify(selectedResoniteUrl)}`);
      pushToast('選択したワールドを起動しました。', 'success');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'ワールドを起動できませんでした';
      pushToast(message, 'error');
    }
  };

  const fetchFriendRequests = async () => {
    if (friendRequestsLoading) return;
    friendRequestsLoading = true;
    try {
      friendRequests = await getFriendRequests();
    } catch (error) {
      friendRequestsError = error instanceof Error ? error.message : 'フレンドリクエストを取得できませんでした';
    } finally {
      friendRequestsLoading = false;
    }
  };

  const submitFriendSend = async () => {
    if (friendSendLoading) return;
    friendSendLoading = true;
    try {
      const target = friendTargetName.trim();
      if (!target) {
        pushToast('ユーザー名を入力してください。', 'error');
      } else {
        await postCommand(`sendfriendrequest ${JSON.stringify(target)}`);
        pushToast('フレンド申請を送りました。', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'フレンド申請を送れませんでした';
      pushToast(message, 'error');
    } finally {
      friendSendLoading = false;
    }
  };

  const submitFriendAccept = async () => {
    if (friendAcceptLoading) return;
    friendAcceptLoading = true;
    try {
      const target = friendTargetName.trim();
      if (!target) {
        pushToast('ユーザー名を入力してください。', 'error');
      } else {
        await postCommand(`acceptfriendrequest ${JSON.stringify(target)}`);
        pushToast('フレンド申請を承認しました。', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'フレンド申請を承認できませんでした';
      pushToast(message, 'error');
    } finally {
      friendAcceptLoading = false;
    }
  };

  const submitFriendRemove = async () => {
    if (friendRemoveLoading) return;
    friendRemoveLoading = true;
    try {
      const target = friendTargetName.trim();
      if (!target) {
        pushToast('ユーザー名を入力してください。', 'error');
      } else {
        await postCommand(`removefriend ${JSON.stringify(target)}`);
        pushToast('フレンドを解除しました。', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'フレンドを解除できませんでした';
      pushToast(message, 'error');
    } finally {
      friendRemoveLoading = false;
    }
  };

  const submitFriendMessage = async () => {
    if (friendMessageLoading) return;
    friendMessageLoading = true;
    try {
      const target = friendTargetName.trim();
      const text = friendMessageText.trim();
      if (!target || !text) {
        pushToast('ユーザー名とメッセージを入力してください。', 'error');
      } else {
        // 改行を<br>に置き換え
        const formattedText = text.replace(/\n/g, '<br>');
        await postCommand(`message ${JSON.stringify(target)} ${JSON.stringify(formattedText)}`);
        pushToast('メッセージを送信しました。', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'メッセージを送信できませんでした';
      pushToast(message, 'error');
    } finally {
      friendMessageLoading = false;
    }
  };

  // 自動再起動機能 - データ読み込み
  const loadRestartData = async () => {
    try {
      restartConfigLoading = true;
      restartStatusLoading = true;
      
      const [config, status] = await Promise.all([
        getRestartConfig(),
        getRestartStatus()
      ]);
      
      restartConfig = config;
      restartStatus = status;
      
      // 初回読み込み完了フラグをセット
      restartConfigInitialized = true;
    } catch (error) {
      console.error('[RestartManagement] Failed to load data:', error);
      pushToast('再起動設定の読み込みに失敗しました', 'error');
    } finally {
      restartConfigLoading = false;
      restartStatusLoading = false;
    }
  };

  // 予定再起動 - 新規追加モーダルを開く
  const openAddScheduleModal = () => {
    editingScheduleId = null;
    editingSchedule = {
      id: '',
      enabled: true,
      type: 'once',
      specificDate: {
        year: new Date().getFullYear(),
        month: new Date().getMonth() + 1,
        day: new Date().getDate(),
        hour: 3,
        minute: 0
      },
      weeklyDay: 0,
      weeklyTime: { hour: 3, minute: 0 },
      dailyTime: { hour: 3, minute: 0 },
      configFile: '__previous__'
    };
    scheduledRestartModalOpen = true;
  };

  // 予定再起動 - 編集モーダルを開く
  const openEditScheduleModal = (schedule: any) => {
    editingScheduleId = schedule.id;
    editingSchedule = JSON.parse(JSON.stringify(schedule)); // Deep copy
    scheduledRestartModalOpen = true;
  };

  // 予定再起動 - モーダルを閉じる
  const closeScheduleModal = () => {
    scheduledRestartModalOpen = false;
    editingScheduleId = null;
    editingSchedule = null;
  };

  // 予定再起動 - 保存
  const saveSchedule = () => {
    if (!restartConfig || !editingSchedule) return;

    if (editingScheduleId) {
      // 編集
      const index = restartConfig.triggers.scheduled.schedules.findIndex(
        (s: any) => s.id === editingScheduleId
      );
      if (index !== -1) {
        restartConfig.triggers.scheduled.schedules[index] = { ...editingSchedule };
      }
    } else {
      // 新規追加
      editingSchedule.id = `schedule-${Date.now()}`;
      restartConfig.triggers.scheduled.schedules = [
        ...restartConfig.triggers.scheduled.schedules,
        { ...editingSchedule }
      ];
    }

    closeScheduleModal();
    pushToast('予定を保存しました（5秒後に自動保存されます）', 'info');
  };

  // 予定再起動 - 削除
  const deleteSchedule = (scheduleId: string) => {
    if (!restartConfig) return;
    
    if (confirm('この予定を削除しますか？')) {
      restartConfig.triggers.scheduled.schedules = restartConfig.triggers.scheduled.schedules.filter(
        (s: any) => s.id !== scheduleId
      );
      pushToast('予定を削除しました（5秒後に自動保存されます）', 'info');
    }
  };

  // 予定再起動 - 有効/無効切り替え
  const toggleScheduleEnabled = (scheduleId: string) => {
    if (!restartConfig) return;
    
    const schedule = restartConfig.triggers.scheduled.schedules.find((s: any) => s.id === scheduleId);
    if (schedule) {
      schedule.enabled = !schedule.enabled;
      restartConfig = restartConfig; // Trigger reactivity
    }
  };

  // 強制再起動ボタン
  const handleForceRestart = async () => {
    if (forceRestartLoading) return;
    
    if (!confirm('⚠️ 強制再起動を実行しますか？\nこの操作は即座に実行されます。')) {
      return;
    }
    
    forceRestartLoading = true;
    try {
      await triggerRestart('forced');
      pushToast('強制再起動を開始しました', 'success');
    } catch (error: any) {
      const message = error.message || '強制再起動の開始に失敗しました';
      pushToast(message, 'error');
    } finally {
      forceRestartLoading = false;
    }
  };

  // 手動再起動トリガーボタン
  const handleManualRestart = async () => {
    if (manualRestartLoading) return;
    
    if (!confirm('手動再起動をトリガーしますか？\n再起動前アクションが実行されます。')) {
      return;
    }
    
    manualRestartLoading = true;
    try {
      await triggerRestart('manual');
      pushToast('手動再起動をトリガーしました', 'success');
    } catch (error: any) {
      const message = error.message || '手動再起動のトリガーに失敗しました';
      pushToast(message, 'error');
    } finally {
      manualRestartLoading = false;
    }
  };

  // 自動保存処理（デバウンス付き）
  const autoSaveRestartConfig = async () => {
    if (!restartConfig || restartSaveLoading) return;
    
    restartSaveLoading = true;
    try {
      await saveRestartConfig(restartConfig);
      console.log('[RestartManagement] Auto-saved configuration');
      
      // 保存後にステータスを再読み込み
      const status = await getRestartStatus();
      restartStatus = status;
    } catch (error: any) {
      const message = error.message || '設定の自動保存に失敗しました';
      pushToast(message, 'error');
    } finally {
      restartSaveLoading = false;
    }
  };

  // 再起動設定をリセット
  const handleResetRestartConfig = async () => {
    if (!confirm('設定をデフォルトにリセットしますか？\n\n現在の設定は失われます。')) {
      return;
    }
    
    try {
      // デバウンスタイマーをクリア
      if (restartConfigDebounceTimer) {
        clearTimeout(restartConfigDebounceTimer);
        restartConfigDebounceTimer = null;
      }
      
      // 初期化フラグをリセット
      restartConfigInitialized = false;
      
      // リセットAPIを呼び出し
      const result = await resetRestartConfig();
      
      // 返されたデフォルト設定を適用
      restartConfig = result.config;
      
      // ステータスも再取得
      restartStatus = await getRestartStatus();
      
      // 初期化フラグをセット
      restartConfigInitialized = true;
      
      pushToast('設定をデフォルトにリセットしました', 'success');
    } catch (error) {
      const message = error instanceof Error ? error.message : '設定のリセットに失敗しました';
      pushToast(message, 'error');
    }
  };

  // フレンド管理タブ - 新機能の関数
  const searchFriendByUsername = async () => {
    const username = friendSearchUsername.trim();
    if (!username) {
      pushToast('ユーザー名を入力してください', 'error');
      return;
    }

    if (friendSearchUsernameLoading) return;
    friendSearchUsernameLoading = true;

    try {
      // 検索前に結果とBAN一覧をリセット
      friendSearchResults = [];
      bansList = [];
      selectedFriendUser = null;
      selectedBanEntry = null;

      const response = await searchResoniteUsers(username);
      
      if (response.count === 0) {
        pushToast('該当するユーザーが見つかりませんでした', 'info');
      } else {
        // 新しい検索結果を設定
        friendSearchResults = response.users;
        pushToast(`${response.count}件のユーザーが見つかりました`, 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'ユーザー情報の取得に失敗しました';
      pushToast(`検索失敗: ${message}`, 'error');
    } finally {
      friendSearchUsernameLoading = false;
    }
  };

  const searchFriendByUserId = async () => {
    const userIdSuffix = friendSearchUserId.trim();
    if (!userIdSuffix) {
      pushToast('ユーザーIDを入力してください', 'error');
      return;
    }

    // プレフィックスを自動で付与
    const fullUserId = `U-${userIdSuffix}`;

    if (friendSearchUserIdLoading) return;
    friendSearchUserIdLoading = true;

    try {
      // 検索前に結果とBAN一覧をリセット
      friendSearchResults = [];
      bansList = [];
      selectedFriendUser = null;
      selectedBanEntry = null;

      const user = await getResoniteUserFull(fullUserId);
      // 検索結果を設定
      friendSearchResults = [user];
      pushToast('ユーザーを見つけました', 'success');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'ユーザー情報の取得に失敗しました';
      pushToast(`検索失敗: ${message}`, 'error');
    } finally {
      friendSearchUserIdLoading = false;
    }
  };

  const loadFriendRequestsList = async () => {
    if (friendRequestsLoading) return;
    friendRequestsLoading = true;
    friendRequestsError = '';
    
    // フレンドリクエスト一覧を取得する際は検索結果と検索フィールドをクリア
    friendSearchResults = [];
    friendSearchUsername = '';
    friendSearchUserId = '';

    try {
      const requests = await getFriendRequests();
      friendRequests = requests;

      // フレンドリクエストを送ってきたユーザーの情報を取得
      if (requests.data && requests.data.length > 0) {
        const promises = requests.data.map(async (username) => {
          try {
            const user = await getResoniteUserFull(username);
            return user;
          } catch (error) {
            console.error(`Failed to fetch user info for ${username}:`, error);
            return null;
          }
        });

        const users = await Promise.all(promises);
        const validUsers = users.filter((u): u is ResoniteUserFull => u !== null);

        // 検索結果に設定
        friendSearchResults = validUsers;

        pushToast(`${validUsers.length}件のフレンドリクエストを読み込みました`, 'success');
      } else {
        pushToast('フレンドリクエストはありません', 'info');
      }
    } catch (error) {
      friendRequestsError = error instanceof Error ? error.message : 'フレンドリクエストの取得に失敗しました';
      pushToast(friendRequestsError, 'error');
    } finally {
      friendRequestsLoading = false;
    }
  };

  const loadBannedUsersList = async () => {
    if (bansLoading) return;
    bansLoading = true;
    
    try {
      // 検索結果とBAN一覧を切り替えるため、検索結果と検索フィールドをクリア
      friendSearchResults = [];
      friendSearchUsername = '';
      friendSearchUserId = '';
      selectedFriendUser = null;
      
      const response = await getBans();
      bansList = response.data;
      selectedBanEntry = null;
      
      if (bansList.length === 0) {
        pushToast('BANリストは空です', 'info');
      } else {
        pushToast(`${bansList.length}件のBANエントリを読み込みました`, 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'BAN一覧の取得に失敗しました';
      pushToast(message, 'error');
    } finally {
      bansLoading = false;
    }
  };
  
  const loadFocusedSessionUsers = async () => {
    if (focusedSessionUsersLoading) return;
    focusedSessionUsersLoading = true;
    
    try {
      // 検索結果とBAN一覧をクリア
      friendSearchResults = [];
      friendSearchUsername = '';
      friendSearchUserId = '';
      bansList = [];
      selectedFriendUser = null;
      selectedBanEntry = null;
      
      const response = await getRuntimeUsers();
      
      if (response.data && response.data.length > 0) {
        // 各ユーザーのIDから完全な情報を取得
        const promises = response.data.map(async (user) => {
          try {
            const fullUser = await getResoniteUserFull(user.id);
            return fullUser;
          } catch (error) {
            console.error(`Failed to fetch user info for ${user.id}:`, error);
            return null;
          }
        });
        
        const users = await Promise.all(promises);
        const validUsers = users.filter((u): u is ResoniteUserFull => u !== null);
        
        // 検索結果に設定
        friendSearchResults = validUsers;
        
        pushToast(`${validUsers.length}件のユーザーを読み込みました`, 'success');
      } else {
        pushToast('フォーカス中のセッションにユーザーがいません', 'info');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'ユーザー一覧の取得に失敗しました';
      pushToast(message, 'error');
    } finally {
      focusedSessionUsersLoading = false;
    }
  };

  const selectFriendUser = (user: ResoniteUserFull) => {
    selectedFriendUser = user;
    selectedBanEntry = null; // BAN選択を解除
  };
  
  const selectBanEntry = (entry: BanEntry) => {
    selectedBanEntry = entry;
    selectedFriendUser = null; // フレンドユーザー選択を解除
  };
  
  const unbanSelectedUser = async () => {
    if (!selectedBanEntry) {
      pushToast('BANエントリを選択してください', 'error');
      return;
    }
    
    if (!confirm(`${selectedBanEntry.username} (${selectedBanEntry.userId}) のBANを解除しますか？`)) {
      return;
    }
    
    try {
      // unbanコマンドはユーザーIDを使用
      await postCommand(`unban ${selectedBanEntry.userId}`);
      pushToast(`${selectedBanEntry.username} のBANを解除しました`, 'success');
      
      // BAN一覧を再読み込み
      await loadBannedUsersList();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'BANの解除に失敗しました';
      pushToast(message, 'error');
    }
  };

  const sendFriendRequestToSelected = async () => {
    if (!selectedFriendUser) {
      pushToast('ユーザーを選択してください', 'error');
      return;
    }

    if (friendSendLoading) return;
    friendSendLoading = true;

    try {
      const username = selectedFriendUser.username || selectedFriendUser.id;
      await postCommand(`sendfriendrequest ${JSON.stringify(username)}`);
      pushToast(`${username} にフレンド申請を送信しました`, 'success');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'フレンド申請の送信に失敗しました';
      pushToast(message, 'error');
    } finally {
      friendSendLoading = false;
    }
  };

  const acceptFriendRequestFromSelected = async () => {
    if (!selectedFriendUser) {
      pushToast('ユーザーを選択してください', 'error');
      return;
    }

    // フレンドリクエスト一覧に含まれているかチェック
    const isInRequests = friendRequests?.data?.some(
      username => username.toLowerCase() === selectedFriendUser?.username?.toLowerCase()
    );

    if (!isInRequests) {
      pushToast('このユーザーからのフレンドリクエストはありません', 'error');
      return;
    }

    if (friendAcceptLoading) return;
    friendAcceptLoading = true;

    try {
      const username = selectedFriendUser.username || selectedFriendUser.id;
      await postCommand(`acceptfriendrequest ${JSON.stringify(username)}`);
      pushToast(`${username} からのフレンド申請を承認しました`, 'success');

      // 承認後、リストから削除
      friendSearchResults = friendSearchResults.filter(u => u.id !== selectedFriendUser?.id);
      if (selectedFriendUser) {
        selectedFriendUser = null;
      }

      // フレンドリクエストリストを再取得
      await loadFriendRequestsList();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'フレンド申請の承認に失敗しました';
      pushToast(message, 'error');
    } finally {
      friendAcceptLoading = false;
    }
  };

  const removeFriendFromSelected = async () => {
    if (!selectedFriendUser) {
      pushToast('ユーザーを選択してください', 'error');
      return;
    }

    if (friendRemoveLoading) return;
    friendRemoveLoading = true;

    try {
      const username = selectedFriendUser.username || selectedFriendUser.id;
      await postCommand(`removefriend ${JSON.stringify(username)}`);
      pushToast(`${username} をフレンドから解除しました`, 'success');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'フレンドの解除に失敗しました';
      pushToast(message, 'error');
    } finally {
      friendRemoveLoading = false;
    }
  };

  const sendMessageToSelected = async () => {
    if (!selectedFriendUser) {
      pushToast('ユーザーを選択してください', 'error');
      return;
    }

    const text = friendMessageText.trim();
    if (!text) {
      pushToast('メッセージを入力してください', 'error');
      return;
    }

    if (friendMessageLoading) return;
    friendMessageLoading = true;

    try {
      const username = selectedFriendUser.username || selectedFriendUser.id;
      // 改行を<br>に置き換え
      const formattedText = text.replace(/\n/g, '<br>');
      await postCommand(`message ${JSON.stringify(username)} ${JSON.stringify(formattedText)}`);
      pushToast(`${username} にメッセージを送信しました`, 'success');
      friendMessageText = ''; // メッセージ送信後にクリア
    } catch (error) {
      const message = error instanceof Error ? error.message : 'メッセージの送信に失敗しました';
      pushToast(message, 'error');
    } finally {
      friendMessageLoading = false;
    }
  };

  const inviteToFocusedSession = async () => {
    if (!selectedFriendUser) {
      pushToast('ユーザーを選択してください', 'error');
      return;
    }

    if (!runtimeWorlds?.focusedSessionId) {
      pushToast('フォーカス中のセッションがありません', 'error');
      return;
    }

    if (friendInviteLoading) return;
    friendInviteLoading = true;

    try {
      const username = selectedFriendUser.username || selectedFriendUser.id;
      await postCommand(`invite ${JSON.stringify(username)}`);
      pushToast(`${username} をフォーカス中のセッション (${runtimeWorlds.focusedSessionName || runtimeWorlds.focusedSessionId}) に招待しました`, 'success');
    } catch (error) {
      const message = error instanceof Error ? error.message : '招待に失敗しました';
      pushToast(message, 'error');
    } finally {
      friendInviteLoading = false;
    }
  };

  const executeCommand = async () => {
    if (commandLoading) return;
    commandLoading = true;
    try {
      const result = await postCommand(commandText);
      commandResult = typeof result === 'string' ? result : JSON.stringify(result);
      commandText = ''; // コマンド実行成功後に入力欄を空にする
    } catch (error) {
      commandResult = error instanceof Error ? error.message : 'コマンドを実行できませんでした';
    } finally {
      commandLoading = false;
    }
  };

  const spawnItem = async () => {
    if (itemSpawnLoading) return;
    itemSpawnLoading = true;
    try {
      const trimmed = itemSpawnUrl.trim();
      if (!trimmed) {
        pushToast('アイテムのURLを入力してください。', 'error');
      } else {
        await postCommand(`spawnitem ${JSON.stringify(trimmed)}`);
        pushToast('アイテムをスポーンしました。', 'success');
        itemSpawnUrl = ''; // 成功後に入力欄を空にする
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'アイテムのスポーンに失敗しました';
      pushToast(message, 'error');
    } finally {
      itemSpawnLoading = false;
    }
  };

  const sendDynamicImpulse = async () => {
    if (dynamicImpulseLoading) return;
    dynamicImpulseLoading = true;
    try {
      const trimmedTag = dynamicImpulseTag.trim();
      const trimmedText = dynamicImpulseText.trim();
      if (!trimmedTag || !trimmedText) {
        pushToast('タグとテキストを入力してください。', 'error');
      } else {
        await postCommand(`dynamicimpulse ${JSON.stringify(trimmedTag)} ${JSON.stringify(trimmedText)}`);
        pushToast('ダイナミックインパルスを送信しました。', 'success');
        dynamicImpulseTag = ''; // 成功後に入力欄を空にする
        dynamicImpulseText = '';
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'ダイナミックインパルスの送信に失敗しました';
      pushToast(message, 'error');
    } finally {
      dynamicImpulseLoading = false;
    }
  };

  const focusWorld = async (world: RuntimeWorldEntry) => {
    if (!$status.running) return;
    worldsError = '';
    worldsLoading = true;
    statusLoading = true;
    usersLoading = true;
    try {
      const target = world.focusTarget ?? world.sessionId;
      const response = await postFocusWorldRefresh(target);

      const unique: RuntimeWorldEntry[] = [];
      const seen = new Set<string>();
      for (const entry of response.worlds.data) {
        const key = entry.sessionId || entry.focusTarget || entry.name;
        if (key && seen.has(key)) continue;
        if (key) seen.add(key);
        unique.push(entry);
      }
      runtimeWorlds = { ...response.worlds, data: unique };

      let preferred: RuntimeWorldEntry | undefined;
      if (response.worlds.focusedSessionId) {
        preferred = unique.find(item => item.sessionId === response.worlds.focusedSessionId);
      }
      if (!preferred && response.worlds.focusedFocusTarget) {
        preferred = unique.find(item => item.focusTarget === response.worlds.focusedFocusTarget);
      }
      if (!preferred) {
        preferred = unique.find(item => item.focused);
      }
      if (!preferred) {
        preferred = unique.find(item => item.sessionId === world.sessionId);
      }
      if (!preferred) {
        preferred = unique[0];
      }

      selectedWorldId = preferred?.sessionId ?? null;
      if (selectedWorldId) {
        localStorage.setItem(WORLD_STORAGE_KEY, selectedWorldId);
      } else {
        localStorage.removeItem(WORLD_STORAGE_KEY);
      }

      runtimeStatus = response.status;
      if (runtimeStatus?.data) {
        sessionNameInput = runtimeStatus.data.name ?? '';
        sessionDescriptionInput = runtimeStatus.data.description ?? '';
        maxUsersInput = runtimeStatus.data.maxUsers ?? null;
        awayKickMinutesInput = (runtimeStatus.data as any).awayKickMinutes ?? null;
        accessLevelInput = runtimeStatus.data.accessLevel ?? '';
        hiddenFromListingInput = runtimeStatus.data.hiddenFromListing ?? false;

        const combinedLevels = new Set([...DEFAULT_ACCESS_LEVELS]);
        if (runtimeStatus.data.accessLevel) {
          combinedLevels.add(runtimeStatus.data.accessLevel);
        }
        accessLevelOptions = Array.from(combinedLevels);
      } else {
        sessionNameInput = '';
        sessionDescriptionInput = '';
        maxUsersInput = null;
        awayKickMinutesInput = null;
        accessLevelInput = '';
        hiddenFromListingInput = false;
        accessLevelOptions = [...DEFAULT_ACCESS_LEVELS];
      }

      runtimeUsers = response.users;
      if (runtimeUsers?.data) {
        const nextSelections: Record<string, string> = {};
        runtimeUsers.data.forEach(user => {
          nextSelections[user.name] = userRoleSelections[user.name] ?? user.role;
        });
        userRoleSelections = nextSelections;
      } else {
        userRoleSelections = {};
      }
    } catch (error) {
      worldsError = error instanceof Error ? error.message : 'フォーカスに失敗しました';
    } finally {
      worldsLoading = false;
      statusLoading = false;
      usersLoading = false;
    }
  };

  const scheduleStartupFinalize = () => {
    if (startupFinalizeTimer) {
      clearTimeout(startupFinalizeTimer);
    }
    const delay = Math.max(STARTUP_WORLD_DELAY - (Date.now() - lastWorldRunningAt), 0);
    startupFinalizeTimer = setTimeout(() => {
      Promise.all([refreshWorlds(true), refreshRuntimeInfo(true)])
        .then(([worldsCount, usersCount]) => {
          startupWorldsReady = worldsCount !== null && worldsCount > 0;
          startupUsersReady = usersCount !== null;
          if (startupWorldsReady && startupUsersReady) {
            pendingStartup = false;
            if (showStartupMessage) {
              appMessage = { type: 'info', text: 'ヘッドレスの起動が完了しました' };
            }

            const focusedId = runtimeWorlds?.focusedSessionId;
            if (
              focusedId &&
              focusedId !== selectedWorldId &&
              (runtimeWorlds?.data?.some(world => world.sessionId === focusedId) ?? false)
            ) {
              selectedWorldId = focusedId;
              localStorage.setItem(WORLD_STORAGE_KEY, selectedWorldId);
            }
            startupRetryCount = 0;
          } else if (startupRetryCount < STARTUP_MAX_RETRIES) {
            startupRetryCount += 1;
            startupFinalizeTimer = setTimeout(() => {
              scheduleStartupFinalize();
            }, STARTUP_RETRY_INTERVAL);
          } else {
            pendingStartup = false;
            appMessage = {
              type: 'warning',
              text: '起動情報の取得に時間がかかっています。セッション一覧を再取得してください。'
            };
          }
        })
        .catch(error => {
          console.error('Failed to finalize startup data load:', error);
          if (startupRetryCount < STARTUP_MAX_RETRIES) {
            startupRetryCount += 1;
            startupFinalizeTimer = setTimeout(() => {
              scheduleStartupFinalize();
            }, STARTUP_RETRY_INTERVAL);
          } else {
            pendingStartup = false;
            appMessage = {
              type: 'error',
              text: '起動情報を取得できませんでした。手動で再取得してください。'
            };
          }
        })
        .finally(() => {
          if (!pendingStartup) {
            startupFinalizeTimer = null;
            showStartupMessage = false;
          }
        });
    }, delay);
  };

  $: headlessUserName = $status.userName ?? headlessUserName;
  $: headlessUserId = $status.userId ?? headlessUserId;

  const isHeadlessAccount = (user: any) => {
    if (headlessUserId) {
      if (user.id === headlessUserId) return true;
    }
    if (headlessUserName) {
      if (user.name === headlessUserName) return true;
    }
    return false;
  };

  const ensureSelectedWorldFocused = async () => {
    if (!$status.running) return;
    if (!selectedWorldId) return;
    if (!runtimeWorlds?.data?.length) return;

    if (runtimeWorlds.focusedSessionId === selectedWorldId) return;

    const targetWorld = runtimeWorlds.data.find(world => world.sessionId === selectedWorldId);
    if (!targetWorld) return;

    const focusTarget = targetWorld.focusTarget || targetWorld.sessionId;
    if (!focusTarget) return;

    try {
      await postFocusWorld(focusTarget);
      await refreshWorlds(true);
    } catch (error) {
      console.error('Failed to focus selected world before command:', error);
    }
  };

  // プレビュー編集モードになったときにtextareaの高さを自動調整
  afterUpdate(() => {
    if (isPreviewEditing && previewTextarea) {
      previewTextarea.style.height = 'auto';
      previewTextarea.style.height = previewTextarea.scrollHeight + 'px';
    }
  });
</script>

<svelte:head>
  <title>MarkN Resonite Headless Controller</title>
</svelte:head>

{#if showLogin}
  <!-- ログイン画面 -->
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <h1>MarkN Resonite Headless Controller</h1>
        <p>ログインが必要です</p>
      </div>
      <form on:submit|preventDefault={handleLogin} class="login-form">
        <div class="field">
          <label for="password">パスワード</label>
          <input 
            id="password"
            type="password" 
            bind:value={loginPassword} 
            placeholder="パスワードを入力"
            disabled={loginLoading}
            autocomplete="current-password"
          />
        </div>
        <button type="submit" class="login-button" disabled={loginLoading}>
          {#if loginLoading}
            ログイン中...
          {:else}
            ログイン
          {/if}
        </button>
      </form>
      <div class="login-info">
        <p>デフォルトパスワード: <code>admin123</code></p>
        <p><small>本番環境では必ずパスワードを変更してください</small></p>
      </div>
    </div>
  </div>
{:else}
<div class="layout">
  <header class="topbar">
    <div class="brand">
      <span class="logo">MRHC</span>
      <div>
        <h1>MarkN Resonite Headless Controller</h1>
        {#if headlessUserName}
          <p class="account-label">起動ユーザー: {headlessUserName}</p>
        {/if}
      </div>
    </div>
    <div class="topbar-controls">
      <label class="field">
        <div class="field-header">
          <span>コンフィグ選択</span>
          {#if backendReachable && $configs.length}
            <button type="button" class="refresh-config-button" on:click={refreshConfigsOnly} title="設定ファイル一覧を更新" aria-label="設定ファイル一覧を更新">
              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true">
                <path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" />
              </svg>
            </button>
          {/if}
        </div>
        {#if backendReachable && $configs.length}
          <div class="field-row">
            <div class="select-wrapper">
              <select bind:value={selectedConfig} disabled={$status.running || actionInProgress}>
                {#each $configs as config}
                  <option value={config.path}>{config.name}</option>
                {/each}
              </select>
            </div>
          </div>
        {:else}
          <div class="field-placeholder">利用可能な設定がありません</div>
        {/if}
      </label>
      <div class="resource-capsule combined">
        <div class="resource-row">
          <span>CPU</span>
          <strong>{resourceMetrics.find(m => m.label === 'CPU')?.value || 'N/A'}</strong>
        </div>
        <div class="resource-row">
          <span>メモリ</span>
          <strong>{resourceMetrics.find(m => m.label === 'メモリ')?.value || 'N/A'}</strong>
        </div>
      </div>
      <div class="status-indicators">
        <div class="status-info">
          <div class="backend-status" class:offline={!backendReachable}>
            <span class="dot"></span>
            {backendReachable ? '接続済' : '未接続'}
          </div>
          <div class={$status.running ? 'online' : 'offline'}>
            <span class="dot"></span>
            {$status.running ? '起動中' : '停止中'}
          </div>
        </div>
        <button type="button" on:click={handleStart} disabled={$status.running || actionInProgress || !$configs.length}>起動</button>
        <button type="button" on:click={handleStop} class="danger" disabled={!$status.running || actionInProgress}>停止</button>
        <button type="button" on:click={handleLogout} class="logout-button" title="ログアウト">ログアウト</button>
      </div>
    </div>
  </header>

  <div class="content">
    <aside class="sidebar">
      <section class="session-card">
        <div class="section-header">
          <h2>セッション一覧</h2>
          <button
            type="button"
            class="refresh-button"
            on:click={() => refreshWorlds()}
            disabled={!$status.running || worldsLoading}
            aria-label="セッション一覧を再取得"
          >
            <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true">
              <path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" />
            </svg>
          </button>
        </div>
        {#if worldsError}
          <p class="feedback">{worldsError}</p>
        {/if}
        <div class="session-list">
          {#if runtimeWorlds?.data?.length}
            {#each runtimeWorlds?.data || [] as world}
              <button
                type="button"
                class="session"
                class:active={selectedWorldId === world.sessionId}
                class:focused={world.focused}
                on:click={() => focusWorld(world)}
                disabled={!$status.running}
              >
                <div class="session-body">
                  <div class="session-row">
                    <div class="session-name">{world.name}</div>
                  </div>
                  <div class="session-row">
                    <span class="session-access">{world.accessLevel ?? 'Unknown'}</span>
                    <span class="session-counts">
                      <span class="count-label">active</span>
                      <span class="present">{world.presentUsers ?? '-'}</span>
                      <span class="slash">/</span>
                      <span class="count-label">users</span>
                      <span class="total">{world.currentUsers ?? '-'}</span>
                      <span class="slash">/</span>
                      <span class="count-label">max</span>
                      <span class="max">{world.maxUsers ?? '-'}</span>
                    </span>
                  </div>
                </div>
              </button>
            {/each}
          {:else if worldsLoading}
            <p class="info">取得中...</p>
          {:else}
            <p class="empty">アクティブなセッションがありません。</p>
          {/if}
        </div>
      </section>

      <section class="logs-card compact">
        <div class="section-header">
          <h2>ログ</h2>
          <div class="header-actions">
            <button type="button" class="refresh-button" on:click={copyLogsToClipboard} aria-label="ログをコピー">
              <svg viewBox="0 0 24 24" class="refresh-icon" aria-hidden="true">
                <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v16h13c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 18H8V7h11v16z"/>
              </svg>
            </button>
            <button type="button" class="refresh-button" on:click={() => { clearLogs(); pushToast('表示中のログを消去しました', 'success'); }} aria-label="表示ログをクリア">
              <svg viewBox="0 0 24 24" class="refresh-icon" aria-hidden="true">
                <path d="M16 9v10H8V9h8m-1.5-6h-5l-1 1H5v2h14V4h-3.5l-1-1zM18 7H6v12a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V7z"/>
              </svg>
            </button>
          </div>
        </div>
        <div class="log-container selectable" bind:this={logContainer}>
          {#if !$logs.length}
            <p class="empty">まだログがありません。</p>
          {:else}
            {#each $logs.slice(-LOG_DISPLAY_LIMIT) as log}
              <div class:stderr={log.level === 'stderr'} class:command-log={isCommandLog(log)}>
                <pre>{log.message}</pre>
              </div>
            {/each}
          {/if}
        </div>
        
        <!-- コマンド入力エリア -->
        <div class="command-section">
          <div class="command-input">
            <input
              type="text"
              bind:value={commandText}
              placeholder="コマンドを入力（例: worlds）"
              on:keydown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault();
                  executeCommand();
                }
              }}
              disabled={!$status.running || commandLoading}
            />
            <button type="button" on:click={executeCommand} disabled={!$status.running || commandLoading || !commandText.trim()}>
              {commandLoading ? '実行中...' : '実行'}
            </button>
          </div>
          {#if commandResult}
            <pre class="command-result">{commandResult}</pre>
          {/if}
        </div>
      </section>
    </aside>

    <main class="dashboard">
      {#if initialLoading}
        <div class="panel notice loading">読み込み中...</div>
      {:else}
        {#if appMessage}
          <div class={`panel notice ${appMessage.type}`}>{appMessage.text}</div>
        {/if}

        <nav class="tab-bar">
          {#each tabs as tab}
            <button
              type="button"
              class:active={activeTab === tab.id}
              on:click={() => (activeTab = tab.id)}
            >
              {tab.label}
            </button>
          {/each}
        </nav>

        <div class="tab-panels">
          <section class="panel" class:active={activeTab === 'dashboard'}>
            <div class="panel-grid three">
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>セッション設定</h2>
                  <button
                    type="button"
                    class="refresh-button"
                    on:click={() => refreshRuntimeStatus()}
                    disabled={!$status.running || statusLoading}
                    aria-label="ステータス再取得"
                  >
                    <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true">
                      <path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" />
                    </svg>
                  </button>
                </div>
                <div class="card status-card" aria-busy={statusLoading}>
                  {#if !$status.running}
                    <p class="empty">サーバーが起動すると状態が表示されます。</p>
                  {:else if runtimeStatus}
                    <form class="status-form" on:submit|preventDefault={() => {}}>
                      <label>
                        <span>セッション名</span>
                        <div class="field-row">
                          <input type="text" bind:value={sessionNameInput} />
                          <button type="button" class="status-action-button" on:click={applySessionName} disabled={statusActionLoading.name}>
                            適用
                          </button>
                        </div>
                      </label>

                      <label>
                        <span>SessionID</span>
                        <div class="field-row">
                          <input class="muted" type="text" value={runtimeStatus.data.sessionId ?? ''} readonly />
                          <button type="button" class="status-action-button" on:click={copySessionId}>
                            IDコピー
                          </button>
                          <button type="button" class="status-action-button" on:click={copyPasteSessionUrl}>
                            貼付け用URL
                          </button>
                        </div>
                      </label>

                      <label>
                        <span>最大人数</span>
                        <div class="field-row">
                          <input
                            type="number"
                            min="0"
                            bind:value={maxUsersInput}
                            on:input={(event) => {
                              const value = (event.target as HTMLInputElement).value;
                              maxUsersInput = value === '' ? null : Number(value);
                            }}
                          />
                          <button type="button" class="status-action-button" on:click={applyMaxUsers} disabled={statusActionLoading.maxUsers}>
                            適用
                          </button>
                        </div>
                      </label>

                      <label>
                        <span>最大AFK時間(分)</span>
                        <div class="field-row">
                          <input
                            type="number"
                            min="0"
                            bind:value={awayKickMinutesInput}
                            on:input={(event) => {
                              const value = (event.target as HTMLInputElement).value;
                              awayKickMinutesInput = value === '' ? null : Number(value);
                            }}
                          />
                          <button type="button" class="status-action-button" on:click={applyAwayKickInterval} disabled={statusActionLoading.awayKickMinutes}>
                            適用
                          </button>
                        </div>
                      </label>

                      <label>
                        <span>アクセスレベル</span>
                        <div class="select-wrapper narrow">
                          <select
                            value={accessLevelInput || runtimeStatus.data.accessLevel || DEFAULT_ACCESS_LEVELS[0]}
                            on:change={(event) => {
                              accessLevelInput = (event.target as HTMLSelectElement).value;
                              applyAccessLevel(accessLevelInput);
                            }}
                            disabled={statusActionLoading.accessLevel}
                          >
                            {#each accessLevelOptions as level}
                              <option value={level}>{level}</option>
                            {/each}
                          </select>
                        </div>
                      </label>

                      <div class="toggle-row">
                        <span>セッションリストに表示しない</span>
                        <button
                          type="button"
                          class={hiddenFromListingInput ? 'status-action-button active' : 'status-action-button'}
                          aria-pressed={hiddenFromListingInput}
                          on:click={() => handleHiddenFromListingChange(!hiddenFromListingInput)}
                          disabled={statusActionLoading.hidden}
                        >
                          {hiddenFromListingInput ? 'オン' : 'オフ'}
                        </button>
                      </div>

                      <div class="description-block">
                        <div class="description-header">
                          <label for="session-description">説明</label>
                          <button type="button" class="status-action-button" on:click={applyDescription} disabled={statusActionLoading.description}>
                            適用
                          </button>
                        </div>
                        <textarea id="session-description" rows="4" bind:value={sessionDescriptionInput}></textarea>
                      </div>

                      <div class="action-buttons">
                        <button type="button" class="save" on:click={() => executeSessionCommand('save')} disabled={statusActionLoading.save}>
                          ワールドを保存
                        </button>
                        <button type="button" class="close" on:click={() => executeSessionCommand('close')} disabled={statusActionLoading.close}>
                          セッションを閉じる
                        </button>
                        <button type="button" class="restart" on:click={() => executeSessionCommand('restart')} disabled={statusActionLoading.restart}>
                          セッションを再起動
                        </button>
                      </div>
                    </form>
                  {:else}
                    <p class="empty">読み込み中...</p>
                  {/if}
                </div>
              </div>

              <div class="panel-column">
                <div class="panel-heading">
                  <h2>ユーザー</h2>
                  <button
                    type="button"
                    class="refresh-button"
                    on:click={() => refreshRuntimeUsers()}
                    disabled={!$status.running || usersLoading}
                    aria-label="ユーザー再取得"
                  >
                    <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true">
                      <path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" />
                    </svg>
                  </button>
                </div>
                <div class="card users-card" aria-busy={usersLoading}>
                  {#if !$status.running}
                    <p class="empty">サーバーが起動するとユーザーが表示されます。</p>
                  {:else if runtimeUsers}
                    {#if runtimeUsers.data.length}
                      <table>
                        <thead>
                          <tr>
                            <th>User</th>
                            <th>AFK</th>
                            <th>Role</th>
                            <th>Silenced</th>
                            <th>Actions</th>
                          </tr>
                        </thead>
                        <tbody>
                          {#each runtimeUsers.data as user}
                            <tr>
                              <td>
                                <strong>{user.name}</strong>
                                <span class="sub">{user.id}</span>
                              </td>
                              <td>{user.present ? 'Active' : 'AFK'}</td>
                              <td>
                                <div class="select-wrapper">
                                  <select
                                    class:disabled-control={isHeadlessAccount(user)}
                                    value={userRoleSelections[user.name] ?? user.role}
                                    on:change={(event) => {
                                      const value = (event.target as HTMLSelectElement).value;
                                      userRoleSelections = { ...userRoleSelections, [user.name]: value };
                                      updateUserRole(user.name, value);
                                    }}
                                    disabled={userActionLoading[`${user.name}-role`] || isHeadlessAccount(user)}
                                  >
                                    {#each ROLE_OPTIONS as option}
                                      <option value={option}>{option}</option>
                                    {/each}
                                  </select>
                                </div>
                              </td>
                              <td>
                                <div class="select-wrapper">
                                  <select
                                    class:disabled-control={isHeadlessAccount(user)}
                                    value={user.silenced ? 'silence' : 'unsilence'}
                                    on:change={(event) => {
                                      const value = (event.target as HTMLSelectElement).value as 'silence' | 'unsilence';
                                      sendUserAction(user.name, value);
                                    }}
                                    disabled={
                                      userActionLoading[`${user.name}-silence`] ||
                                      userActionLoading[`${user.name}-unsilence`] ||
                                      isHeadlessAccount(user)
                                    }
                                  >
                                    <option value="silence">ミュート</option>
                                    <option value="unsilence">ボイス許可</option>
                                  </select>
                                </div>
                              </td>
                              <td>
                                <div class="user-actions">
                                  {#each USER_ACTIONS as action}
                                    {#if action.key !== 'silence' && action.key !== 'unsilence'}
                                      <button
                                        type="button"
                                        class:disabled-control={isHeadlessAccount(user)}
                                        on:click={() => sendUserAction(user.name, action.key)}
                                        disabled={userActionLoading[`${user.name}-${action.key}`] || isHeadlessAccount(user)}
                                      >
                                        {action.label}
                                      </button>
                                    {/if}
                                  {/each}
                                </div>
                              </td>
                            </tr>
                          {/each}
                        </tbody>
                      </table>
                    {:else}
                      <p class="empty">ユーザー情報が取得できませんでした。</p>
                    {/if}
                  {:else}
                    <p class="empty">読み込み中...</p>
                  {/if}
                </div>
              </div>

              <div class="panel-column">
                <div class="panel-heading">
                  <h2>スポーン・パルス</h2>
                </div>
                <div class="card status-card">
                  <form class="status-form" on:submit|preventDefault={() => {}}>
                    <label>
                      <span>アイテムスポーン</span>
                      <div class="field-row">
                        <input
                          type="url"
                          bind:value={itemSpawnUrl}
                          placeholder="アイテムのURLを入力"
                        />
                        <button type="button" class="status-action-button" on:click={spawnItem} disabled={!$status.running || itemSpawnLoading}>
                          スポーン
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>ダイナミックインパルスstring</span>
                      <div class="field-row">
                        <input
                          type="text"
                          bind:value={dynamicImpulseTag}
                          placeholder="タグ"
                        />
                        <input
                          type="text"
                          bind:value={dynamicImpulseText}
                          placeholder="テキスト"
                        />
                        <button type="button" class="status-action-button" on:click={sendDynamicImpulse} disabled={!$status.running || dynamicImpulseLoading}>
                          送信
                        </button>
                      </div>
                    </label>
                  </form>
                </div>
              </div>
            </div>
          </section>

          <section class="panel" class:active={activeTab === 'newWorld'}>
            <div class="panel-grid two">
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>新規セッション</h2>
                </div>
                <div class="card status-card">
                  <form class="status-form" on:submit|preventDefault={() => {}}>
                    <label>
                      <span>テンプレートから起動</span>
                      <div class="field-row">
                        <div class="select-wrapper narrow">
                          <select
                            bind:value={templateName}
                            disabled={templateLoading}
                          >
                            {#each templateSuggestions as t}
                              <option value={t}>{t}</option>
                            {/each}
                          </select>
                        </div>
                        <button type="button" class="status-action-button" on:click={submitTemplate} disabled={!$status.running || templateLoading}>
                          起動
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>URLから起動</span>
                      <div class="field-row">
                        <input
                          type="text"
                          bind:value={worldUrl}
                          placeholder="resrec://..."
                        />
                        <button type="button" class="status-action-button" on:click={submitWorldUrl} disabled={!$status.running || worldUrlLoading}>
                          起動
                        </button>
                      </div>
                    </label>
                  </form>
                </div>
              </div>

              <div class="panel-column">
                <div class="panel-heading">
                  <h2>検索して起動</h2>
                </div>
                <div class="card status-card">
                  <form class="status-form" on:submit|preventDefault={() => {}}>
                    <label>
                      <span>ワールド検索</span>
                      <div class="field-row">
                        <input type="text" bind:value={worldSearchTerm} placeholder="キーワード" />
                        <button type="button" class="status-action-button" on:click={submitWorldSearch} disabled={worldSearchLoading}>
                          検索
                        </button>
                      </div>
                    </label>

                    {#if worldSearchLoading}
                      <p class="info">検索中...</p>
                    {:else if worldSearchError}
                      <p class="feedback">{worldSearchError}</p>
                    {:else if worldSearchResults.length === 0 && worldSearchTerm}
                      <p class="empty">該当するワールドが見つかりませんでした。</p>
                    {/if}

                    {#if worldSearchResults.length}
                      <div class="action-buttons">
                        <button type="button" class="save" on:click={launchSelectedWorld} disabled={!$status.running || !selectedResoniteUrl}>
                          このワールドを起動
                        </button>
                      </div>

                      <div class="world-grid">
                        {#each worldSearchResults as item}
                          <button
                            type="button"
                            class={selectedResoniteUrl === item.resoniteUrl ? 'world-card selected' : 'world-card'}
                            on:click={() => (selectedResoniteUrl = item.resoniteUrl)}
                          >
                            <div class="thumb" aria-hidden="true">
                              {#if item.imageUrl}
                                <img src={item.imageUrl} alt="" />
                              {:else}
                                <div class="thumb-placeholder">No Image</div>
                              {/if}
                            </div>
                              <div class="meta">
                                <div class="title-row">
                                  <strong class="title">{item.name}</strong>
                                  <span
                                    class="copy-button"
                                    role="button"
                                    tabindex="0"
                                    on:click|stopPropagation={() => copyToClipboard(item.name)}
                                    on:keydown={(e) => e.key === 'Enter' && copyToClipboard(item.name)}
                                    title="ワールド名をコピー"
                                  >
                                    <svg viewBox="0 0 24 24" class="copy-icon" aria-hidden="true">
                                      <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v16h13c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 18H8V7h11v16z"/>
                                    </svg>
                                  </span>
                                </div>
                                <div class="sub-row">
                                  <span class="sub">{item.recordId}</span>
                                  <span
                                    class="copy-button"
                                    role="button"
                                    tabindex="0"
                                    on:click|stopPropagation={() => copyToClipboard(item.recordId)}
                                    on:keydown={(e) => e.key === 'Enter' && copyToClipboard(item.recordId)}
                                    title="レコードIDをコピー"
                                  >
                                    <svg viewBox="0 0 24 24" class="copy-icon" aria-hidden="true">
                                      <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v16h13c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 18H8V7h11v16z"/>
                                    </svg>
                                  </span>
                                </div>
                              </div>
                          </button>
                        {/each}
                      </div>
                    {/if}
                  </form>
                </div>
              </div>
            </div>
          </section>

          {#if showConfigPreview}
            <section class="panel active">
              <div class="panel-grid one">
                <div class="panel-column">
                  <div class="card form-card">
                    <h2>プレビュー（保存前のJSON）</h2>
                    <div class="options">
                      <button type="button" on:click={() => { copyToClipboard(configPreviewText); pushToast('JSONをコピーしました', 'success'); }}>JSONをコピー</button>
                      <button type="button" on:click={() => { showConfigPreview = false; }}>閉じる</button>
                    </div>
                    <div class="full">
                      <pre>{configPreviewText}</pre>
                    </div>
                  </div>
                </div>
              </div>
            </section>
          {/if}

          <section class="panel" class:active={activeTab === 'friends'}>
            <div class="panel-grid two">
              <!-- 左側: ユーザー検索とリスト -->
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>ユーザー検索</h2>
                </div>
                <div class="card status-card">
                  <form class="status-form" on:submit|preventDefault={() => {}}>
                    <!-- ユーザー名で検索 -->
                    <label>
                      <span>ユーザー名で検索</span>
                      <div class="field-row">
                        <input 
                          type="text" 
                          bind:value={friendSearchUsername} 
                          placeholder="ユーザー名を入力" 
                        />
                        <button 
                          type="button" 
                          class="status-action-button" 
                          on:click={searchFriendByUsername}
                          disabled={friendSearchUsernameLoading}
                        >
                          {friendSearchUsernameLoading ? '検索中...' : '検索'}
                        </button>
                      </div>
                    </label>

                    <!-- ユーザーIDで検索 -->
                    <label>
                      <span>ユーザーIDで検索</span>
                      <div class="field-row">
                        <span class="field-prefix">U-</span>
                        <input 
                          type="text" 
                          bind:value={friendSearchUserId} 
                          placeholder="ユーザーIDを入力" 
                        />
                        <button 
                          type="button" 
                          class="status-action-button" 
                          on:click={searchFriendByUserId}
                          disabled={friendSearchUserIdLoading}
                        >
                          {friendSearchUserIdLoading ? '検索中...' : '検索'}
                        </button>
                      </div>
                    </label>

                    <div class="action-buttons">
                      <button 
                        type="button" 
                        class="info"
                        on:click={loadFriendRequestsList} 
                        disabled={!$status.running || friendRequestsLoading}
                      >
                        {friendRequestsLoading ? '取得中...' : 'フレンドリクエスト一覧'}
                      </button>
                      <button 
                        type="button" 
                        class="info"
                        on:click={loadBannedUsersList} 
                        disabled={!$status.running || bansLoading}
                      >
                        {bansLoading ? '取得中...' : 'BAN一覧'}
                      </button>
                      <button 
                        type="button" 
                        class="info"
                        on:click={loadFocusedSessionUsers} 
                        disabled={!$status.running || focusedSessionUsersLoading || !runtimeWorlds?.focusedSessionId}
                      >
                        {focusedSessionUsersLoading ? '取得中...' : 'フォーカスセッション内'}
                      </button>
                    </div>

                    {#if friendSearchResults.length > 0}
                      <div class="user-list">
                        <h3 style="margin-bottom: 0.75rem; font-size: 0.95rem; color: #c7cad3;">検索結果</h3>
                        {#each friendSearchResults as user}
                          <button
                            type="button"
                            class="user-card"
                            class:selected={selectedFriendUser?.id === user.id}
                            on:click={() => selectFriendUser(user)}
                          >
                            <div class="user-avatar">
                              {#if convertResoniteImageUrl(user.profile.iconUrl)}
                                <img src={convertResoniteImageUrl(user.profile.iconUrl)} alt={user.username || 'User'} />
                              {:else}
                                <div class="avatar-placeholder">?</div>
                              {/if}
                            </div>
                            <div class="user-info">
                              <strong>{user.username || 'Unknown'}</strong>
                              <span class="sub">{user.id}</span>
                            </div>
                          </button>
                        {/each}
                      </div>
                    {:else if bansList.length > 0}
                      <div class="user-list">
                        <h3 style="margin-bottom: 0.75rem; font-size: 0.95rem; color: #ff6b6b;">BAN一覧</h3>
                        {#each bansList as ban}
                          <button
                            type="button"
                            class="user-card ban-card"
                            class:selected={selectedBanEntry?.userId === ban.userId}
                            on:click={() => selectBanEntry(ban)}
                          >
                            <div class="user-avatar">
                              <div class="avatar-placeholder ban">🚫</div>
                            </div>
                            <div class="user-info">
                              <strong>{ban.username}</strong>
                              <span class="sub">{ban.userId}</span>
                              {#if ban.machineIds}
                                <span class="sub machine-id" title={ban.machineIds}>Machine: {ban.machineIds.substring(0, 16)}...</span>
                              {/if}
                            </div>
                          </button>
                        {/each}
                      </div>
                    {:else}
                      <p class="empty">検索結果またはBAN一覧が表示されます</p>
                    {/if}
                  </form>
                </div>
              </div>

              <!-- 右側: 選択したユーザーへの操作 -->
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>ユーザー操作</h2>
                </div>
                <div class="card status-card">
                  {#if selectedFriendUser}
                    <form class="status-form" on:submit|preventDefault={() => {}}>
                      <div class="selected-user-display">
                        <div class="user-card selected" style="cursor: default;">
                          <div class="user-avatar">
                            {#if convertResoniteImageUrl(selectedFriendUser.profile.iconUrl)}
                              <img src={convertResoniteImageUrl(selectedFriendUser.profile.iconUrl)} alt={selectedFriendUser.username || 'User'} />
                            {:else}
                              <div class="avatar-placeholder">?</div>
                            {/if}
                          </div>
                          <div class="user-info">
                            <strong>{selectedFriendUser.username || 'Unknown'}</strong>
                            <span class="sub">{selectedFriendUser.id}</span>
                          </div>
                        </div>
                      </div>

                      <div class="action-buttons" style="display: grid; grid-template-columns: repeat(4, 1fr); gap: 0.5rem;">
                        <button 
                          type="button" 
                          class="save"
                          on:click={acceptFriendRequestFromSelected}
                          disabled={
                            !$status.running || 
                            friendAcceptLoading || 
                            !friendRequests?.data?.some(username => 
                              username.toLowerCase() === selectedFriendUser?.username?.toLowerCase()
                            )
                          }
                        >
                          {friendAcceptLoading ? '承認中...' : 'フレンド申請を承認'}
                        </button>
                        <button 
                          type="button" 
                          class="info"
                          on:click={sendFriendRequestToSelected}
                          disabled={!$status.running || friendSendLoading}
                        >
                          {friendSendLoading ? '送信中...' : 'フレンド申請を送る'}
                        </button>
                        <button 
                          type="button" 
                          class="close"
                          on:click={removeFriendFromSelected}
                          disabled={!$status.running || friendRemoveLoading}
                        >
                          {friendRemoveLoading ? '解除中...' : 'フレンド解除'}
                        </button>
                        <button 
                          type="button" 
                          class="save"
                          on:click={inviteToFocusedSession}
                          disabled={!$status.running || friendInviteLoading || !runtimeWorlds?.focusedSessionId}
                        >
                          {friendInviteLoading ? '招待中...' : 'フォーカス中のセッションに招待'}
                        </button>
                      </div>

                      <div class="description-block">
                        <div class="description-header">
                          <label for="friend-message-text">メッセージ送信</label>
                          <button 
                            type="button" 
                            class="status-action-button"
                            on:click={sendMessageToSelected}
                            disabled={!$status.running || friendMessageLoading}
                          >
                            {friendMessageLoading ? '送信中...' : '送信'}
                          </button>
                        </div>
                        <textarea 
                          id="friend-message-text"
                          rows="3" 
                          bind:value={friendMessageText}
                          placeholder="メッセージを入力..."
                        ></textarea>
                      </div>
                    </form>
                  {:else if selectedBanEntry}
                    <form class="status-form" on:submit|preventDefault={() => {}}>
                      <div class="selected-user-display">
                        <div class="user-card ban-card selected" style="cursor: default;">
                          <div class="user-avatar">
                            <div class="avatar-placeholder ban">🚫</div>
                          </div>
                          <div class="user-info">
                            <strong>{selectedBanEntry.username}</strong>
                            <span class="sub">{selectedBanEntry.userId}</span>
                            {#if selectedBanEntry.machineIds}
                              <span class="sub machine-id" title={selectedBanEntry.machineIds}>Machine: {selectedBanEntry.machineIds}</span>
                            {/if}
                          </div>
                        </div>
                      </div>

                      <div class="action-buttons">
                        <button 
                          type="button" 
                          class="danger"
                          on:click={unbanSelectedUser}
                          disabled={!$status.running}
                        >
                          BAN解除
                        </button>
                      </div>
                      
                      <div style="padding: 0.75rem; background: rgba(255, 107, 107, 0.1); border: 1px solid rgba(255, 107, 107, 0.3); border-radius: 0.5rem; margin-top: 1rem;">
                        <p style="font-size: 0.85rem; color: #ff6b6b; margin: 0; line-height: 1.5;">
                          ⚠️ このユーザーはBANされています。BAN解除ボタンを押すと、このユーザーは再びサーバーに接続できるようになります。
                        </p>
                      </div>
                    </form>
                  {:else}
                    <p class="empty">左側のリストからユーザーまたはBANエントリを選択してください</p>
                  {/if}
                </div>
              </div>
            </div>
          </section>

          <section class="panel" class:active={activeTab === 'settings'}>
            <!-- コンフィグ読み込み・作成セクション -->
            <div class="config-create-section">
              <div class="config-controls">
                  <div class="config-create-button">
                    <button type="button" on:click={generateConfigFile} class="config-create-btn" disabled={configGenerationLoading}>
                      {configGenerationLoading ? '作成中...' : 'コンフィグファイルを作成'}
                    </button>
                  </div>
                  <div class="config-load-section">
                    <div class="field-row">
                      <div class="select-wrapper">
                        <select bind:value={selectedConfigToLoad} disabled={configLoadLoading || configDeleteLoading}>
                          <option value="">コンフィグファイルを選択してください</option>
                          {#each $configs as config}
                            <option value={config.path}>{config.name}</option>
                          {/each}
                        </select>
                      </div>
                      <button type="button" on:click={loadConfigFile} disabled={!selectedConfigToLoad || configLoadLoading || configDeleteLoading}>
                        {configLoadLoading ? '読み込み中...' : '読み込み'}
                      </button>
                      <button type="button" class="delete-button" on:click={deleteConfigFile} disabled={!selectedConfigToLoad || configLoadLoading || configDeleteLoading} title="削除">
                        {configDeleteLoading ? '削除中...' : '削除'}
                      </button>
                    </div>
                  </div>
                </div>
            </div>

            <div class="panel-grid two">
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>基本設定</h2>
                </div>
                <div class="card status-card">
                  <form class="status-form" on:submit|preventDefault={() => {}}>
                    <!-- 基本設定 -->
                    <label>
                      <span><span class="required">*</span>ファイル名 <small class="note">一部使用できない文字があります</small></span>
                      <div class="field-row">
                        <input type="text" bind:value={configName} placeholder="例: イベント用" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('name')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true">
                            <path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" />
                          </svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span><span class="required">*</span>ユーザー名</span>
                      <div class="field-row">
                        <input type="text" bind:value={configUsername} placeholder="Headlessのユーザー名" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('username')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span><span class="required">*</span>パスワード</span>
                      <div class="field-row">
                        <input type={showPassword ? 'text' : 'password'} bind:value={configPassword} placeholder="あなたのResoniteパスワード" />
                        <button type="button" class="refresh-config-button eye" class:active={showPassword} aria-pressed={showPassword} on:click={() => (showPassword = !showPassword)} title={showPassword ? '非表示' : '表示'} aria-label="表示切替">
                          <svg viewBox="0 0 24 24" class="refresh-icon" aria-hidden="true"><path d="M12 5c-7.633 0-10 7-10 7s2.367 7 10 7 10-7 10-7-2.367-7-10-7zm0 12a5 5 0 1 1 0-10 5 5 0 0 1 0 10zm0-8a3 3 0 1 0 .002 6.002A3 3 0 0 0 12 9z"/></svg>
                        </button>
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('password')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>コメント <small class="note">特に効果はありません</small></span>
                      <div class="field-row">
                        <input type="text" bind:value={configComment} placeholder="設定ファイルの説明" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('comment')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <!-- 詳細設定 -->

                    <label>
                      <span>更新レート（fps）</span>
                      <div class="field-row">
                        <input type="number" bind:value={configTickRate} min="1" max="120" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('tickRate')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>最大同時アセット転送数</span>
                      <div class="field-row">
                        <input type="number" bind:value={configMaxConcurrentAssetTransfers} min="1" max="1000" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('maxConcurrentAssetTransfers')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>ユーザー名オーバーライド <small class="note">表示名を書き換える</small></span>
                      <div class="field-row">
                        <input type="text" bind:value={configUsernameOverride} placeholder="表示用ユーザー名" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('usernameOverride')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>データフォルダ</span>
                      <div class="field-row">
                        <input type="text" bind:value={configDataFolder} placeholder="データ保存フォルダ" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('dataFolder')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>キャッシュフォルダ</span>
                      <div class="field-row">
                        <input type="text" bind:value={configCacheFolder} placeholder="キャッシュフォルダ" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('cacheFolder')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>ログフォルダ</span>
                      <div class="field-row">
                        <input type="text" bind:value={configLogsFolder} placeholder="ログ保存フォルダ" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('logsFolder')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>許可されたURLホスト</span>
                      <div class="field-row">
                        <input type="text" bind:value={configAllowedUrlHosts} placeholder="例: example.com,api.example.com" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('allowedUrlHosts')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>

                    <label>
                      <span>自動スポーンアイテム</span>
                      <div class="field-row">
                        <input type="text" bind:value={configAutoSpawnItems} placeholder="例: resrec:///U-User/R-Item1,resrec:///U-User/R-Item2" />
                        <button type="button" class="refresh-config-button" on:click={() => resetBasicField('autoSpawnItems')} title="リセット" aria-label="リセット">
                          <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                        </button>
                      </div>
                    </label>
                  </form>
                </div>
              </div>

              <div class="panel-column">
                <div class="panel-heading">
                  <h2>セッション設定</h2>
                </div>
                <div class="card status-card">
                  <!-- Session tab buttons -->
                  <div class="session-tab-bar">
                    {#each sessions as session}
                      <button
                        type="button"
                        class="session-tab"
                        class:active={activeSessionTab === session.id}
                        on:click={() => activeSessionTab = session.id}
                      >
                        セッション {session.id}
                        {#if sessions.length > 1}
                          <span
                            class="remove-session-btn"
                            role="button"
                            tabindex="0"
                            on:click|stopPropagation={() => removeSession(session.id)}
                            on:keydown={(e) => e.key === 'Enter' && removeSession(session.id)}
                            title="セッションを削除"
                          >
                            ×
                          </span>
                        {/if}
                      </button>
                    {/each}
                    <button type="button" class="add-session-btn" on:click={addSession} title="セッションを追加">
                      +
                    </button>
                  </div>

                  <!-- Session content -->
                  {#each sessions as session}
                    {#if activeSessionTab === session.id}
                      <form class="status-form">
                        <label>
                          <span>セッション名</span>
                          <div class="field-row">
                            <input type="text" bind:value={session.sessionName} placeholder="ワールド名を使用" />
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('sessionName')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>カスタムセッションID</span>
                          <div class="field-row">
                            <div class="field-row" style="gap:0.4rem; align-items:center;">
                              <input type="text" bind:value={session.customSessionIdPrefix} placeholder="U-userID" class="prefix" />
                              <button 
                                type="button" 
                                class="status-action-button" 
                                style="padding: 0.35rem 0.75rem; font-size: 0.85rem;"
                                on:click={autoFillUserid}
                                disabled={useridLoading}
                                title="ユーザー名からUserIDを自動取得"
                              >
                                {useridLoading ? '取得中...' : '自動入力'}
                              </button>
                              <span class="separator">:</span>
                              <input type="text" bind:value={session.customSessionIdSuffix} placeholder="空欄で無効" />
                            </div>
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('customSessionId')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>説明</span>
                          <div class="field-row">
                            <textarea bind:value={session.description} placeholder="セッションの説明" rows="2"></textarea>
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('description')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>最大ユーザー数</span>
                          <div class="field-row">
                            <input type="number" bind:value={session.maxUsers} min="1" max="100" />
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('maxUsers')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>アクセスレベル</span>
                          <div class="field-row">
                            <div class="select-wrapper narrow">
                              <select bind:value={session.accessLevel}>
                              <option value="Private">Private</option>
                              <option value="LAN">LAN</option>
                              <option value="Contacts">Contacts</option>
                              <option value="ContactsPlus">ContactsPlus</option>
                              <option value="RegisteredUsers">RegisteredUsers</option>
                              <option value="Anyone">Anyone</option>
                              </select>
                            </div>
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('accessLevel')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>ワールド <small class="note">レコードURLを入力</small></span>
                          <div class="field-row">
                            <button type="button" class="status-action-button" on:click={() => { session.showWorldSearch = !session.showWorldSearch; sessions = [...sessions]; }} aria-pressed={session.showWorldSearch}>
                              検索{session.showWorldSearch ? '▲' : '▼'}
                            </button>
                            <input type="text" bind:value={session.loadWorldURL} placeholder="例：resrec:///U-***/R-***" />
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('loadWorldURL')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>
                        {#if session.showWorldSearch}
                          <div class="world-search">
                            <div class="field-row" style="margin-top:0.5rem;">
                              <input type="text" bind:value={session.worldSearchTerm} placeholder="ワールド名で検索" />
                              <button type="button" on:click={async () => {
                                (session as any).worldSearchLoading = true; sessions = [...sessions];
                                try {
                                  const res = await getWorldSearch(session.worldSearchTerm.trim());
                                  (session as any).worldSearchResults = res.items;
                                  (session as any).worldSearchError = '';
                                } catch (e) {
                                  (session as any).worldSearchError = e instanceof Error ? e.message : '検索に失敗しました';
                                } finally {
                                  (session as any).worldSearchLoading = false; sessions = [...sessions];
                                }
                              }}>検索</button>
                            </div>
                            {#if session.worldSearchLoading}
                              <p class="info">検索中...</p>
                            {:else if session.worldSearchError}
                              <p class="feedback">{session.worldSearchError}</p>
                            {:else if session.worldSearchResults && session.worldSearchResults.length}
                              <div class="world-grid">
                                {#each session.worldSearchResults as item}
                                  <button
                                    type="button"
                                    class="world-card"
                                    on:click={() => { session.loadWorldURL = item.resoniteUrl; sessions = [...sessions]; }}
                                  >
                                    <div class="thumb" aria-hidden="true">
                                      {#if item.imageUrl}
                                        <img src={item.imageUrl} alt="" />
                                      {:else}
                                        <div class="thumb-placeholder">No Image</div>
                                      {/if}
                                    </div>
                                    <div class="meta">
                                      <div class="title-row">
                                        <strong class="title">{item.name}</strong>
                                      </div>
                                      <div class="sub">{item.resoniteUrl}</div>
                                    </div>
                                  </button>
                                {/each}
                              </div>
                            {/if}
                          </div>
                        {/if}

                        <label>
                          <span>ワールドプリセット <small class="note">レコードURL未入力時使用</small></span>
                          <div class="field-row">
                            <div class="select-wrapper narrow">
                              <select bind:value={session.loadWorldPresetName}>
                              <option value="Grid">Grid</option>
                              <option value="Platform">Platform</option>
                              <option value="Blank">Blank</option>
                              </select>
                            </div>
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('loadWorldPresetName')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>タグ <small class="note">カンマ（,）で区切ってください</small></span>
                          <div class="field-row">
                            <input type="text" bind:value={session.tags} placeholder="tag1,tag2,tag3" />
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('tags')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l-64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>個別ユーザー権限設定</span>
                          <div class="field-row">
                            <input type="text" bind:value={session.defaultUserRoles} placeholder="JSON形式で入力してください" />
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('defaultUserRoles')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>AFKキック時間（分） <small class="note">-1で無効</small></span>
                          <div class="field-row">
                            <input type="number" bind:value={session.awayKickMinutes} min="-1" placeholder="5" required />
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('awayKickMinutes')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>自動保存間隔（秒） <small class="note">-1で無効</small></span>
                          <div class="field-row">
                            <input type="number" bind:value={session.autosaveInterval} min="-1" />
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('autosaveInterval')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>セッションリセット間隔（秒） <small class="note">ユーザーがいなくなってから何秒でリセット（-1で無効）</small></span>
                          <div class="field-row">
                            <input type="number" bind:value={session.idleRestartInterval} min="-1" />
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('idleRestartInterval')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>セッションリストに表示しない</span>
                          <div class="field-row">
                            <button
                              type="button"
                              class={session.hideFromPublicListing ? 'status-action-button active' : 'status-action-button'}
                              aria-pressed={!!session.hideFromPublicListing}
                              on:click={() => {
                                session.hideFromPublicListing = !session.hideFromPublicListing;
                                sessions = [...sessions];
                              }}
                            >
                              {session.hideFromPublicListing ? 'オン' : 'オフ'}
                            </button>
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('hideFromPublicListing')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>自動復旧 <small class="note">落ちたのがセッションだけなら再起動される</small></span>
                          <div class="field-row">
                            <button
                              type="button"
                              class={session.autoRecover ? 'status-action-button active' : 'status-action-button'}
                              aria-pressed={session.autoRecover}
                              on:click={() => session.autoRecover = !session.autoRecover}
                            >
                              {session.autoRecover ? 'オン' : 'オフ'}
                            </button>
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('autoRecover')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>終了時に保存 <small class="note">権限必要</small></span>
                          <div class="field-row">
                            <button
                              type="button"
                              class={session.saveOnExit ? 'status-action-button active' : 'status-action-button'}
                              aria-pressed={session.saveOnExit}
                              on:click={() => session.saveOnExit = !session.saveOnExit}
                            >
                              {session.saveOnExit ? 'オン' : 'オフ'}
                            </button>
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('saveOnExit')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>

                        <label>
                          <span>自動スリープ <small class="note">人がいないとエコモードに</small></span>
                          <div class="field-row">
                            <button
                              type="button"
                              class={session.autoSleep ? 'status-action-button active' : 'status-action-button'}
                              aria-pressed={session.autoSleep}
                              on:click={() => session.autoSleep = !session.autoSleep}
                            >
                              {session.autoSleep ? 'オン' : 'オフ'}
                            </button>
                            <button type="button" class="refresh-config-button" on:click={() => resetCurrentSessionField('autoSleep')} title="リセット" aria-label="リセット">
                              <svg viewBox="0 -960 960 960" class="refresh-icon" aria-hidden="true"><path d="M482-160q-134 0-228-93t-94-227v-7l-64 64-56-56 160-160 160 160-56 56-64-64v7q0 100 70.5 170T482-240q26 0 51-6t49-18l60 60q-38 22-78 33t-82 11Zm278-161L600-481l56-56 64 64v-7q0-100-70.5-170T478-720q-26 0-51 6t-49 18l-60-60q38-22 78-33t82-11q134 0 228 93t94 227v7l64-64 56 56-160 160Z" /></svg>
                            </button>
                          </div>
                        </label>
                      </form>
                    {/if}
                  {/each}
                </div>
              </div>
            </div>

          </section>

          <!-- プレビューエリア（常時表示） -->
          <section class="panel" class:active={activeTab === 'settings'}>
            <div class="panel-heading">
              <h2>プレビュー</h2>
              <div class="preview-controls">
                {#if !isPreviewEditing}
                  <button type="button" class="edit-preview-btn" on:click={startPreviewEdit}>
                    編集
                  </button>
                {:else}
                  <button type="button" class="save-preview-btn" on:click={savePreviewEdit}>
                    保存
                  </button>
                  <button type="button" class="cancel-preview-btn" on:click={cancelPreviewEdit}>
                    キャンセル
                  </button>
                {/if}
              </div>
            </div>
            <div class="card status-card">
              {#if !isPreviewEditing}
                <pre class="config-preview">{maskPasswordInJson(configPreviewText) || '設定を入力すると、ここにプレビューが表示されます。'}</pre>
              {:else}
                <textarea 
                  class="config-preview-edit" 
                  bind:value={editedPreviewText}
                  bind:this={previewTextarea}
                  placeholder="JSONを編集してください..."
                  on:input={(e) => {
                    const target = e.target as HTMLTextAreaElement;
                    target.style.height = 'auto';
                    target.style.height = target.scrollHeight + 'px';
                  }}
                  on:focus={(e) => {
                    const target = e.target as HTMLTextAreaElement;
                    target.style.height = 'auto';
                    target.style.height = target.scrollHeight + 'px';
                  }}
                ></textarea>
                {#if previewEditError}
                  <div class="preview-error">{previewEditError}</div>
                {/if}
              {/if}
            </div>
          </section>

          <!-- 自動再起動設定タブ -->
          <section class="panel" class:active={activeTab === 'restart'}>
            <!-- 再起動ボタンセクション -->
            <div class="config-create-section">
              <div class="config-controls">
                <div class="config-create-button">
                  <button 
                    type="button" 
                    class="config-create-btn danger-button"
                    on:click={handleForceRestart}
                    disabled={forceRestartLoading}
                  >
                    {forceRestartLoading ? '実行中...' : '強制再起動'}
                  </button>
                </div>
                <div class="config-create-button">
                  <button 
                    type="button" 
                    class="config-create-btn"
                    on:click={handleManualRestart}
                    disabled={manualRestartLoading}
                  >
                    {manualRestartLoading ? '実行中...' : '手動再起動トリガー'}
                  </button>
                </div>
                <div class="config-create-button">
                  <button 
                    type="button" 
                    class="config-create-btn"
                    on:click={handleResetRestartConfig}
                  >
                    リセット
                  </button>
                </div>
                {#if restartSaveLoading}
                  <div style="display: flex; align-items: center; gap: 0.5rem; color: #61d1fa; font-size: 0.9rem;">
                    <div class="loader" style="width: 20px; height: 20px; border-width: 2px;"></div>
                    <span>保存中...</span>
                  </div>
                {/if}
              </div>
            </div>

            <div class="panel-grid two">
              <!-- 左カラム -->
              <div class="panel-column">
                <!-- 1️⃣ 再起動トリガー設定 -->
                <div class="panel-heading">
                  <h2>1️⃣ 再起動トリガー設定</h2>
                </div>
                <div class="card status-card">
                  {#if restartConfig}
                    <form class="status-form" on:submit|preventDefault={() => {}}>
                      <!-- 予定再起動 -->
                      <label style="border-bottom: 1px solid #2b2f35; padding-bottom: 0.5rem;">
                        <span style="font-size: 1rem; font-weight: 700;">📅 予定再起動</span>
                        <div class="field-row" style="gap: 0.5rem;">
                          <button
                            type="button"
                            class={restartConfig && restartConfig.triggers.scheduled.enabled ? 'status-action-button active' : 'status-action-button'}
                            on:click={() => { if (restartConfig) restartConfig.triggers.scheduled.enabled = !restartConfig.triggers.scheduled.enabled; }}
                          >
                            {restartConfig && restartConfig.triggers.scheduled.enabled ? 'オン' : 'オフ'}
                          </button>
                          <button
                            type="button"
                            class="status-action-button"
                            on:click={openAddScheduleModal}
                          >
                            + 予定を追加
                          </button>
                        </div>
                      </label>
                      
                      {#if restartConfig.triggers.scheduled.schedules.length === 0}
                        <div style="padding: 0.75rem; background: rgba(255, 255, 255, 0.03); border-radius: 0.5rem; margin: 0.5rem 0; text-align: center;">
                          <p style="font-size: 0.9rem; color: #a0a0a0; margin: 0;">
                            予定が登録されていません
                          </p>
                        </div>
                      {:else}
                        {#if !restartConfig.triggers.scheduled.enabled}
                          <div style="padding: 0.75rem; background: rgba(255, 165, 0, 0.1); border: 1px solid rgba(255, 165, 0, 0.3); border-radius: 0.5rem; margin: 0.5rem 0;">
                            <p style="font-size: 0.85rem; color: #ffa500; margin: 0;">
                              ⚠️ 予定再起動がオフです。予定を実行するには、予定再起動をオンにしてください。
                            </p>
                          </div>
                        {/if}
                        <div style="display: flex; flex-direction: column; gap: 0.75rem; margin-top: 0.75rem;">
                          {#each restartConfig.triggers.scheduled.schedules as schedule, index (schedule.id)}
                            <div style="padding: 0.75rem; background: rgba(255, 255, 255, 0.03); border-radius: 0.5rem; border: 1px solid {schedule.enabled ? '#61d1fa' : '#2b2f35'};">
                              <!-- ヘッダー行 -->
                              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
                                <span style="font-weight: 600; color: {schedule.enabled ? '#61d1fa' : '#a0a0a0'};">
                                  予定 #{index + 1}
                                </span>
                                <div style="display: flex; gap: 0.5rem; align-items: center;">
                                  <button
                                    type="button"
                                    class={schedule.enabled ? 'status-action-button active' : 'status-action-button'}
                                    style="font-size: 0.8rem; padding: 0.25rem 0.5rem;"
                                    on:click={() => toggleScheduleEnabled(schedule.id)}
                                  >
                                    {schedule.enabled ? 'ON' : 'OFF'}
                                  </button>
                                  <button
                                    type="button"
                                    class="status-action-button"
                                    style="font-size: 0.8rem; padding: 0.25rem 0.5rem;"
                                    on:click={() => openEditScheduleModal(schedule)}
                                  >
                                    編集
                                  </button>
                                  <button
                                    type="button"
                                    class="status-action-button"
                                    style="font-size: 0.8rem; padding: 0.25rem 0.5rem; background: #ff6b6b; border-color: #ff6b6b; color: #ffffff;"
                                    on:click={() => deleteSchedule(schedule.id)}
                                  >
                                    削除
                                  </button>
                                </div>
                              </div>
                              
                              <!-- 内容 -->
                              <div style="font-size: 0.85rem; color: #d0d0d0; display: flex; flex-direction: column; gap: 0.25rem;">
                                <div>
                                  <span style="color: #a0a0a0;">タイプ:</span>
                                  {#if schedule.type === 'once' && schedule.specificDate}
                                    指定日時 - {schedule.specificDate.year}/{String(schedule.specificDate.month).padStart(2, '0')}/{String(schedule.specificDate.day).padStart(2, '0')} {String(schedule.specificDate.hour).padStart(2, '0')}:{String(schedule.specificDate.minute).padStart(2, '0')}
                                  {:else if schedule.type === 'weekly' && schedule.weeklyDay !== undefined && schedule.weeklyTime}
                                    毎週 - {['日', '月', '火', '水', '木', '金', '土'][schedule.weeklyDay]}曜日 {String(schedule.weeklyTime.hour).padStart(2, '0')}:{String(schedule.weeklyTime.minute).padStart(2, '0')}
                                  {:else if schedule.type === 'daily' && schedule.dailyTime}
                                    毎日 - {String(schedule.dailyTime.hour).padStart(2, '0')}:{String(schedule.dailyTime.minute).padStart(2, '0')}
                                  {/if}
                                </div>
                                <div>
                                  <span style="color: #a0a0a0;">起動コンフィグ:</span>
                                  {schedule.configFile === '__previous__' ? '直前のコンフィグを使用' : schedule.configFile}
                                </div>
                              </div>
                            </div>
                          {/each}
                        </div>
                      {/if}
                      
                      <!-- 高負荷時再起動 -->
                      <label style="border-bottom: 1px solid #2b2f35; padding-bottom: 0.5rem; margin-top: 1rem;">
                        <span style="font-size: 1rem; font-weight: 700;">⚡ 高負荷時再起動</span>
                        <div class="field-row">
                          <button
                            type="button"
                            class={restartConfig && restartConfig.triggers.highLoad.enabled ? 'status-action-button active' : 'status-action-button'}
                            on:click={() => { if (restartConfig) restartConfig.triggers.highLoad.enabled = !restartConfig.triggers.highLoad.enabled; }}
                          >
                            {restartConfig && restartConfig.triggers.highLoad.enabled ? 'オン' : 'オフ'}
                          </button>
                        </div>
                      </label>
                      
                      {#if restartConfig.triggers.highLoad.enabled}
                        <div class="sub-settings">
                        <label>
                          <span>CPU閾値</span>
                          <div class="field-row">
                            <input 
                              type="number" 
                              min="10" 
                              max="100"
                              bind:value={restartConfig.triggers.highLoad.cpuThreshold}
                              placeholder="80"
                            />
                            <span style="color: #a0a0a0; font-size: 0.9rem;">%</span>
                          </div>
                        </label>
                        
                        <label>
                          <span>メモリ閾値</span>
                          <div class="field-row">
                            <input 
                              type="number" 
                              min="10" 
                              max="100"
                              bind:value={restartConfig.triggers.highLoad.memoryThreshold}
                              placeholder="80"
                            />
                            <span style="color: #a0a0a0; font-size: 0.9rem;">%</span>
                          </div>
                        </label>
                        
                        <label>
                          <span>継続時間</span>
                          <div class="field-row">
                            <input 
                              type="number" 
                              min="1" 
                              max="120"
                              bind:value={restartConfig.triggers.highLoad.durationMinutes}
                              placeholder="10"
                            />
                            <span style="color: #a0a0a0; font-size: 0.9rem;">分</span>
                          </div>
                        </label>
                        </div>
                      {/if}
                    </form>
                  {:else}
                    <p class="empty">設定を読み込み中...</p>
                  {/if}
                </div>

                <!-- 📊 ステータス表示 -->
                <div class="panel-heading">
                  <h2>📊 ステータス表示</h2>
                </div>
                <div class="card status-card">
                  <div class="status-display-list">
                    <div class="status-display-item">
                      <span class="status-display-label">次回の予定再起動</span>
                        <div class="field-row">
                        <div class="status-display-value">
                          {#if restartStatus?.nextScheduledRestart.datetime}
                            {new Date(restartStatus.nextScheduledRestart.datetime).toLocaleString('ja-JP')}
                            {#if restartStatus.nextScheduledRestart.configFile}
                              ({restartStatus.nextScheduledRestart.configFile === '__previous__' ? '直前のコンフィグを使用' : restartStatus.nextScheduledRestart.configFile})
                            {/if}
                          {:else}
                            未設定
                          {/if}
                        </div>
                      </div>
                    </div>

                    {#if restartStatus?.scheduledRestartPreparing?.preparing}
                      <div class="status-display-item" style="background: rgba(97, 209, 250, 0.1); border: 1px solid rgba(97, 209, 250, 0.3); border-radius: 0.5rem; padding: 0.75rem;">
                        <span class="status-display-label" style="color: #61d1fa;">🔔 予定再起動準備中</span>
                          <div class="field-row">
                          <div class="status-display-value" style="color: #61d1fa;">
                            {#if restartStatus.scheduledRestartPreparing.scheduledTime}
                              {new Date(restartStatus.scheduledRestartPreparing.scheduledTime).toLocaleString('ja-JP')} 予定
                              {#if restartStatus.scheduledRestartPreparing.configFile}
                                ({restartStatus.scheduledRestartPreparing.configFile === '__previous__' ? '直前のコンフィグを使用' : restartStatus.scheduledRestartPreparing.configFile})
                              {/if}
                            {/if}
                          </div>
                        </div>
                        <p style="font-size: 0.8rem; color: #a0a0a0; margin: 0.5rem 0 0 0;">
                          高負荷トリガーは無効化されています。強制再起動ボタンは使用可能です。
                          </p>
                        </div>
                      {/if}
                    
                    <div class="status-display-item">
                      <span class="status-display-label">現在の稼働時間</span>
                      <div class="field-row">
                        <div class="status-display-value">
                          {#if restartStatus && restartStatus.currentUptime > 0}
                            {Math.floor(restartStatus.currentUptime / 3600)}時間
                            {Math.floor((restartStatus.currentUptime % 3600) / 60)}分
                  {:else}
                            -
                  {/if}
                        </div>
                      </div>
                </div>

                    <div class="status-display-item">
                      <span class="status-display-label">現在のCPU使用率</span>
                      <div class="field-row">
                        <div class="status-display-value">
                          {#if $metrics}
                            {$metrics.cpu.usage.toFixed(1)}%
                          {:else}
                            -
                          {/if}
                        </div>
                      </div>
                    </div>
                    
                    <div class="status-display-item">
                      <span class="status-display-label">現在のメモリ使用率</span>
                      <div class="field-row">
                        <div class="status-display-value">
                          {#if $metrics}
                            {$metrics.memory.usage.toFixed(1)}%
                          {:else}
                            -
                          {/if}
                        </div>
                      </div>
                    </div>
                    
                    <div class="status-display-item">
                      <span class="status-display-label">現在の合計ユーザー数</span>
                      <div class="field-row">
                        <div class="status-display-value">
                          {#if runtimeWorlds}
                            {runtimeWorlds.data.reduce((sum: number, w: RuntimeWorldEntry) => sum + Math.max(0, (w.currentUsers || 0) - 1), 0)}人
                          {:else}
                            -
                          {/if}
                        </div>
                      </div>
                    </div>
                    
                    <div class="status-display-item">
                      <span class="status-display-label">最後の再起動</span>
                      <div class="field-row">
                        <div class="status-display-value">
                          {#if restartStatus?.lastRestart.timestamp}
                            {new Date(restartStatus.lastRestart.timestamp).toLocaleString('ja-JP')}
                            {#if restartStatus.lastRestart.trigger}
                              (トリガー: {restartStatus.lastRestart.trigger})
                            {/if}
                          {:else}
                            -
                          {/if}
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 右カラム -->
              <div class="panel-column">
                <!-- 2️⃣ 再起動前アクション設定 -->
                <div class="panel-heading">
                  <h2>2️⃣ 再起動前アクション設定（全トリガー対象）</h2>
                </div>
                <div class="card status-card">
                  {#if restartConfig}
                    <form class="status-form" on:submit|preventDefault={() => {}}>
                      <!-- 待機制御 -->
                      <label>
                        <span>再起動待機</span>
                        <div class="field-row">
                          <input 
                            type="number" 
                            min="0" 
                            max="120"
                            bind:value={restartConfig.preRestartActions.waitControl.waitForZeroUsers}
                            placeholder="5"
                          />
                          <span style="color: #a0a0a0; font-size: 0.9rem;">分</span>
                        </div>
                      </label>
                      
                      <label>
                        <span>強制実行タイムアウト</span>
                        <div class="field-row">
                          <input 
                            type="number" 
                            min="1" 
                            max="1440"
                            bind:value={restartConfig.preRestartActions.waitControl.forceRestartTimeout}
                            on:input={(e) => {
                              const target = e.target as HTMLInputElement;
                              const value = Number(target.value);
                              
                              // アクション実行タイミングが強制実行タイムアウトより大きい場合は自動調整
                              if (restartConfig.preRestartActions.waitControl.actionTiming > value) {
                                restartConfig.preRestartActions.waitControl.actionTiming = value;
                              }
                            }}
                            placeholder="120"
                          />
                          <span style="color: #a0a0a0; font-size: 0.9rem;">分</span>
                        </div>
                      </label>
                      
                      <label>
                        <span>アクション実行タイミング</span>
                        <div class="field-row">
                          <input 
                            type="number" 
                            min="1" 
                            max={restartConfig.preRestartActions.waitControl.forceRestartTimeout}
                            bind:value={restartConfig.preRestartActions.waitControl.actionTiming}
                            on:input={(e) => {
                              const target = e.target as HTMLInputElement;
                              const value = Number(target.value);
                              const maxValue = restartConfig.preRestartActions.waitControl.forceRestartTimeout;
                              
                              // 強制実行タイムアウトより大きい場合は補正
                              if (value > maxValue) {
                                restartConfig.preRestartActions.waitControl.actionTiming = maxValue;
                              }
                              // 1未満の場合は1に補正
                              if (value < 1) {
                                restartConfig.preRestartActions.waitControl.actionTiming = 1;
                              }
                            }}
                            placeholder="2"
                          />
                          <span style="color: #a0a0a0; font-size: 0.9rem;">分前</span>
                        </div>
                        <small style="color: #86888b; font-size: 0.85rem;">
                          最大: {restartConfig.preRestartActions.waitControl.forceRestartTimeout}分（強制実行タイムアウト）
                        </small>
                      </label>
                      
                      <!-- チャットメッセージ -->
                      <label>
                        <span>💬 チャットメッセージ送信</span>
                        <div class="field-row">
                          <button
                            type="button"
                            class={restartConfig && restartConfig.preRestartActions.chatMessage.enabled ? 'status-action-button active' : 'status-action-button'}
                            on:click={() => { if (restartConfig) restartConfig.preRestartActions.chatMessage.enabled = !restartConfig.preRestartActions.chatMessage.enabled; }}
                          >
                            {restartConfig && restartConfig.preRestartActions.chatMessage.enabled ? 'オン' : 'オフ'}
                          </button>
                        </div>
                      </label>
                      
                      {#if restartConfig.preRestartActions.chatMessage.enabled}
                        <div class="sub-settings">
                          <label class="full-width-field">
                          <span>メッセージ内容</span>
                          <div class="field-row">
                            <textarea 
                              bind:value={restartConfig.preRestartActions.chatMessage.message}
                              placeholder="🔄 サーバーが間もなく再起動します。"
                              rows="2"
                              style="flex: 1; padding: 0.5rem; background: rgba(17, 21, 29, 0.6); border: 1px solid #2b2f35; border-radius: 0.5rem; color: #e1e1e0; resize: vertical;"
                            ></textarea>
                          </div>
                        </label>
                        </div>
                      {/if}
                      
                      <!-- アイテムスポーン警告 -->
                      <label>
                        <span>📦 アイテムスポーン警告</span>
                        <div class="field-row">
                          <button
                            type="button"
                            class={restartConfig && restartConfig.preRestartActions.itemSpawn.enabled ? 'status-action-button active' : 'status-action-button'}
                            on:click={() => { if (restartConfig) restartConfig.preRestartActions.itemSpawn.enabled = !restartConfig.preRestartActions.itemSpawn.enabled; }}
                          >
                            {restartConfig && restartConfig.preRestartActions.itemSpawn.enabled ? 'オン' : 'オフ'}
                          </button>
                        </div>
                      </label>
                      
                      {#if restartConfig.preRestartActions.itemSpawn.enabled}
                        <div class="sub-settings">
                        <label>
                          <span>アイテム種類</span>
                            <div class="field-row" style="gap: 0.5rem;">
                              <div class="select-wrapper" style="min-width: 200px;">
                                <select 
                                  bind:value={restartConfig.preRestartActions.itemSpawn.itemType}
                                  on:change={(e) => {
                                    if (!restartConfig) return;
                                    const target = e.target as HTMLSelectElement;
                                    if (target.value === 'とらぞセッション閉店アナウンス') {
                                      restartConfig.preRestartActions.itemSpawn.itemUrl = 'resrec:///U-MarkN/R-d347f78c-d30a-4664-9b6f-2984078880a8';
                                    } else if (target.value === 'テキスト読み上げ') {
                                      restartConfig.preRestartActions.itemSpawn.itemUrl = 'resrec:///U-MarkN/R-5eacacd2-3163-42bd-95ee-bb6810c993e1';
                                    }
                                  }}
                                >
                                  <option value="とらぞセッション閉店アナウンス">とらぞセッション閉店アナウンス</option>
                                  <option value="テキスト読み上げ">テキスト読み上げ</option>
                            </select>
                              </div>
                              <span style="color: #a0a0a0; font-size: 0.85rem; white-space: nowrap;">URL</span>
                              <input 
                                type="text"
                                bind:value={restartConfig.preRestartActions.itemSpawn.itemUrl}
                                placeholder="resrec:///U-MarkN/R-..."
                                style="flex: 1;"
                              />
                          </div>
                        </label>
                        
                          {#if restartConfig.preRestartActions.itemSpawn.itemType !== 'とらぞセッション閉店アナウンス'}
                            <label class="full-width-field">
                          <span>メッセージ内容</span>
                          <div class="field-row">
                            <textarea 
                              bind:value={restartConfig.preRestartActions.itemSpawn.message}
                              placeholder="🔄 サーバー再起動通知"
                              rows="2"
                              style="flex: 1; padding: 0.5rem; background: rgba(17, 21, 29, 0.6); border: 1px solid #2b2f35; border-radius: 0.5rem; color: #e1e1e0; resize: vertical;"
                            ></textarea>
                          </div>
                        </label>
                          {/if}
                        </div>
                      {/if}
                      
                      <!-- セッション設定変更 -->
                      <label>
                        <span>🚫 アクセスレベル→プライベート</span>
                        <div class="field-row">
                          <button
                            type="button"
                            class={restartConfig && restartConfig.preRestartActions.sessionChanges.setPrivate ? 'status-action-button active' : 'status-action-button'}
                            on:click={() => { if (restartConfig) restartConfig.preRestartActions.sessionChanges.setPrivate = !restartConfig.preRestartActions.sessionChanges.setPrivate; }}
                          >
                            {restartConfig && restartConfig.preRestartActions.sessionChanges.setPrivate ? 'オン' : 'オフ'}
                          </button>
                        </div>
                      </label>
                      
                      <label>
                        <span>👥 MaxUser→1</span>
                        <div class="field-row">
                          <button
                            type="button"
                            class={restartConfig && restartConfig.preRestartActions.sessionChanges.setMaxUserToOne ? 'status-action-button active' : 'status-action-button'}
                            on:click={() => { if (restartConfig) restartConfig.preRestartActions.sessionChanges.setMaxUserToOne = !restartConfig.preRestartActions.sessionChanges.setMaxUserToOne; }}
                          >
                            {restartConfig && restartConfig.preRestartActions.sessionChanges.setMaxUserToOne ? 'オン' : 'オフ'}
                          </button>
                        </div>
                      </label>
                      
                      <label>
                        <span>📝 セッション名変更</span>
                        <div class="field-row">
                          <button
                            type="button"
                            class={restartConfig && restartConfig.preRestartActions.sessionChanges.changeSessionName.enabled ? 'status-action-button active' : 'status-action-button'}
                            on:click={() => { if (restartConfig) restartConfig.preRestartActions.sessionChanges.changeSessionName.enabled = !restartConfig.preRestartActions.sessionChanges.changeSessionName.enabled; }}
                          >
                            {restartConfig && restartConfig.preRestartActions.sessionChanges.changeSessionName.enabled ? 'オン' : 'オフ'}
                          </button>
                        </div>
                      </label>
                      
                      {#if restartConfig.preRestartActions.sessionChanges.changeSessionName.enabled}
                        <div class="sub-settings">
                        <label>
                          <span>変更後の名前</span>
                          <div class="field-row">
                            <input 
                              type="text" 
                              bind:value={restartConfig.preRestartActions.sessionChanges.changeSessionName.newName}
                              placeholder="[再起動します]"
                            />
                          </div>
                        </label>
                </div>
                      {/if}
                      
                      <!-- リトライ設定 -->
                      <label>
                        <span>🔄 リトライ</span>
                        <div class="field-row" style="display: flex; align-items: center; gap: 0.5rem;">
                          <span style="color: #e9f9ff; font-size: 0.9rem;">回数</span>
                          <input 
                            type="number" 
                            min="0" 
                            max="10"
                            bind:value={restartConfig.failsafe.retryCount}
                            placeholder="3"
                            style="width: 80px;"
                          />
                          <span style="color: #a0a0a0; font-size: 0.9rem;">回</span>
                          <span style="color: #e9f9ff; font-size: 0.9rem; margin-left: 0.5rem;">間隔</span>
                          <input 
                            type="number" 
                            min="1" 
                            max="300"
                            bind:value={restartConfig.failsafe.retryIntervalSeconds}
                            placeholder="30"
                            style="width: 80px;"
                          />
                          <span style="color: #a0a0a0; font-size: 0.9rem;">秒</span>
                        </div>
                      </label>
                    </form>
                  {:else}
                    <p class="empty">設定を読み込み中...</p>
                  {/if}
                </div>
                </div>
                        </div>
          </section>
                      </div>
                          {/if}
    </main>
                    </div>
                    
  {#if $status.running && pendingStartup && showStartupMessage}
    <div class="startup-overlay">
      <div class="loader"></div>
      <p>セッション情報を読み込んでいます...</p>
                        </div>
                          {/if}

  <div class="toast-container" aria-live="polite">
    {#each $notifications as notification (notification.id)}
      <div class={`toast ${notification.type}`}>
        <span>{notification.message}</span>
                        </div>
    {/each}
                    </div>
                    
  <!-- 予定再起動編集モーダル -->
  {#if scheduledRestartModalOpen && editingSchedule}
    <div class="modal-overlay" on:click={closeScheduleModal}>
      <div class="modal-content" on:click|stopPropagation>
        <div class="modal-header">
          <h2>{editingScheduleId ? '予定を編集' : '予定を追加'}</h2>
          <button type="button" class="modal-close" on:click={closeScheduleModal}>×</button>
            </div>

        <div class="modal-body">
          <form on:submit|preventDefault={saveSchedule}>
            <!-- タイプ選択 -->
            <label class="modal-label">
              <span>再起動タイプ</span>
              <div style="display: flex; gap: 0.5rem; margin-top: 0.5rem;">
                  <button 
                    type="button" 
                  class={editingSchedule.type === 'once' ? 'schedule-type-button active' : 'schedule-type-button'}
                  on:click={() => editingSchedule.type = 'once'}
                  >
                  指定日時
                  </button>
                  <button 
                    type="button" 
                  class={editingSchedule.type === 'weekly' ? 'schedule-type-button active' : 'schedule-type-button'}
                  on:click={() => editingSchedule.type = 'weekly'}
                >
                  毎週
                </button>
                <button
                  type="button"
                  class={editingSchedule.type === 'daily' ? 'schedule-type-button active' : 'schedule-type-button'}
                  on:click={() => editingSchedule.type = 'daily'}
                >
                  毎日
                  </button>
                </div>
            </label>

            <!-- 指定日時の入力 -->
            {#if editingSchedule.type === 'once'}
              <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(100px, 1fr)); gap: 0.5rem; margin-top: 1rem;">
                <label class="modal-label">
                  <span>年</span>
                  <input type="number" bind:value={editingSchedule.specificDate.year} min="2024" max="2100" required />
                </label>
                <label class="modal-label">
                  <span>月</span>
                  <input type="number" bind:value={editingSchedule.specificDate.month} min="1" max="12" required />
                </label>
                <label class="modal-label">
                  <span>日</span>
                  <input type="number" bind:value={editingSchedule.specificDate.day} min="1" max="31" required />
                </label>
                <label class="modal-label">
                  <span>時</span>
                  <input type="number" bind:value={editingSchedule.specificDate.hour} min="0" max="23" required />
                </label>
                <label class="modal-label">
                  <span>分</span>
                  <input type="number" bind:value={editingSchedule.specificDate.minute} min="0" max="59" required />
                </label>
        </div>
      {/if}

            <!-- 毎週の入力 -->
            {#if editingSchedule.type === 'weekly'}
              <div style="display: grid; grid-template-columns: 2fr 1fr 1fr; gap: 0.5rem; margin-top: 1rem;">
                <label class="modal-label">
                  <span>曜日</span>
                  <select bind:value={editingSchedule.weeklyDay} required>
                    <option value={0}>日曜日</option>
                    <option value={1}>月曜日</option>
                    <option value={2}>火曜日</option>
                    <option value={3}>水曜日</option>
                    <option value={4}>木曜日</option>
                    <option value={5}>金曜日</option>
                    <option value={6}>土曜日</option>
                  </select>
                </label>
                <label class="modal-label">
                  <span>時</span>
                  <input type="number" bind:value={editingSchedule.weeklyTime.hour} min="0" max="23" required />
                </label>
                <label class="modal-label">
                  <span>分</span>
                  <input type="number" bind:value={editingSchedule.weeklyTime.minute} min="0" max="59" required />
                </label>
    </div>
  {/if}

            <!-- 毎日の入力 -->
            {#if editingSchedule.type === 'daily'}
              <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; margin-top: 1rem;">
                <label class="modal-label">
                  <span>時</span>
                  <input type="number" bind:value={editingSchedule.dailyTime.hour} min="0" max="23" required />
                </label>
                <label class="modal-label">
                  <span>分</span>
                  <input type="number" bind:value={editingSchedule.dailyTime.minute} min="0" max="59" required />
                </label>
      </div>
            {/if}

            <!-- コンフィグファイル選択 -->
            <label class="modal-label" style="margin-top: 1rem;">
              <span>起動コンフィグファイル</span>
              <select bind:value={editingSchedule.configFile} required>
                <option value="__previous__">直前のコンフィグを使用</option>
                {#if $configs && $configs.length > 0}
                  {#each $configs as config}
                    <option value={config.name}>{config.name}</option>
    {/each}
                {:else}
                  <option value="default.json">default.json</option>
                {/if}
              </select>
            </label>

            <div class="modal-actions">
              <button type="button" class="modal-cancel-btn" on:click={closeScheduleModal}>
                キャンセル
              </button>
              <button type="submit" class="modal-save-btn">
                保存
              </button>
  </div>
          </form>
        </div>
      </div>
    </div>
  {/if}
</div>
{/if}

<style>
  :global(body) {
    margin: 0;
    background: #11151d;
    color: #e1e1e0;
    font-family: 'Noto Sans JP', system-ui, sans-serif;
  }

  :global(*),
  :global(*::before),
  :global(*::after) {
    box-sizing: border-box;
  }

  button,
  input,
  select,
  textarea {
    font-family: inherit;
  }

  button {
    cursor: pointer;
  }

  button:disabled {
    cursor: not-allowed;
    opacity: 0.5;
  }

  .layout {
    min-height: 100vh;
    display: grid;
    grid-template-rows: auto 1fr;
  }

  .topbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1.25rem 2rem;
    background: #11151d;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
    position: sticky;
    top: 0;
    z-index: 10;
    flex-wrap: wrap;
    gap: 1rem;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .brand h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.25rem;
  }

  .account-label {
    margin: 0;
    font-size: 0.82rem;
    color: rgba(225, 225, 224, 0.65);
  }

  .logo {
    width: 3rem;
    height: 3rem;
    border-radius: 0.75rem;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background-color: #2a4a72;
    background-image:
      linear-gradient(45deg, rgba(74, 126, 179, 0.55) 25%, transparent 25%),
      linear-gradient(-45deg, rgba(74, 126, 179, 0.55) 25%, transparent 25%),
      linear-gradient(45deg, transparent 75%, rgba(16, 58, 96, 0.8) 75%),
      linear-gradient(-45deg, transparent 75%, rgba(16, 58, 96, 0.8) 75%);
    background-size: 16px 16px;
    background-position: 0 0, 0 8px, 8px -8px, -8px 0;
    color: #ecf3ff;
    font-weight: 700;
    letter-spacing: 1px;
  }

  .topbar-controls {
    display: flex;
    gap: 1.5rem;
    align-items: flex-end;
    flex-wrap: wrap;
  }

  .topbar-controls .field {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    font-size: 0.85rem;
  }

  .field-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .note {
    color: rgba(225, 225, 224, 0.65);
    font-size: 0.85em;
    font-weight: normal;
  }

  .required {
    color: #ff6b6b;
    font-weight: bold;
    margin-right: 0.25rem;
  }

  .refresh-config-button {
    background: #2b2f35;
    border: none;
    border-radius: 0.5rem;
    padding: 0.4rem;
    color: #61d1fa;
    cursor: pointer;
    transition: background 0.15s ease, transform 0.15s ease;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .refresh-config-button:hover {
    background: #34404c;
    transform: translateY(-1px);
  }

  .refresh-config-button .refresh-icon {
    width: 16px;
    height: 16px;
    fill: currentColor;
  }

  .refresh-config-button.eye.active {
    background: #61d1fa;
    color: #11151d;
  }

  .topbar-controls select {
    min-width: 220px;
    padding: 0.55rem 0.9rem;
    border-radius: 0.75rem;
    border: none;
    background: #2b2f35;
    color: inherit;
    box-shadow: 0 8px 18px rgba(0, 0, 0, 0.25) inset;
  }

  .select-wrapper {
    position: relative;
    display: inline-flex;
    width: 100%;
  }

  .select-wrapper.narrow {
    max-width: 240px;
  }

  .select-wrapper select {
    width: 100%;
    padding-right: 2rem;
    appearance: none;
    background: #2b2f35;
    color: #f5f5f5;
    border: none;
    border-radius: 0.65rem;
  }

  .select-wrapper::after {
    content: '\25BC';
    position: absolute;
    right: 0.7rem;
    top: 50%;
    transform: translateY(-50%);
    color: #61d1fa;
    pointer-events: none;
    font-size: 0.65rem;
  }

  .resource-capsule {
    background: rgba(43, 59, 71, 0.85);
    border-radius: 0.65rem;
    padding: 0.35rem 0.6rem;
    display: grid;
    gap: 0.25rem;
    font-size: 0.75rem;
    min-width: 108px;
    min-height: 38px;
    text-align: center;
    color: #e1f6ff;
    box-shadow: 0 8px 18px rgba(0, 0, 0, 0.2);
  }

  .resource-capsule.combined {
    min-width: 120px;
    min-height: 50px;
    padding: 0.4rem 0.7rem;
  }

  .resource-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.5rem;
  }

  .resource-capsule span {
    color: #a9c8e0;
    font-weight: 600;
  }

  .resource-capsule strong {
    font-size: 0.95rem;
  }

  .status-indicators {
    display: flex;
    align-items: stretch;
    gap: 0.9rem;
  }

  .status-indicators > * {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-height: 54px;
    padding: 0 1.45rem;
    border-radius: 0.85rem;
    box-shadow: 0 8px 20px rgba(0, 0, 0, 0.25);
  }

  .status-indicators .online,
  .status-indicators .offline {
    background: rgba(17, 21, 29, 0.7);
    font-weight: 600;
    gap: 0.35rem;
  }

  .status-info {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    background: rgba(17, 21, 29, 0.7);
    padding: 0.6rem 1rem;
    border-radius: 0.85rem;
    box-shadow: 0 8px 20px rgba(0, 0, 0, 0.25);
    min-height: 54px;
    justify-content: center;
  }

  .status-info .backend-status,
  .status-info .online,
  .status-info .offline {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.85rem;
    font-weight: 600;
    background: none;
    box-shadow: none;
    padding: 0;
    min-height: auto;
    color: #cceaff;
  }

  .status-info .backend-status .dot,
  .status-info .online .dot,
  .status-info .offline .dot {
    width: 0.75rem;
    height: 0.75rem;
    border-radius: 999px;
  }

  .status-info .backend-status:not(.offline) .dot,
  .status-info .online .dot {
    background: #59eb5c;
  }

  .status-info .backend-status.offline .dot,
  .status-info .offline .dot {
    background: #ff7676;
  }

  .status-info .backend-status.offline,
  .status-info .offline {
    color: #ffdcdc;
  }

  .status-indicators .dot {
    width: 0.75rem;
    height: 0.75rem;
    border-radius: 999px;
  }

  .status-indicators .online .dot {
    background: #59eb5c;
  }

  .status-indicators .offline .dot {
    background: #ff7676;
  }

  .status-indicators .online,
  .status-indicators .offline {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-weight: 600;
  }

  .status-indicators button {
    background: #61d1fa;
    color: #000000;
    font-weight: 700;
    font-size: 1.1rem;
    letter-spacing: 0.03em;
    min-width: 140px;
    transition: transform 0.2s ease, filter 0.2s ease;
    border: none;
  }

  .status-indicators button:hover:enabled {
    transform: translateY(-1px);
    filter: brightness(1.08);
  }

  .status-indicators button.danger {
    background: #ff7676;
    color: #11151d;
  }

  .status-indicators button.danger:hover:enabled {
    filter: brightness(1.05);
  }

  .content {
    display: grid;
    grid-template-columns: 380px 1fr;
    min-height: 0;
  }

  .sidebar {
    background: #1a2a36;
    padding: 1.75rem;
    border-right: 1px solid rgba(255, 255, 255, 0.08);
    display: grid;
    gap: 1.75rem;
    align-content: start;
  }

  .sidebar section {
    display: grid;
    gap: 0.9rem;
  }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .section-header .header-actions {
    margin-left: auto;
    display: inline-flex;
    gap: 0.4rem;
    align-items: center;
  }

  .sidebar h2 {
    font-size: 1rem;
    color: #61d1fa;
    margin: 0;
    font-weight: 700;
  }

  .session-card {
    display: grid;
    gap: 0.75rem;
  }

  .session-list {
    display: grid;
    gap: 0.75rem;
  }

  .session {
    background: #2b2f35;
    padding: 0.75rem;
    border-radius: 0.75rem;
    border: 4px solid transparent;
    display: flex;
    align-items: center;
    font-size: 0.85rem;
    color: #e1f6ff;
    box-shadow: 0 8px 18px rgba(0, 0, 0, 0.2);
    transition: background 0.15s ease, transform 0.15s ease;
  }

  .session-body {
    display: grid;
    gap: 0.25rem;
    width: 100%;
  }

  .session-name {
    font-size: 1rem;
    font-weight: 700;
    letter-spacing: 0.01em;
    color: #f5f5f5;
  }

  .session-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
  }

  .session-access {
    color: #9fb8ff;
    font-weight: 600;
    font-size: 0.78rem;
  }

  .session-counts {
    display: inline-flex;
    align-items: baseline;
    gap: 0.18rem;
    font-family: 'JetBrains Mono', monospace;
  }

  .session-counts .count-label {
    font-size: 0.65rem;
    color: #7a8696;
    font-weight: 500;
    margin-right: 0.1rem;
  }

  .session-counts .present {
    font-size: 0.85rem;
    font-weight: 700;
    color: #f0f1ff;
  }

  .session-counts .total {
    color: #687183;
    font-size: 0.85rem;
  }

  .session-counts .max {
    color: #aeb8c9;
    font-size: 0.85rem;
  }

  .session-counts .slash {
    color: #5a6476;
    font-size: 0.8rem;
  }

  .session:hover:enabled {
    background: #34404c;
    transform: translateY(-1px);
  }

  .session.active {
    border-color: #ba64f2;
    box-shadow: 0 0 0 3px rgba(186, 100, 242, 0.4);
  }

  .session.focused {
    background: #2b2f35;
    color: #e1f6ff;
    border-color: #ba64f2;
    box-shadow: 0 0 0 3px rgba(186, 100, 242, 0.4);
  }

  .session strong {
    display: block;
    margin-bottom: 0.15rem;
  }

  .session span {
    color: #9aa3b3;
    font-size: 0.7rem;
  }

  .session .counts {
    color: #61d1fa;
    font-weight: 600;
  }

  .session-list button {
    padding: 0.5rem 0.75rem;
    border-radius: 0.6rem;
    border: none;
    background: #2b2f35;
    color: #61d1fa;
    font-weight: 600;
    transition: background 0.15s ease, transform 0.15s ease;
  }

  .resource-metrics {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 0.6rem;
  }

  .resource-metrics li {
    background: rgba(17, 21, 29, 0.65);
    border-radius: 0.75rem;
    border: 1px solid rgba(255, 255, 255, 0.05);
    padding: 0.6rem 0.75rem;
    display: flex;
    justify-content: space-between;
    font-size: 0.9rem;
  }

  .resource-metrics span {
    color: #86888b;
  }

  .resource-metrics strong {
    font-weight: 600;
  }

  .nav-links ul {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 0.5rem;
  }

  .nav-links button {
    width: 100%;
    text-align: left;
    padding: 0.6rem 0.75rem;
    border-radius: 0.65rem;
    border: 1px solid transparent;
    background: rgba(32, 35, 47, 0.85);
    color: #e1e1e0;
    font-weight: 600;
    transition: background 0.15s ease;
  }

  .nav-links button:hover {
    background: rgba(97, 209, 250, 0.22);
    color: #61d1fa;
  }

  .nav-links button.active {
    border-color: rgba(186, 100, 242, 0.45);
    background: rgba(186, 100, 242, 0.45);
    color: #11151d;
  }

  .dashboard {
    background: #0f1219;
    padding: 2rem;
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .tab-bar {
    display: flex;
    gap: 0.75rem;
    background: rgba(17, 21, 29, 0.8);
    padding: 0.5rem;
    border-radius: 0.9rem;
  }

  .tab-bar button {
    flex: 1;
    padding: 0.65rem 1rem;
    border-radius: 0.75rem;
    border: none;
    background: #2b2f35;
    color: #61d1fa;
    font-weight: 600;
    transition: background 0.15s ease, color 0.15s ease, transform 0.15s ease;
  }

  .tab-bar button:hover {
    background: #34404c;
    color: #61d1fa;
    transform: translateY(-1px);
  }

  .tab-bar button.active {
    background: #ba64f2;
    color: #ffffff;
    box-shadow: 0 0 12px rgba(186, 100, 242, 0.35);
  }

  .tab-panels {
    display: grid;
    gap: 1.5rem;
  }

  .panel {
    display: none;
    gap: 1.5rem;
  }

  .panel.active {
    display: grid;
    gap: 1.5rem;
  }

  .panel-grid {
    display: grid;
    gap: 1.5rem;
  }

  .panel-grid.two {
    display: grid;
    gap: 1.5rem;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  }

  .panel-grid.one {
    grid-template-columns: minmax(0, 1fr);
  }

  .card,
  .panel.notice {
    background: rgba(23, 27, 38, 0.85);
    border-radius: 1rem;
    padding: 1.5rem;
    box-shadow: 0 18px 38px rgba(0, 0, 0, 0.25);
    border: 1px solid rgba(255, 255, 255, 0.04);
  }

  .panel.notice.loading,
  .panel.notice.error {
    text-align: center;
    font-weight: 600;
  }

  .panel.notice.error {
    background: rgba(255, 118, 118, 0.1);
    color: #ffb2b2;
    border: 1px solid rgba(255, 118, 118, 0.35);
  }

  .panel.notice.info {
    background: rgba(97, 209, 250, 0.1);
    color: #c7efff;
    border: 1px solid rgba(97, 209, 250, 0.25);
  }

  .panel.notice.warning {
    background: rgba(255, 210, 0, 0.1);
    color: #ffe8a3;
    border: 1px solid rgba(255, 210, 0, 0.25);
  }

  .card h2 {
    margin: 0 0 1rem;
    font-size: 1.1rem;
  }

  .summary-card .metrics {
    display: grid;
    gap: 1rem;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    margin: 1.25rem 0;
  }

  .summary-card .metrics div {
    background: #2b2f35;
    padding: 0.85rem;
    border-radius: 0.75rem;
    border: 1px solid rgba(17, 21, 29, 0.45);
    color: #f5f5f6;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  }

  .summary-card .metrics span {
    color: #c7cad3;
    font-size: 0.75rem;
    font-weight: 600;
  }

  .summary-card .metrics strong {
    display: block;
    margin-top: 0.35rem;
    font-size: 1.05rem;
    color: #ffffff;
  }

  .options {
    display: flex;
    gap: 1rem;
    align-items: center;
    margin-top: 1rem;
  }

  .options button {
    padding: 0.5rem 0.9rem;
    border-radius: 0.6rem;
    border: 1px solid rgba(97, 209, 250, 0.3);
    background: rgba(17, 21, 29, 0.75);
    color: #61d1fa;
    font-weight: 600;
  }

  dl {
    margin: 0;
    display: grid;
    gap: 0.75rem;
  }

  dl div {
    display: grid;
    gap: 0.25rem;
  }

  dl dt {
    font-size: 0.75rem;
    color: #86888b;
  }

  dl dd {
    margin: 0;
    font-size: 0.95rem;
  }

  dl .full pre {
    margin: 0;
    background: rgba(17, 21, 29, 0.65);
    border-radius: 0.6rem;
    padding: 0.75rem;
    font-family: 'JetBrains Mono', monospace;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
    background: #1a2a36;
    border-radius: 0.75rem;
    overflow: hidden;
  }

  td select {
    background: #2b2f35;
    border: none;
    border-radius: 0.55rem;
    padding: 0.35rem 1.9rem 0.35rem 0.5rem;
    color: #e1f6ff;
    font-size: 0.8rem;
    appearance: none;
    background-image:
      linear-gradient(45deg, transparent 50%, rgba(207, 214, 228, 0.8) 50%),
      linear-gradient(135deg, rgba(207, 214, 228, 0.8) 50%, transparent 50%);
    background-position: calc(100% - 1.1rem) calc(50% - 2px), calc(100% - 0.75rem) calc(50% - 2px);
    background-size: 9px 9px;
    background-repeat: no-repeat;
  }

  th,
  td {
    padding: 0.75rem;
    border-bottom: 1.5px solid rgba(255, 255, 255, 0.15);
    text-align: left;
  }

  th {
    font-size: 0.75rem;
    color: #e8edf6;
  }

  td .sub {
    display: block;
    font-size: 0.7rem;
    color: #86888b;
  }

  tr:last-child td {
    border-bottom: none;
  }

  .logs-card .log-container {
    background: rgba(17, 21, 29, 0.6);
    border-radius: 0.75rem;
    padding: 1.1rem 1.15rem 1.05rem;
    max-height: 38vh;
    overflow: auto;
    scrollbar-width: none;
    -ms-overflow-style: none;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    font-size: 0.75rem;
  }

  .logs-card .log-container::-webkit-scrollbar {
    display: none;
  }

  .logs-card .log-container div {
    display: flex;
    gap: 0.5rem;
    align-items: flex-start;
    padding: 0.35rem 0.45rem;
    border-radius: 0.45rem;
    background: rgba(11, 14, 20, 0.9);
  }

  .logs-card .log-container.selectable,
  .logs-card .log-container.selectable * {
    user-select: text;
    -webkit-user-select: text;
  }

  .logs-card .log-container div.stderr {
    border-left: 2px solid #ff7676;
  }

  .logs-card .log-container div.command-log {
    background: rgba(97, 209, 250, 0.18);
  }

  .logs-card pre {
    margin: 0;
    font-family: 'JetBrains Mono', monospace;
    font-size: 0.7rem;
    white-space: pre-wrap;
    word-break: break-word;
  }

  /* サイドバー内のコマンドセクション */
  .command-section {
    padding: 0.5rem 1rem;
    background: transparent;
    border-top: none;
  }

  .command-section .command-input {
    margin-bottom: 0.25rem;
  }

  .command-section .command-input input {
    padding: 0.35rem 0.6rem;
    font-size: 0.8rem;
  }

  .command-section .command-input button {
    padding: 0.35rem 0.75rem;
    font-size: 0.8rem;
  }

  .command-section .command-result {
    max-height: 200px;
    overflow-y: auto;
  }

  .form-card form {
    display: grid;
    gap: 0.75rem;
  }

  .command-input {
    display: flex;
    gap: 0.75rem;
  }

  .command-input input {
    flex: 1;
    padding: 0.5rem 0.75rem;
    border-radius: 0.6rem;
    border: none;
    background: rgba(17, 21, 29, 0.65);
    color: inherit;
  }

  .command-input button,
  .form-card button,
  .summary-card button,
  .runtime-card button,
  .command-card button {
    padding: 0.55rem 1.1rem;
    border-radius: 0.65rem;
    border: none;
    background: #2b2f35;
    color: #61d1fa;
    font-weight: 600;
    transition: background 0.15s ease, transform 0.15s ease;
  }

  .command-input button:hover,
  .form-card button:hover,
  .summary-card button:hover,
  .runtime-card button:hover,
  .command-card button:hover,
  .session-list button:hover {
    background: #34404c;
    transform: translateY(-1px);
  }

  .command-help,
  .command-status {
    color: #86888b;
    font-size: 0.85rem;
  }

  .command-result {
    background: rgba(17, 21, 29, 0.75);
    border-radius: 0.75rem;
    padding: 0.75rem 1rem;
    font-family: 'JetBrains Mono', monospace;
    font-size: 0.82rem;
    line-height: 1.4;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    font-size: 0.9rem;
  }

  .field span {
    color: #86888b;
  }

  input,
  select,
  textarea {
    padding: 0.55rem 0.75rem;
    border-radius: 0.6rem;
    border: 1px solid rgba(17, 21, 29, 0.45);
    background: #2b2f35;
    color: #f5f6fb;
  }

  input::placeholder,
  textarea::placeholder {
    color: #9fa5b2;
  }

  textarea {
    resize: vertical;
    min-height: 3.5rem;
  }

  .checkbox {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    color: #86888b;
  }

  .warning {
    color: #e69e50;
    font-weight: 600;
  }

  .info {
    color: #86888b;
    font-size: 0.85rem;
  }

  .empty {
    text-align: center;
    color: #86888b;
  }

  .friend-requests {
    list-style: none;
    padding: 0;
    margin: 1rem 0 0;
    display: grid;
    gap: 0.5rem;
  }

  .friend-requests li {
    background: #1a1e27;
    border-radius: 0.6rem;
    padding: 0.6rem 0.75rem;
    color: #d7f1ff;
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .user-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .user-actions button {
    padding: 0.3rem 0.6rem;
    border-radius: 0.55rem;
    border: none;
    background: #2b2f35;
    color: #cfd6e4;
    font-size: 0.75rem;
    font-weight: 600;
  }

  td select[value='silence'],
  td select[value='unsilence'] {
    width: 100%;
  }

  .user-actions button:hover:enabled {
    background: #3a404d;
  }

  td .feedback {
    display: block;
    margin-top: 0.35rem;
    font-size: 0.7rem;
    color: #ff7676;
  }

  td .feedback.success {
    color: #59eb5c;
  }

  .field-row {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .field-row input {
    flex: 1;
  }

  .field-row button {
    padding: 0.4rem 0.8rem;
    border-radius: 0.6rem;
    border: none;
    background: #2b2f35;
    color: #61d1fa;
    font-weight: 600;
    font-size: 0.8rem;
    transition: background 0.15s ease, transform 0.15s ease;
  }

  .field-row button:not(.status-action-button):not(.schedule-type-button):hover:enabled {
    background: rgba(97, 209, 250, 0.2);
  }

  .checkbox-field {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    color: #86888b;
  }

  .checkbox-field input {
    width: 1.1rem;
    height: 1.1rem;
    accent-color: #61d1fa;
  }

  .action-buttons {
    display: flex;
    gap: 0.75rem;
    margin-top: 1rem;
  }

    .config-create-section {
      margin-bottom: 2rem;
    }

    .config-create-btn {
      background: #3f9e44;
      color: #ffffff;
      border: 1px solid #2d7a32;
      border-radius: 0.5rem;
      padding: 0.75rem 1.5rem;
      font-size: 0.875rem;
      font-weight: 500;
      cursor: pointer;
      transition: all 0.2s ease;
      height: 2.5rem;
      display: flex;
      align-items: center;
      justify-content: center;
      min-width: 200px;
    }

    .config-create-btn:hover:not(:disabled) {
      background: #2d7a32;
      border-color: #1b5e20;
      transform: translateY(-1px);
      box-shadow: 0 4px 12px rgba(63, 158, 68, 0.3);
    }

    .config-create-btn:active:not(:disabled) {
      transform: translateY(0);
      box-shadow: 0 2px 6px rgba(63, 158, 68, 0.2);
    }

    .config-create-btn:disabled {
      background: #6b7280;
      border-color: #4b5563;
      color: #9ca3af;
      cursor: not-allowed;
      transform: none;
      box-shadow: none;
    }

  .config-create-section .action-buttons {
    margin-top: 0.5rem;
  }

    .config-controls {
      display: flex;
      gap: 1rem;
      align-items: flex-end;
      justify-content: flex-start;
    }

  .config-load-section {
    flex: 0 0 auto;
    max-width: 450px;
  }

  .config-load-section .field-row {
    display: flex;
    gap: 0.75rem;
    align-items: center;
  }

  .config-load-section .select-wrapper {
    flex: 1;
    min-width: 200px;
  }

  .config-load-section .delete-button {
    background: #dc3545;
  }

  .config-load-section .delete-button:hover:not(:disabled) {
    background: #c82333;
  }

  .config-load-section .delete-button:disabled {
    background: #6c757d;
    cursor: not-allowed;
    opacity: 0.5;
  }

  .config-create-button {
    flex-shrink: 0;
  }

  .config-preview {
    background: #1a1f2e;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 0.5rem;
    padding: 1rem;
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
    font-size: 0.85rem;
    line-height: 1.4;
    color: #e1e1e0;
    white-space: pre-wrap;
    word-wrap: break-word;
  }

  .preview-controls {
    display: flex;
    gap: 0.5rem;
    margin-left: auto;
  }

  .edit-preview-btn, .save-preview-btn, .cancel-preview-btn {
    padding: 0.5rem 1rem;
    border-radius: 0.375rem;
    font-size: 0.875rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.2s ease;
    border: 1px solid;
  }

  .edit-preview-btn {
    background: #3f9e44;
    color: #ffffff;
    border-color: #2d7a32;
  }

  .edit-preview-btn:hover {
    background: #2d7a32;
    border-color: #1b5e20;
  }

  .save-preview-btn {
    background: #3b82f6;
    color: #ffffff;
    border-color: #2563eb;
  }

  .save-preview-btn:hover {
    background: #2563eb;
    border-color: #1d4ed8;
  }

  .cancel-preview-btn {
    background: #6b7280;
    color: #ffffff;
    border-color: #4b5563;
  }

  .cancel-preview-btn:hover {
    background: #4b5563;
    border-color: #374151;
  }

  .config-preview-edit {
    background: #1a1f2e;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 0.5rem;
    padding: 1rem;
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
    font-size: 0.85rem;
    line-height: 1.4;
    color: #e1e1e0;
    width: 100%;
    min-height: 300px;
    resize: none;
    outline: none;
    overflow-y: hidden;
    user-select: text;
    -webkit-user-select: text;
    -moz-user-select: text;
    -ms-user-select: text;
  }

  .config-preview-edit:focus {
    border-color: #3b82f6;
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
  }

  .preview-error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #dc2626;
    padding: 0.75rem;
    border-radius: 0.375rem;
    margin-top: 0.5rem;
    font-size: 0.875rem;
  }

  .separator {
    color: #888;
    font-weight: bold;
    font-size: 1.1rem;
    user-select: none;
  }

  .prefix {
    min-width: 80px;
    max-width: 120px;
  }

  .loading-spinner {
    color: #3b82f6;
    font-size: 0.875rem;
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  .action-buttons button {
    flex: 1;
    padding: 0.55rem 1.1rem;
    border-radius: 0.65rem;
    border: none;
    color: #f5f5f5;
    font-weight: 600;
    transition: background 0.15s ease;
    background: #2b2f35; /* デフォルト背景色 */
  }

  .action-buttons button.save {
    background: #24512c;
  }

  .action-buttons button.close {
    background: #5d323a;
  }

  .action-buttons button.restart {
    background: #48392a;
  }

  .action-buttons button.info {
    background: #2d4359; /* 情報系ボタン用の青系 */
  }
  
  .action-buttons button.danger {
    background: #c03434; /* BAN解除などの危険な操作用 */
    color: #ffffff;
  }

  .action-buttons button:hover:enabled {
    transform: translateY(-1px);
    filter: brightness(1.15);
  }

  .action-buttons button.save:hover:enabled {
    background: #2f6d3b;
  }

  .action-buttons button.close:hover:enabled {
    background: #7a404b;
  }

  .action-buttons button.restart:hover:enabled {
    background: #5f4d33;
  }

  .action-buttons button.info:hover:enabled {
    background: #3a5a7a;
  }
  
  .action-buttons button.danger:hover:enabled {
    background: #d94545;
  }

  .status-card button:not(.status-action-button),
  .status-card input,
  .status-card textarea,
  .runtime-card button,
  .command-card button {
    padding: 0.55rem 1.1rem;
    border-radius: 0.65rem;
  }

  .status-card input,
  .status-card textarea,
  .status-card select {
    border: none;
    border-radius: 0.55rem;
    padding: 0.4rem 0.6rem;
    background: #2b2f35;
    color: #e1f6ff;
    font-size: 0.85rem;
  }

  .status-card .muted {
    background: rgba(24, 34, 43, 0.85);
    color: #9aa3b3;
  }

  .status-card .status-form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .status-form label {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.34rem 0.55rem;
    background: #11151d;
    border-radius: 0.75rem;
    font-size: 0.9rem;
  }

  /* サブ設定項目のスタイル */
  .sub-settings {
    margin-top: 0.5rem;
    padding: 0.75rem 0 0.25rem 1rem;
    border-left: 2px solid rgba(97, 209, 250, 0.3);
    background: rgba(0, 0, 0, 0.15);
    border-radius: 0 0.5rem 0.5rem 0;
  }

  .sub-settings label {
    background: rgba(17, 21, 29, 0.5);
    opacity: 0.9;
  }

  .sub-settings label span:first-child {
    color: rgba(225, 225, 224, 0.8);
    font-size: 0.85rem;
  }

  /* メッセージ内容などの全幅フィールド */
  .full-width-field {
    flex-direction: column !important;
    align-items: stretch !important;
  }

  .full-width-field > span:first-child {
    margin-bottom: 0.5rem;
  }

  .full-width-field .field-row {
    width: 100%;
  }

  .status-form label > span {
    color: #f5f5f5;
    font-weight: 600;
  }

  .status-form .field-row {
    display: flex;
    gap: 0.65rem;
    align-items: center;
    justify-content: flex-end;
  }

  .status-form .field-row.end {
    justify-content: flex-end;
  }

  /* ステータス表示用のスタイル */
  .status-display-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .status-display-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.34rem 0.55rem;
    background: #11151d;
    border-radius: 0.75rem;
    font-size: 0.9rem;
  }

  .status-display-label {
    color: #f5f5f5;
    font-weight: 600;
  }

  .status-display-value {
    padding: 0.5rem 0.75rem;
    background: rgba(17, 21, 29, 0.6);
    border-radius: 0.5rem;
    color: #e1e1e0;
    font-size: 0.9rem;
    min-width: 150px;
    text-align: right;
  }

  .status-card .checkbox-field {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    padding: 0.34rem 0.55rem;
    background: #11151d;
    border-radius: 0.75rem;
    font-weight: 600;
    color: #f5f5f5;
  }

  .status-card .checkbox-field input {
    order: 1;
    width: 1rem;
    height: 1rem;
  }

  .status-card .action-buttons {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
  }

  .status-card .feedback {
    font-size: 0.75rem;
    color: #ff7676;
  }

  .status-card .feedback.success {
    color: #59eb5c;
  }

  @media (max-width: 1200px) {
    .content {
      grid-template-columns: 1fr;
    }

    .sidebar {
      border-right: none;
      border-bottom: 1px solid rgba(255, 255, 255, 0.08);
      grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
    }
  }

  @media (max-width: 720px) {
    .topbar {
      flex-direction: column;
      align-items: flex-start;
      gap: 1rem;
    }

    .topbar-controls {
      width: 100%;
      flex-wrap: wrap;
      gap: 1rem;
    }

    .command-input {
      flex-direction: column;
    }

    .command-input button,
    .session-list button,
    .status-indicators button,
    .resource-metrics button,
    .summary-card button {
      width: 100%;
    }
  }

  .field-placeholder {
    min-width: 220px;
    padding: 0.55rem 0.9rem;
    border-radius: 0.75rem;
    background: rgba(43, 47, 53, 0.4);
    color: rgba(225, 225, 224, 0.6);
    border: 1px dashed rgba(186, 100, 242, 0.4);
  }

  .backend-status {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.5rem 0.9rem;
    background: rgba(48, 73, 89, 0.5);
    border-radius: 0.75rem;
    font-size: 0.85rem;
    color: #cceaff;
  }

  .backend-status .dot {
    width: 0.65rem;
    height: 0.65rem;
    border-radius: 50%;
    background: #59eb5c;
  }

  .backend-status.offline {
    background: rgba(120, 43, 43, 0.6);
    color: #ffdcdc;
  }

  .backend-status.offline .dot {
    background: #ff7676;
  }

  .status-form .field-row {
    display: flex;
    gap: 0.65rem;
    align-items: center;
  }

  .status-form .field-row .slash {
    color: rgba(225, 225, 224, 0.35);
  }

  select.disabled-control,
  select.disabled-control:disabled,
  .user-actions button.disabled-control,
  .user-actions button:disabled {
    opacity: 0.4;
    pointer-events: none;
    cursor: not-allowed;
  }

  .panel-column {
    display: flex;
    flex-direction: column;
    gap: 0.175rem;
  }

  .panel-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    color: #61d1fa;
    font-weight: 700;
    letter-spacing: 0.02em;
  }

  .panel-heading h2 {
    margin: 0;
    font-size: 1.2rem;
  }

  .refresh-button {
    width: 36px;
    height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: #2b2f35;
    color: #ffffff;
    border-radius: 0.55rem;
    border: none;
    font-size: 1.1rem;
    transition: background 0.15s ease;
  }

  .refresh-button:hover:enabled {
    background: rgba(97, 209, 250, 0.22);
  }

  .refresh-button:disabled {
    opacity: 0.45;
  }

  .status-card,
  .users-card {
    background: #2b2f35;
    padding: 0.7rem 0.55rem;
  }

  .description-block {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.34rem 0.55rem;
    background: #11151d;
    border-radius: 0.75rem;
  }

  .description-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.9rem;
    color: #f5f5f5;
    font-weight: 600;
  }

  .description-header button {
    background: #2b2f35;
    color: #61d1fa;
    border: none;
    padding: 0.35rem 0.75rem;
    border-radius: 0.6rem;
    font-weight: 600;
  }

  .description-header button:hover:enabled {
    background: rgba(97, 209, 250, 0.2);
  }

  .description-block textarea {
    width: 100%;
    min-height: 6rem;
    border-radius: 0.6rem;
    border: none;
    background: #2b2f35;
  }

  .field-row button {
    padding: 0.4rem 0.8rem;
    border-radius: 0.6rem;
    border: none;
    background: #2b2f35;
    color: #61d1fa;
    font-weight: 600;
    font-size: 0.8rem;
    transition: background 0.15s ease, transform 0.15s ease;
  }

  .field-row button:not(.status-action-button):not(.schedule-type-button):hover:enabled {
    background: rgba(97, 209, 250, 0.2);
  }

  .status-action-button {
    width: 72px;
    height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: rgba(24, 34, 43, 0.85);
    color: #61d1fa;
    border-radius: 0.55rem;
    border: none;
    font-weight: 600;
    font-size: 0.92rem;
    padding: 0;
    transition: none;
  }

  .status-action-button:hover:enabled {
    background: rgba(24, 34, 43, 0.85);
    color: #61d1fa;
  }

  .status-action-button:disabled {
    opacity: 0.55;
  }

  .status-action-button.active {
    background: #61d1fa;
    color: #11151d;
    border-color: transparent;
  }

  .status-action-button.active:hover:enabled {
    background: #61d1fa;
    color: #11151d;
  }

  .toggle-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.34rem 0.55rem;
    background: #11151d;
    border-radius: 0.75rem;
    font-size: 0.9rem;
    color: #f5f5f5;
    font-weight: 600;
  }

  .status-action-button:focus-visible {
    outline: none;
    box-shadow: none;
  }

  .refresh-icon {
    width: 20px;
    height: 20px;
    fill: currentColor;
  }

  .users-card table {
    background: #11151d;
  }

  /* world search */
  .world-search {
    background: #11151d; /* ヘッダーと同色 */
    border-radius: 0.75rem;
    padding: 0.75rem;
    margin-top: 0.5rem;
    box-shadow: 0 8px 18px rgba(0, 0, 0, 0.2);
  }

  .world-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 1rem;
  }

  @media (max-width: 768px) {
    .world-grid {
      grid-template-columns: 1fr;
    }
  }

  .world-card {
    background: #11151d;
    border: 2px solid transparent;
    border-radius: 0.75rem;
    overflow: hidden;
    transition: all 0.2s ease;
    cursor: pointer;
  }

  .world-card:hover {
    border-color: #61d1fa;
    box-shadow: 0 0 0 2px rgba(97, 209, 250, 0.2);
  }

  .world-card.selected {
    border-color: #ba64f2;
    box-shadow: 0 0 0 3px rgba(186, 100, 242, 0.35);
  }

  .world-card .thumb {
    width: 100%;
    height: 140px;
    background: #11151d;
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
    overflow: hidden;
  }

  .world-card .thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .thumb-placeholder {
    color: #9aa3b3;
    font-size: 0.85rem;
  }

  .world-card .meta {
    padding: 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .world-card .title-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .world-card .sub-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .world-card .title {
    font-size: 0.95rem;
    color: #f5f5f5;
    font-weight: 500;
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .world-card .sub {
    font-size: 0.8rem;
    color: #9aa3b3;
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .copy-button {
    background: rgba(97, 209, 250, 0.1);
    border: 1px solid rgba(97, 209, 250, 0.3);
    color: #61d1fa;
    cursor: pointer;
    font-size: 0.7rem;
    padding: 0.25rem 0.5rem;
    border-radius: 0.25rem;
    transition: all 0.15s ease;
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 1.5rem;
    min-height: 1.5rem;
  }

  .copy-button:hover {
    background: rgba(97, 209, 250, 0.2);
    border-color: rgba(97, 209, 250, 0.5);
  }

  .copy-button:focus-visible {
    outline: 2px solid #61d1fa;
    outline-offset: 1px;
  }

  .copy-icon {
    width: 0.875rem;
    height: 0.875rem;
    fill: currentColor;
  }

  /* ユーザーカードスタイル */
  .user-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .user-card {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #11151d;
    border: 2px solid transparent;
    border-radius: 0.75rem;
    padding: 0.75rem;
    transition: all 0.2s ease;
    cursor: pointer;
    width: 100%;
    text-align: left;
  }

  .user-card:hover {
    border-color: #61d1fa;
    box-shadow: 0 0 0 2px rgba(97, 209, 250, 0.2);
    background: #171b26;
  }

  .user-card.selected {
    border-color: #ba64f2;
    box-shadow: 0 0 0 3px rgba(186, 100, 242, 0.35);
    background: #1e2332;
  }

  .user-avatar {
    width: 48px;
    height: 48px;
    border-radius: 50%;
    overflow: hidden;
    background: #2b2f35;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .user-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .avatar-placeholder {
    color: #9aa3b3;
    font-size: 1.25rem;
    font-weight: 600;
  }
  
  .avatar-placeholder.ban {
    color: #ff6b6b;
    font-size: 1.5rem;
  }
  
  .user-card.ban-card {
    border-color: rgba(255, 107, 107, 0.3);
    background: rgba(255, 107, 107, 0.05);
  }
  
  .user-card.ban-card:hover {
    border-color: #ff6b6b;
    box-shadow: 0 0 0 2px rgba(255, 107, 107, 0.2);
    background: rgba(255, 107, 107, 0.1);
  }
  
  .user-card.ban-card.selected {
    border-color: #ff6b6b;
    box-shadow: 0 0 0 3px rgba(255, 107, 107, 0.35);
    background: rgba(255, 107, 107, 0.15);
  }
  
  .machine-id {
    font-family: 'JetBrains Mono', monospace;
    font-size: 0.75rem;
  }

  .user-info {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    flex: 1;
    min-width: 0;
  }

  .user-info strong {
    font-size: 0.95rem;
    color: #f5f5f5;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .user-info .sub {
    font-size: 0.75rem;
    color: #9aa3b3;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* フィールドプレフィックス */
  .field-prefix {
    color: #9aa3b3;
    font-weight: 600;
    margin-right: 0.5rem;
    user-select: none;
  }


  .startup-overlay {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: 1000;
  }

  .loader {
    border: 4px solid #f3f3f3;
    border-top: 4px solid #3498db;
    border-radius: 50%;
    width: 40px;
    height: 40px;
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
  }

  .toast-container {
    position: fixed;
    top: 1.25rem;
    left: 50%;
    transform: translateX(-50%);
    display: grid;
    gap: 0.5rem;
    z-index: 9999;
    pointer-events: none;
  }

  .toast {
    min-width: 180px;
    max-width: 260px;
    padding: 0.55rem 0.9rem;
    border-radius: 0.6rem;
    background: rgba(17, 21, 29, 0.96);
    color: #e9f9ff;
    font-weight: 500;
    font-size: 0.82rem;
    line-height: 1.35;
    box-shadow: 0 10px 24px rgba(0, 0, 0, 0.24);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    border: 1px solid rgba(97, 209, 250, 0.28);
    backdrop-filter: blur(5px);
    pointer-events: auto;
    animation: toast-pop 220ms ease-out;
  }

  @keyframes toast-pop {
    0% {
      opacity: 0;
      transform: translate(-50%, -20px);
    }
    100% {
      opacity: 1;
      transform: translate(-50%, 0);
    }
  }

  .toast.success {
    border-color: rgba(84, 226, 146, 0.5);
    color: #c0ffd9;
  }

  .toast.error {
    border-color: rgba(255, 118, 118, 0.58);
    color: #ffd0d6;
  }

  .toast.info {
    border-color: rgba(97, 209, 250, 0.6);
  }

  /* Session tabs styles */
  .session-tab-bar {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1rem;
    flex-wrap: wrap;
  }

  .session-tab {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 1rem;
    background: #2b2f35;
    color: #61d1fa;
    border: none;
    border-radius: 0.5rem;
    font-weight: 600;
    transition: background 0.15s ease;
    position: relative;
  }

  .session-tab:hover {
    background: #34404c;
  }

  .session-tab.active {
    background: #ba64f2;
    color: #ffffff;
  }

  .remove-session-btn {
    background: #ff7676;
    color: #ffffff;
    border: none;
    border-radius: 50%;
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    font-weight: bold;
    margin-left: 0.5rem;
    transition: background 0.15s ease;
    cursor: pointer;
  }

  .remove-session-btn:hover {
    background: #ff5555;
  }

  .add-session-btn {
    background: #3f9e44;
    color: #ffffff;
    border: none;
    border-radius: 0.5rem;
    width: 40px;
    height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 18px;
    font-weight: bold;
    transition: background 0.15s ease;
  }

  .add-session-btn:hover {
    background: #2d7a32;
  }

  /* レスポンシブ対応 */
  @media (max-width: 1200px) {
    .topbar {
      flex-direction: column;
      align-items: stretch;
      gap: 1rem;
    }

    .topbar-controls {
      justify-content: center;
      flex-wrap: wrap;
    }

    .brand h1 {
      font-size: 1.3rem;
    }
  }

  @media (max-width: 768px) {
    .topbar {
      padding: 1rem;
    }

    .topbar-controls {
      flex-direction: column;
      align-items: stretch;
      gap: 1rem;
    }

    .topbar-controls .field {
      min-width: auto;
    }

    .topbar-controls select {
      min-width: auto;
    }

    .status-indicators {
      justify-content: center;
    }

    .brand h1 {
      font-size: 1.1rem;
    }

    .logo {
      width: 2.5rem;
      height: 2.5rem;
    }
  }

  /* ログイン画面のスタイル */
  .login-container {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(135deg, #1a1d23 0%, #2b2f35 100%);
    padding: 2rem;
  }

  .login-card {
    background: #2b2f35;
    border-radius: 1rem;
    padding: 2.5rem;
    box-shadow: 0 20px 40px rgba(0, 0, 0, 0.3);
    max-width: 400px;
    width: 100%;
  }

  .login-header {
    text-align: center;
    margin-bottom: 2rem;
  }

  .login-header h1 {
    color: #ffffff;
    font-size: 1.5rem;
    margin-bottom: 0.5rem;
  }

  .login-header p {
    color: #a0a0a0;
    font-size: 0.9rem;
  }

  .login-form {
    margin-bottom: 1.5rem;
  }

  .login-form .field {
    margin-bottom: 1.5rem;
  }

  .login-form label {
    display: block;
    color: #ffffff;
    font-size: 0.9rem;
    margin-bottom: 0.5rem;
  }

  .login-form input {
    width: 100%;
    padding: 0.75rem;
    border: 1px solid #404040;
    border-radius: 0.5rem;
    background: #1a1d23;
    color: #ffffff;
    font-size: 1rem;
  }

  .login-form input:focus {
    outline: none;
    border-color: #4a9eff;
    box-shadow: 0 0 0 2px rgba(74, 158, 255, 0.2);
  }

  .login-button {
    width: 100%;
    padding: 0.75rem;
    background: #4a9eff;
    color: #ffffff;
    border: none;
    border-radius: 0.5rem;
    font-size: 1rem;
    font-weight: 500;
    cursor: pointer;
    transition: background-color 0.2s;
  }

  .login-button:hover:not(:disabled) {
    background: #3a8eef;
  }

  .login-button:disabled {
    background: #404040;
    cursor: not-allowed;
  }

  .login-info {
    text-align: center;
    color: #a0a0a0;
    font-size: 0.85rem;
  }

  .login-info code {
    background: #404040;
    padding: 0.2rem 0.4rem;
    border-radius: 0.25rem;
    font-family: monospace;
  }

  .logout-button {
    background: #61d1fa;
    color: #000000;
    font-weight: 700;
    font-size: 1.1rem;
    letter-spacing: 0.03em;
    min-width: 140px;
    transition: transform 0.2s ease, filter 0.2s ease;
    border: none;
    border-radius: 0.85rem;
    padding: 0 1.45rem;
    min-height: 54px;
    cursor: pointer;
  }

  .logout-button:hover {
    transform: translateY(-1px);
    filter: brightness(1.08);
  }

  /* 自動再起動設定タブのスタイル */
  .config-create-btn.danger-button {
    background: #ff6b6b;
    color: #ffffff;
    border: none;
  }

  .config-create-btn.danger-button:hover {
    background: #ff8787;
    border: none;
  }

  /* モーダルスタイル */
  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: 2000;
    backdrop-filter: blur(3px);
  }

  .modal-content {
    background: #1a1e27;
    border-radius: 0.75rem;
    max-width: 600px;
    width: 90%;
    max-height: 90vh;
    overflow-y: auto;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
    border: 1px solid #2b2f35;
  }

  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid #2b2f35;
  }

  .modal-header h2 {
    margin: 0;
    font-size: 1.25rem;
    color: #61d1fa;
  }

  .modal-close {
    background: none;
    border: none;
    color: #a0a0a0;
    font-size: 2rem;
    cursor: pointer;
    transition: color 0.15s ease;
    padding: 0;
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .modal-close:hover {
    color: #ffffff;
  }

  .modal-body {
    padding: 1.5rem;
  }

  .modal-label {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .modal-label span {
    font-weight: 600;
    color: #e9f9ff;
  }

  .modal-label input,
  .modal-label select {
    padding: 0.5rem 0.75rem;
    background: #11151d;
    border: 1px solid #2b2f35;
    border-radius: 0.5rem;
    color: #e9f9ff;
    font-size: 0.95rem;
  }

  .modal-label input:focus,
  .modal-label select:focus {
    outline: none;
    border-color: #61d1fa;
  }

  .modal-actions {
    display: flex;
    gap: 0.75rem;
    margin-top: 1.5rem;
    justify-content: flex-end;
  }

  .modal-cancel-btn,
  .modal-save-btn {
    padding: 0.625rem 1.25rem;
    border-radius: 0.5rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
    border: none;
  }

  .modal-cancel-btn {
    background: #2b2f35;
    color: #e9f9ff;
  }

  .modal-cancel-btn:hover {
    background: #34404c;
  }

  .modal-save-btn {
    background: #61d1fa;
    color: #11151d;
  }

  .modal-save-btn:hover {
    background: #7dd9fc;
  }

  /* 予定タイプ選択ボタン（ホバー無効） */
  .schedule-type-button {
    padding: 0.5rem 1rem;
    background: #2b2f35;
    color: #e9f9ff;
    border: 1px solid #2b2f35;
    border-radius: 0.5rem;
    font-weight: 600;
    cursor: pointer;
    transition: none;
  }

  .schedule-type-button:hover:enabled {
    background: #2b2f35;
    color: #e9f9ff;
  }

  .schedule-type-button.active {
    background: #61d1fa;
    color: #11151d;
    border-color: #61d1fa;
  }

  .schedule-type-button.active:hover:enabled {
    background: #61d1fa;
    color: #11151d;
  }


</style>
