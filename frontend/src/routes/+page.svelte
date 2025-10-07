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
    getRuntimeWorlds,
    postCommand,
    postFocusWorld,
    postFocusWorldRefresh,
    startServer,
    stopServer,
    getWorldSearch,
    type WorldSearchItem,
    type RuntimeStatusData,
    type RuntimeUsersData,
    type FriendRequestsData,
    type RuntimeWorldsData,
    type RuntimeWorldEntry,
    type ConfigEntry,
    type LogEntry
  } from '$lib';

  const { status, logs, configs, setConfigs, setStatus, setLogs, clearLogs } = createServerStores();

  const tabs = [
    { id: 'dashboard', label: 'ダッシュボード' },
    { id: 'newWorld', label: '新規セッション' },
    { id: 'friends', label: 'フレンド管理' },
    { id: 'settings', label: 'コンフィグ作成' },
    { id: 'commands', label: 'コマンド' }
  ];

  let activeTab: (typeof tabs)[number]['id'] = 'dashboard';

  let initialLoading = true;
  let selectedConfig: string | undefined;
  let appMessage: { type: 'error' | 'warning' | 'info'; text: string } | null = null;
  const notificationsStore = writable<Array<{ id: number; message: string; type: 'success' | 'error' | 'info' }>>([]);
  const notifications = derived(notificationsStore, value => value);
  const pushToast = (message: string, type: 'success' | 'error' | 'info' = 'info', duration = 4200) => {
    const id = Date.now() + Math.random();
    notificationsStore.update(items => [...items, { id, message, type }]);
    setTimeout(() => {
      notificationsStore.update(items => items.filter(item => item.id !== id));
    }, duration);
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
  let backendReachable = true;
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

  let friendSendLoading = false;

  let friendAcceptLoading = false;

  let friendRemoveLoading = false;

  let friendTargetName = '';
  let friendMessageText = '';
  let friendMessageLoading = false;
  let friendRequests: FriendRequestsData | null = null;
  let friendRequestsLoading = false;
  let friendRequestsError = '';

  // World search state
  let worldSearchTerm = '';
  let worldSearchLoading = false;
  let worldSearchError = '';
  let worldSearchResults: WorldSearchItem[] = [];
  let selectedResoniteUrl: string | null = null;

  // Config generation state
  // 仕様: 何も操作せず「作成」で default.json と同内容を生成するためデフォルト名を 'default' に設定
  let configName = 'default';
  let configUsername = '';
  let configPassword = '';
  let configGenerationLoading = false;
  
  // Advanced config settings
  let configComment = '';
  let configUniverseId = '';
  let configTickRate = 60.0;
  let configMaxConcurrentAssetTransfers = 128;
  let configUsernameOverride = '';
  let configDataFolder = '';
  let configCacheFolder = '';
  let configLogsFolder = '';
  let configAllowedUrlHosts = '';
  let configAutoSpawnItems = '';
  
  // Session management (default.json と同値になるよう初期値設定)
  let sessions = [
    {
      id: 1,
      isEnabled: true,
      sessionName: '',
      customSessionId: '',
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
      awayKickMinutes: -1.0,
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
    }
  ];
  let activeSessionTab = 1;
  let nextSessionId = 2;
  let showConfigPreview = false;
  let configPreviewText = '';
  
  // 下書き保存/復元: コンフィグ作成タブの編集中データをLocalStorageに自動保存
  const loadDraft = () => {
    try {
      const raw = localStorage.getItem(DRAFT_STORAGE_KEY);
      if (!raw) return false;
      const draft = JSON.parse(raw);
      configName = draft.configName ?? '';
      configUsername = draft.configUsername ?? '';
      configPassword = draft.configPassword ?? '';
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
      const draft = {
        configName,
        configUsername,
        configPassword,
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
        sessions
      };
      localStorage.setItem(DRAFT_STORAGE_KEY, JSON.stringify(draft));
    } catch {
      // ignore
    }
  };

  const clearDraft = () => {
    localStorage.removeItem(DRAFT_STORAGE_KEY);
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

  const resourceMetrics = [
    { label: 'CPU', value: '--- %' },
    { label: 'メモリ', value: '--- GB' }
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
      awayKickMinutes: 30,
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
      
      // 仕様: ユーザー名/パスワードは空でOK。name のみ必須。
      if (!trimmedName) {
        pushToast('設定名を入力してください', 'error');
        return;
      }

      // セッションデータを処理
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

        // 文字列フィールド: 空は null、カンマ区切りは配列、JSONはパース。default.json と整合
        processedSession.sessionName = session.sessionName.trim() ? session.sessionName.trim() : null;
        processedSession.customSessionId = session.customSessionId.trim() ? session.customSessionId.trim() : null;
        processedSession.description = session.description.trim() ? session.description.trim() : null;
        processedSession.tags = session.tags.trim() ? session.tags.split(',').map(t => t.trim()).filter(Boolean) : null;
        processedSession.loadWorldURL = session.loadWorldURL.trim() ? session.loadWorldURL.trim() : null;
        processedSession.loadWorldPresetName = session.loadWorldPresetName.trim() ? session.loadWorldPresetName.trim() : 'Grid';
        processedSession.overrideCorrespondingWorldId = session.overrideCorrespondingWorldId.trim() ? session.overrideCorrespondingWorldId.trim() : null;
        if (session.forcePort !== null && session.forcePort !== '') processedSession.forcePort = Number(session.forcePort);
        else processedSession.forcePort = null;
        if (session.defaultUserRoles.trim()) {
          try {
            processedSession.defaultUserRoles = JSON.parse(session.defaultUserRoles);
          } catch {
            processedSession.defaultUserRoles = null;
          }
        } else {
          processedSession.defaultUserRoles = null;
        }
        processedSession.roleCloudVariable = session.roleCloudVariable.trim() ? session.roleCloudVariable.trim() : null;
        processedSession.allowUserCloudVariable = session.allowUserCloudVariable.trim() ? session.allowUserCloudVariable.trim() : null;
        processedSession.denyUserCloudVariable = session.denyUserCloudVariable.trim() ? session.denyUserCloudVariable.trim() : null;
        processedSession.requiredUserJoinCloudVariable = session.requiredUserJoinCloudVariable.trim() ? session.requiredUserJoinCloudVariable.trim() : null;
        processedSession.requiredUserJoinCloudVariableDenyMessage = session.requiredUserJoinCloudVariableDenyMessage.trim() ? session.requiredUserJoinCloudVariableDenyMessage.trim() : null;
        processedSession.parentSessionIds = session.parentSessionIds.trim() ? session.parentSessionIds.split(',').map(s => s.trim()).filter(Boolean) : null;
        processedSession.autoInviteUsernames = session.autoInviteUsernames.trim() ? session.autoInviteUsernames.split(',').map(s => s.trim()).filter(Boolean) : null;
        processedSession.autoInviteMessage = session.autoInviteMessage.trim() ? session.autoInviteMessage.trim() : null;
        processedSession.saveAsOwner = session.saveAsOwner.trim() ? session.saveAsOwner.trim() : null;

        return processedSession;
      });

      const configData = {
        comment: configComment.trim() || `${trimmedName}の設定ファイル`,
        universeId: configUniverseId.trim() ? configUniverseId.trim() : null,
        tickRate: configTickRate,
        maxConcurrentAssetTransfers: configMaxConcurrentAssetTransfers,
        usernameOverride: configUsernameOverride.trim() ? configUsernameOverride.trim() : null,
        dataFolder: configDataFolder.trim() ? configDataFolder.trim() : null,
        cacheFolder: configCacheFolder.trim() ? configCacheFolder.trim() : null,
        logsFolder: configLogsFolder.trim() ? configLogsFolder.trim() : null,
        allowedUrlHosts: configAllowedUrlHosts.trim() ? configAllowedUrlHosts.split(',').map(h => h.trim()).filter(Boolean) : null,
        autoSpawnItems: configAutoSpawnItems.trim() ? configAutoSpawnItems.split(',').map(i => i.trim()).filter(Boolean) : null,
        startWorlds: processedSessions
      };

      // ここで初めてバックエンドへ送信（"作成"ボタン押下時）
      await generateConfig(trimmedName, trimmedUsername, trimmedPassword, configData);
      pushToast('コンフィグファイルを作成しました', 'success');
      
      // 設定ファイル一覧を更新
      await refreshConfigsOnly();
      
      // フォームをクリア
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
      configAllowedUrlHosts = '';
      configAutoSpawnItems = '';
      sessions = [{
        id: 1,
        isEnabled: true,
        sessionName: '',
        customSessionId: '',
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
        awayKickMinutes: 30,
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
      configGenerationLoading = false;
    }
  };

  // プレビュー生成（保存は行わず、UIにJSONを表示）
  const openConfigPreview = () => {
    const trimmedName = configName.trim() || 'Config';
    const trimmedUsername = configUsername.trim();
    const trimmedPassword = configPassword.trim();
    // セッション整形（generateConfigFileのロジックと同一）
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
      if (session.sessionName.trim()) processedSession.sessionName = session.sessionName.trim();
      if (session.customSessionId.trim()) processedSession.customSessionId = session.customSessionId.trim();
      if (session.description.trim()) processedSession.description = session.description.trim();
      if (session.tags.trim()) processedSession.tags = session.tags.split(',').map(t => t.trim()).filter(Boolean);
      if (session.loadWorldURL.trim()) processedSession.loadWorldURL = session.loadWorldURL.trim();
      if (session.loadWorldPresetName.trim()) processedSession.loadWorldPresetName = session.loadWorldPresetName.trim();
      if (session.overrideCorrespondingWorldId.trim()) processedSession.overrideCorrespondingWorldId = session.overrideCorrespondingWorldId.trim();
      if (session.forcePort !== null && session.forcePort !== '') processedSession.forcePort = Number(session.forcePort);
      if (session.defaultUserRoles.trim()) {
        try {
          processedSession.defaultUserRoles = JSON.parse(session.defaultUserRoles);
        } catch {}
      }
      if (session.roleCloudVariable.trim()) processedSession.roleCloudVariable = session.roleCloudVariable.trim();
      if (session.allowUserCloudVariable.trim()) processedSession.allowUserCloudVariable = session.allowUserCloudVariable.trim();
      if (session.denyUserCloudVariable.trim()) processedSession.denyUserCloudVariable = session.denyUserCloudVariable.trim();
      if (session.requiredUserJoinCloudVariable.trim()) processedSession.requiredUserJoinCloudVariable = session.requiredUserJoinCloudVariable.trim();
      if (session.requiredUserJoinCloudVariableDenyMessage.trim()) processedSession.requiredUserJoinCloudVariableDenyMessage = session.requiredUserJoinCloudVariableDenyMessage.trim();
      if (session.parentSessionIds.trim()) processedSession.parentSessionIds = session.parentSessionIds.split(',').map(s => s.trim()).filter(Boolean);
      if (session.autoInviteUsernames.trim()) processedSession.autoInviteUsernames = session.autoInviteUsernames.split(',').map(s => s.trim()).filter(Boolean);
      if (session.autoInviteMessage.trim()) processedSession.autoInviteMessage = session.autoInviteMessage.trim();
      if (session.saveAsOwner.trim()) processedSession.saveAsOwner = session.saveAsOwner.trim();
      return processedSession;
    });

    const configObject = {
      $schema: 'https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json',
      comment: configComment.trim() || `${trimmedName}の設定ファイル`,
      universeId: configUniverseId.trim() || null,
      tickRate: configTickRate,
      maxConcurrentAssetTransfers: configMaxConcurrentAssetTransfers,
      usernameOverride: configUsernameOverride.trim() ? configUsernameOverride.trim() : null,
      loginCredential: trimmedUsername,
      loginPassword: trimmedPassword,
      startWorlds: processedSessions,
      dataFolder: configDataFolder.trim() || null,
      cacheFolder: configCacheFolder.trim() || null,
      logsFolder: configLogsFolder.trim() || null,
      allowedUrlHosts: configAllowedUrlHosts.trim() ? configAllowedUrlHosts.split(',').map(h => h.trim()).filter(Boolean) : null,
      autoSpawnItems: configAutoSpawnItems.trim() ? configAutoSpawnItems.split(',').map(i => i.trim()).filter(Boolean) : null
    };

    configPreviewText = JSON.stringify(configObject, null, 2);
    showConfigPreview = true;
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
        const message = error instanceof Error ? error.message : '初期データの取得に失敗しました';
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

  onMount(() => {
    // 起動時に下書き復元を試行
    const restored = loadDraft();
    if (restored) {
      pushToast('編集中の下書きを復元しました', 'info');
    }
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
  $: if (activeTab === 'settings') {
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
        await postCommand(`message ${JSON.stringify(target)} ${JSON.stringify(text)}`);
        pushToast('メッセージを送信しました。', 'success');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'メッセージを送信できませんでした';
      pushToast(message, 'error');
    } finally {
      friendMessageLoading = false;
    }
  };

  const executeCommand = async () => {
    if (commandLoading) return;
    commandLoading = true;
    try {
      const result = await postCommand(commandText);
      commandResult = typeof result === 'string' ? result : JSON.stringify(result);
    } catch (error) {
      commandResult = error instanceof Error ? error.message : 'コマンドを実行できませんでした';
    } finally {
      commandLoading = false;
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
</script>

<svelte:head>
  <title>MarkN Resonite Headless Controller</title>
</svelte:head>

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
                      <span class="present">{world.presentUsers ?? '-'}</span>
                      <span class="slash">/</span>
                      <span class="total">{world.currentUsers ?? '-'}</span>
                      <span class="slash">/</span>
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
            <div class="panel-grid two">
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
                            コピー
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
            <div class="panel-grid one">
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>/friends</h2>
                </div>
                <div class="card form-card">
                  <h2>対象ユーザー</h2>
                  <label class="field">
                    <span>フレンド名</span>
                    <input type="text" bind:value={friendTargetName} placeholder="ユーザー名" />
                  </label>
                  <p class="info">以下の操作はすべてこの名前を使用します。</p>
                  <button type="button" on:click={fetchFriendRequests} disabled={!$status.running || friendRequestsLoading}>
                    フレンドリクエスト一覧を取得
                  </button>
                  {#if friendRequestsLoading}
                    <p class="info">取得中...</p>
                  {:else if friendRequestsError}
                    <p class="feedback">{friendRequestsError}</p>
                  {:else if friendRequests?.data?.length}
                    <ul class="friend-requests">
                      {#each friendRequests.data as request}
                        <li>{request}</li>
                      {/each}
                    </ul>
                  {:else if friendRequests}
                    <p class="info">保留中のフレンドリクエストはありません。</p>
                  {/if}
                </div>
              </div>
            </div>

            <div class="panel-grid two">
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>/send</h2>
                </div>
                <div class="card form-card">
                  <h2>フレンド申請を送る</h2>
                  <form on:submit|preventDefault={submitFriendSend}>
                    <button type="submit" disabled={!$status.running || friendSendLoading}>送信</button>
                  </form>
                </div>
              </div>
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>/accept</h2>
                </div>
                <div class="card form-card">
                  <h2>申請を承認する</h2>
                  <form on:submit|preventDefault={submitFriendAccept}>
                    <button type="submit" disabled={!$status.running || friendAcceptLoading}>承認</button>
                  </form>
                </div>
              </div>
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>/remove</h2>
                </div>
                <div class="card form-card">
                  <h2>フレンド解除</h2>
                  <form on:submit|preventDefault={submitFriendRemove}>
                    <button type="submit" disabled={!$status.running || friendRemoveLoading}>解除</button>
                  </form>
                </div>
              </div>
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>/message</h2>
                </div>
                <div class="card form-card">
                  <h2>メッセージ送信</h2>
                  <form on:submit|preventDefault={submitFriendMessage}>
                    <label class="field">
                      <span>メッセージ</span>
                      <textarea rows="3" bind:value={friendMessageText}></textarea>
                    </label>
                    <button type="submit" disabled={!$status.running || friendMessageLoading}>送信</button>
                  </form>
                </div>
              </div>
            </div>
          </section>

          <section class="panel" class:active={activeTab === 'settings'}>
            <div class="panel-grid two">
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>基本設定</h2>
                </div>
                <div class="card status-card">
                  <form class="status-form" on:submit|preventDefault={generateConfigFile}>
                    <!-- 基本設定 -->
                    <label>
                      <span>設定名</span>
                      <div class="field-row">
                        <input type="text" bind:value={configName} placeholder="例: メインサーバー" />
                      </div>
                    </label>

                    <label>
                      <span>Resoniteユーザー名</span>
                      <div class="field-row">
                        <input type="text" bind:value={configUsername} placeholder="あなたのResoniteユーザー名" />
                      </div>
                    </label>

                    <label>
                      <span>パスワード</span>
                      <div class="field-row">
                        <input type="password" bind:value={configPassword} placeholder="あなたのResoniteパスワード" />
                      </div>
                    </label>

                    <label>
                      <span>コメント</span>
                      <div class="field-row">
                        <input type="text" bind:value={configComment} placeholder="設定ファイルの説明" />
                      </div>
                    </label>

                    <!-- 詳細設定 -->
                    <label>
                      <span>Universe ID</span>
                      <div class="field-row">
                        <input type="text" bind:value={configUniverseId} placeholder="Universe ID (オプション)" />
                      </div>
                    </label>

                    <label>
                      <span>Tick Rate</span>
                      <div class="field-row">
                        <input type="number" bind:value={configTickRate} min="1" max="120" />
                      </div>
                    </label>

                    <label>
                      <span>最大同時アセット転送数</span>
                      <div class="field-row">
                        <input type="number" bind:value={configMaxConcurrentAssetTransfers} min="1" max="1000" />
                      </div>
                    </label>

                    <label>
                      <span>ユーザー名オーバーライド</span>
                      <div class="field-row">
                        <input type="text" bind:value={configUsernameOverride} placeholder="表示用ユーザー名" />
                      </div>
                    </label>

                    <label>
                      <span>データフォルダ</span>
                      <div class="field-row">
                        <input type="text" bind:value={configDataFolder} placeholder="データ保存フォルダ" />
                      </div>
                    </label>

                    <label>
                      <span>キャッシュフォルダ</span>
                      <div class="field-row">
                        <input type="text" bind:value={configCacheFolder} placeholder="キャッシュフォルダ" />
                      </div>
                    </label>

                    <label>
                      <span>ログフォルダ</span>
                      <div class="field-row">
                        <input type="text" bind:value={configLogsFolder} placeholder="ログ保存フォルダ" />
                      </div>
                    </label>

                    <label>
                      <span>許可されたURLホスト</span>
                      <div class="field-row">
                        <input type="text" bind:value={configAllowedUrlHosts} placeholder="例: example.com,api.example.com" />
                      </div>
                    </label>

                    <label>
                      <span>自動スポーンアイテム</span>
                      <div class="field-row">
                        <input type="text" bind:value={configAutoSpawnItems} placeholder="例: resrec:///U-User/R-Item1,resrec:///U-User/R-Item2" />
                      </div>
                    </label>


                    <div class="action-buttons">
                      <button type="button" on:click={openConfigPreview} disabled={configGenerationLoading}>プレビュー</button>
                      <button type="submit" class="save" disabled={configGenerationLoading}>
                        {configGenerationLoading ? '作成中...' : 'コンフィグファイルを作成'}
                      </button>
                    </div>
                  </form>
                </div>
              </div>

              <div class="panel-column">
                <div class="panel-heading">
                  <h2>セッション</h2>
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
                            <input type="text" bind:value={session.sessionName} placeholder="セッション名" />
                          </div>
                        </label>

                        <label>
                          <span>カスタムセッションID</span>
                          <div class="field-row">
                            <input type="text" bind:value={session.customSessionId} placeholder="U-UserID:CustomID" />
                          </div>
                        </label>

                        <label>
                          <span>説明</span>
                          <div class="field-row">
                            <textarea bind:value={session.description} placeholder="セッションの説明" rows="2"></textarea>
                          </div>
                        </label>

                        <label>
                          <span>最大ユーザー数</span>
                          <div class="field-row">
                            <input type="number" bind:value={session.maxUsers} min="1" max="100" />
                          </div>
                        </label>

                        <label>
                          <span>アクセスレベル</span>
                          <div class="field-row">
                            <select bind:value={session.accessLevel}>
                              <option value="Private">Private</option>
                              <option value="LAN">LAN</option>
                              <option value="Contacts">Contacts</option>
                              <option value="ContactsPlus">ContactsPlus</option>
                              <option value="RegisteredUsers">RegisteredUsers</option>
                              <option value="Anyone">Anyone</option>
                            </select>
                          </div>
                        </label>

                        <label>
                          <span>ワールドURL</span>
                          <div class="field-row">
                            <input type="text" bind:value={session.loadWorldURL} placeholder="resrec:///U-UserID/R-RecordID" />
                          </div>
                        </label>

                        <label>
                          <span>ワールドプリセット</span>
                          <div class="field-row">
                            <select bind:value={session.loadWorldPresetName}>
                              <option value="Grid">Grid</option>
                              <option value="Platform">Platform</option>
                              <option value="Blank">Blank</option>
                            </select>
                          </div>
                        </label>

                        <label>
                          <span>タグ</span>
                          <div class="field-row">
                            <input type="text" bind:value={session.tags} placeholder="tag1,tag2,tag3" />
                          </div>
                        </label>

                        <label>
                          <span>デフォルトユーザーロール</span>
                          <div class="field-row">
                            <input type="text" bind:value={session.defaultUserRoles} placeholder="JSON形式で入力してください" />
                          </div>
                        </label>

                        <label>
                          <span>AFKキック時間（分）</span>
                          <div class="field-row">
                            <input type="number" bind:value={session.awayKickMinutes} min="-1" />
                          </div>
                        </label>

                        <label>
                          <span>アイドル再起動間隔（分）</span>
                          <div class="field-row">
                            <input type="number" bind:value={session.idleRestartInterval} min="0" />
                          </div>
                        </label>

                        <div class="checkbox-field">
                          <input type="checkbox" bind:checked={session.isEnabled} />
                          <span>セッションを有効にする</span>
                        </div>

                        <div class="checkbox-field">
                          <input type="checkbox" bind:checked={session.useCustomJoinVerifier} />
                          <span>カスタム参加検証を使用</span>
                        </div>

                        <div class="checkbox-field">
                          <input type="checkbox" bind:checked={session.mobileFriendly} />
                          <span>モバイルフレンドリー</span>
                        </div>

                        <div class="checkbox-field">
                          <input type="checkbox" bind:checked={session.keepOriginalRoles} />
                          <span>元のロールを保持</span>
                        </div>

                        <div class="checkbox-field">
                          <input type="checkbox" bind:checked={session.autoRecover} />
                          <span>自動復旧</span>
                        </div>

                        <div class="checkbox-field">
                          <input type="checkbox" bind:checked={session.saveOnExit} />
                          <span>終了時に保存</span>
                        </div>

                        <div class="checkbox-field">
                          <input type="checkbox" bind:checked={session.autoSleep} />
                          <span>自動スリープ</span>
                        </div>
                      </form>
                    {/if}
                  {/each}
                </div>
              </div>
            </div>

          </section>

          <section class="panel" class:active={activeTab === 'commands'}>
            <div class="panel-grid one">
              <div class="panel-column">
                <div class="panel-heading">
                  <h2>/console</h2>
                </div>
                <div class="card command-card">
                  <h2>コマンドコンソール</h2>
                  <p class="command-help">ヘッドレスが起動している間に直接コマンドを実行できます。</p>
                  <div class="command-input">
                    <input
                      type="text"
                      bind:value={commandText}
                      placeholder="例: worlds"
                      on:keydown={(event) => {
                        if (event.key === 'Enter' && !event.shiftKey) {
                          event.preventDefault();
                          executeCommand();
                        }
                      }}
                      disabled={!$status.running || commandLoading}
                    />
                    <button type="button" on:click={executeCommand} disabled={!$status.running || commandLoading || !commandText.trim()}>
                      実行
                    </button>
                  </div>
                  {#if commandLoading}
                    <p class="command-status">実行中...</p>
                  {:else if commandResult}
                    <pre class="command-result">{commandResult}</pre>
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
</div>

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
    border: 1px solid rgba(255, 255, 255, 0.12);
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

  .field-row button:hover:enabled {
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

  .action-buttons button {
    flex: 1;
    padding: 0.55rem 1.1rem;
    border-radius: 0.65rem;
    border: none;
    color: #f5f5f5;
    font-weight: 600;
    transition: background 0.15s ease;
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

  .action-buttons button.save:hover:enabled {
    background: #2f6d3b;
  }

  .action-buttons button.close:hover:enabled {
    background: #7a404b;
  }

  .action-buttons button.restart:hover:enabled {
    background: #5f4d33;
  }

  .action-buttons button:hover:enabled {
    transform: translateY(-1px);
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

  .field-row button:hover:enabled {
    background: rgba(97, 209, 250, 0.2);
  }

  .status-action-button {
    width: 72px;
    height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: #2b2f35;
    color: #61d1fa;
    border-radius: 0.55rem;
    border: none;
    font-weight: 600;
    font-size: 0.92rem;
    padding: 0;
    transition: background 0.15s ease;
  }

  .status-action-button:hover:enabled {
    background: rgba(97, 209, 250, 0.22);
  }

  .status-action-button:disabled {
    opacity: 0.55;
  }

  .status-action-button.active {
    background: rgba(97, 209, 250, 0.28);
    color: #0b1926;
    border-color: transparent;
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
    background: #59eb5c;
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
    background: #4ddb50;
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

</style>
