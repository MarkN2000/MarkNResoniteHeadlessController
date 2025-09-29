<script lang="ts">
  import { onMount, afterUpdate } from 'svelte';
  import {
    createServerStores,
    getStatus,
    getLogs,
    getConfigs,
    getRuntimeStatus,
    getRuntimeUsers,
    getFriendRequests,
    getRuntimeWorlds,
    postCommand,
    postFocusWorld,
    startServer,
    stopServer,
    type RuntimeStatusData,
    type RuntimeUsersData,
    type FriendRequestsData,
    type RuntimeWorldsData,
    type RuntimeWorldEntry
  } from '$lib';

  const { status, logs, configs, setConfigs, setStatus, setLogs, clearLogs } = createServerStores();

  const tabs = [
    { id: 'dashboard', label: 'ダッシュボード' },
    { id: 'newWorld', label: '新規ワールド' },
    { id: 'friends', label: 'フレンド管理' },
    { id: 'settings', label: '設定' },
    { id: 'commands', label: 'コマンド' }
  ];

  let activeTab: (typeof tabs)[number]['id'] = 'dashboard';

  let initialLoading = true;
  let selectedConfig: string | undefined;
  let errorMessage = '';
  let actionInProgress = false;
  const LOG_DISPLAY_LIMIT = 1000;
  let logContainer: HTMLDivElement | null = null;
  let runtimeStatus: RuntimeStatusData | null = null;
  let runtimeUsers: RuntimeUsersData | null = null;
  let runtimeLoading = false;
  let commandText = '';
  let commandResult = '';
  let commandLoading = false;
  let wasRunning = false;
  let runtimeWorlds: RuntimeWorldsData | null = null;
  let worldsLoading = false;
  let worldsError = '';
  let selectedWorldId: string | null = null;
  let currentWorld: RuntimeWorldEntry | null = null;

  const STORAGE_KEY = 'mrhc:selectedConfig';
  const WORLD_STORAGE_KEY = 'mrhc:selectedWorldId';

  let templateName = '';
  let templateMessage = '';
  let templateSuccess = false;
  let templateLoading = false;

  let worldUrl = '';
  let worldUrlMessage = '';
  let worldUrlSuccess = false;
  let worldUrlLoading = false;

  let friendSendMessage = '';
  let friendSendSuccess = false;
  let friendSendLoading = false;

  let friendAcceptMessage = '';
  let friendAcceptSuccess = false;
  let friendAcceptLoading = false;

  let friendRemoveMessage = '';
  let friendRemoveSuccess = false;
  let friendRemoveLoading = false;

  let friendTargetName = '';
  let friendMessageText = '';
  let friendMessageFeedback = '';
  let friendMessageSuccess = false;
  let friendMessageLoading = false;
  let friendRequests: FriendRequestsData | null = null;
  let friendRequestsLoading = false;
  let friendRequestsError = '';

  const ROLE_OPTIONS = ['Admin', 'Builder', 'Moderator', 'Guest', 'Spectator'];
  const USER_ACTIONS = [
    { key: 'silence', label: 'ミュート' },
    { key: 'unsilence', label: 'ボイス許可' },
    { key: 'respawn', label: 'リスポーン' },
    { key: 'kick', label: 'キック' },
    { key: 'ban', label: 'BAN' }
  ] as const;

  const getActionLabel = (key: UserActionDefinition['key']) =>
    USER_ACTIONS.find(action => action.key === key)?.label ?? key;

  const DEFAULT_ACCESS_LEVELS = ['Public', 'LAN', 'Friends', 'Private', 'Hidden'];

  const resourceMetrics = [
    { label: 'CPU', value: '--- %' },
    { label: 'メモリ', value: '--- GB' }
  ];

  let accessLevelOptions = [...DEFAULT_ACCESS_LEVELS];
  let sessionNameInput = '';
  let sessionDescriptionInput = '';
  let maxUsersInput = '';
  let accessLevelInput = '';
  let hiddenFromListingInput = false;
  let statusActionLoading: Record<string, boolean> = {};
  let statusActionFeedback: Record<string, { message: string; success: boolean } | undefined> = {};

  type UserActionDefinition = (typeof USER_ACTIONS)[number];

  let userRoleSelections: Record<string, string> = {};
  let userActionLoading: Record<string, boolean> = {};
  let userActionFeedback: Record<string, { message: string; success: boolean } | undefined> = {};

  const setStatusLoading = (key: string, value: boolean) => {
    statusActionLoading = { ...statusActionLoading, [key]: value };
  };

  const setStatusFeedback = (key: string, message: string, success: boolean) => {
    statusActionFeedback = {
      ...statusActionFeedback,
      [key]: { message, success }
    };
  };

  afterUpdate(() => {
    if (logContainer) {
      logContainer.scrollTop = logContainer.scrollHeight;
    }
  });

  $: if ($status.running && !wasRunning) {
    Promise.all([refreshWorlds(), refreshRuntimeInfo()]);
  }

  $: wasRunning = $status.running;

  $: currentWorld = runtimeWorlds?.data.find(world => world.sessionId === selectedWorldId) ?? null;

  const refreshWorlds = async (suppressError = false) => {
    worldsLoading = true;
    if (!suppressError) worldsError = '';
    try {
      const worlds = await getRuntimeWorlds();
      runtimeWorlds = worlds;
      const focused = worlds.data.find(world => world.focused);
      if (focused) {
        selectedWorldId = focused.sessionId;
      } else if (selectedWorldId) {
        const exists = worlds.data.some(world => world.sessionId === selectedWorldId);
        if (!exists) {
          selectedWorldId = worlds.data[0]?.sessionId ?? null;
        }
      } else {
        selectedWorldId = worlds.data[0]?.sessionId ?? null;
      }
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
    } finally {
      worldsLoading = false;
    }
  };

  const refreshRuntimeInfo = async (suppressError = false) => {
    runtimeLoading = true;
    try {
      runtimeStatus = await getRuntimeStatus();
      runtimeUsers = await getRuntimeUsers();
      if (runtimeUsers?.data) {
        const nextSelections: Record<string, string> = {};
        runtimeUsers.data.forEach(user => {
          nextSelections[user.name] = userRoleSelections[user.name] ?? user.role;
        });
        userRoleSelections = nextSelections;
      }
      if (runtimeStatus?.data) {
        sessionNameInput = runtimeStatus.data.name ?? '';
        sessionDescriptionInput = runtimeStatus.data.description ?? '';
        maxUsersInput = runtimeStatus.data.maxUsers !== undefined ? String(runtimeStatus.data.maxUsers) : '';
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
        maxUsersInput = '';
        accessLevelInput = '';
        hiddenFromListingInput = false;
        accessLevelOptions = [...DEFAULT_ACCESS_LEVELS];
      }
    } catch (error) {
      runtimeStatus = null;
      runtimeUsers = null;
      if (!suppressError) {
        errorMessage = error instanceof Error ? error.message : 'ランタイム情報を取得できませんでした';
      }
    } finally {
      runtimeLoading = false;
    }
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

  const loadInitialData = async () => {
    initialLoading = true;
    try {
      const configList = await getConfigs();
      setConfigs(configList);
      const storedConfig = localStorage.getItem(STORAGE_KEY);
      const defaultConfig = configList.find(item => item.path === storedConfig) ?? configList[0];
      selectedConfig = defaultConfig?.path;
      if (selectedConfig) {
        localStorage.setItem(STORAGE_KEY, selectedConfig);
      }

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
        maxUsersInput = '';
        accessLevelInput = '';
        hiddenFromListingInput = false;
        accessLevelOptions = [...DEFAULT_ACCESS_LEVELS];
      }
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : '初期データの取得に失敗しました';
    } finally {
      initialLoading = false;
    }
  };

  onMount(() => {
    loadInitialData();
  });

  const sendUserAction = async (username: string, action: UserActionDefinition['key']) => {
    if (!username) return;
    userActionLoading = { ...userActionLoading, [`${username}-${action}`]: true };
    userActionFeedback = { ...userActionFeedback, [username]: undefined };
    try {
      await postCommand(`${action} "${username}"`);
      userActionFeedback = {
        ...userActionFeedback,
        [username]: { message: `${getActionLabel(action)} を送信しました`, success: true }
      };
      if ($status.running) {
        await Promise.all([refreshRuntimeInfo(true), refreshWorlds(true)]);
      }
    } catch (error) {
      userActionFeedback = {
        ...userActionFeedback,
        [username]: {
          message: error instanceof Error ? error.message : 'コマンド送信に失敗しました',
          success: false
        }
      };
    } finally {
      userActionLoading = { ...userActionLoading, [`${username}-${action}`]: false };
    }
  };

  const updateUserRole = async (username: string, role: string) => {
    if (!username || !role) return;
    userActionLoading = { ...userActionLoading, [`${username}-role`]: true };
    userActionFeedback = { ...userActionFeedback, [username]: undefined };
    try {
      await postCommand(`role "${username}" "${role}"`);
      userActionFeedback = {
        ...userActionFeedback,
        [username]: { message: `ロールを ${role} に変更しました`, success: true }
      };
      if ($status.running) {
        await refreshRuntimeInfo(true);
      }
    } catch (error) {
      userActionFeedback = {
        ...userActionFeedback,
        [username]: {
          message: error instanceof Error ? error.message : 'ロール変更に失敗しました',
          success: false
        }
      };
    } finally {
      userActionLoading = { ...userActionLoading, [`${username}-role`]: false };
    }
  };

  const sendStatusCommand = async (key: string, command: string, successMessage: string) => {
    if (!$status.running) {
      setStatusFeedback(key, 'サーバーが起動していません', false);
      return;
    }
    setStatusLoading(key, true);
    setStatusFeedback(key, '', true);
    try {
      await postCommand(command);
      setStatusFeedback(key, successMessage, true);
      await refreshRuntimeInfo(true);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
      setStatusFeedback(key, message, false);
    } finally {
      setStatusLoading(key, false);
    }
  };

  const applySessionName = async () => {
    const value = sessionNameInput.trim();
    if (!value) {
      setStatusFeedback('name', 'セッション名を入力してください', false);
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
        setStatusFeedback('sessionId', 'SessionIDをコピーしました', true);
      } else {
        setStatusFeedback('sessionId', 'クリップボードに対応していません', false);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'コピーに失敗しました';
      setStatusFeedback('sessionId', message, false);
    }
  };

  const applyMaxUsers = async () => {
    if (!maxUsersInput.trim()) {
      setStatusFeedback('maxUsers', '最大人数を入力してください', false);
      return;
    }
    const parsed = Number(maxUsersInput);
    if (!Number.isFinite(parsed) || parsed < 0) {
      setStatusFeedback('maxUsers', '0以上の数値を入力してください', false);
      return;
    }
    await sendStatusCommand('maxUsers', `maxusers ${parsed}`, '最大人数を更新しました');
  };

  const applyAccessLevel = async (value?: string) => {
    const nextLevel = value ?? accessLevelInput;
    if (!nextLevel) {
      setStatusFeedback('accessLevel', 'アクセスレベルを選択してください', false);
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

  const executeWorldCommand = async (command: 'save' | 'close') => {
    const successMessage = command === 'save' ? 'ワールドを保存しました' : 'ワールドを閉じるコマンドを送信しました';
    await sendStatusCommand(command, command, successMessage);
  };

  const handleStart = async () => {
    if (actionInProgress) return;
    actionInProgress = true;
    try {
      await startServer(selectedConfig);
      await Promise.all([refreshWorlds(), refreshRuntimeInfo(), refreshConfigsOnly()]);
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : 'サーバーを起動できませんでした';
    } finally {
      actionInProgress = false;
    }
  };

  const handleStop = async () => {
    if (actionInProgress) return;
    actionInProgress = true;
    try {
      await stopServer();
      runtimeStatus = null;
      runtimeUsers = null;
      runtimeWorlds = null;
      await refreshConfigsOnly();
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : 'サーバーを停止できませんでした';
    } finally {
      actionInProgress = false;
    }
  };

  const submitTemplate = async () => {
    if (templateLoading) return;
    templateLoading = true;
    try {
      await postCommand(`/template ${templateName}`);
      templateMessage = 'テンプレートを起動しました。';
      templateSuccess = true;
    } catch (error) {
      templateMessage = error instanceof Error ? error.message : 'テンプレートを起動できませんでした';
      templateSuccess = false;
    } finally {
      templateLoading = false;
    }
  };

  const submitWorldUrl = async () => {
    if (worldUrlLoading) return;
    worldUrlLoading = true;
    try {
      await postCommand(`/url ${worldUrl}`);
      worldUrlMessage = 'URLから起動しました。';
      worldUrlSuccess = true;
    } catch (error) {
      worldUrlMessage = error instanceof Error ? error.message : 'URLから起動できませんでした';
      worldUrlSuccess = false;
    } finally {
      worldUrlLoading = false;
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
      await postCommand(`/friend send ${friendTargetName}`);
      friendSendMessage = 'フレンド申請を送りました。';
      friendSendSuccess = true;
    } catch (error) {
      friendSendMessage = error instanceof Error ? error.message : 'フレンド申請を送れませんでした';
      friendSendSuccess = false;
    } finally {
      friendSendLoading = false;
    }
  };

  const submitFriendAccept = async () => {
    if (friendAcceptLoading) return;
    friendAcceptLoading = true;
    try {
      await postCommand(`/friend accept ${friendTargetName}`);
      friendAcceptMessage = 'フレンド申請を承認しました。';
      friendAcceptSuccess = true;
    } catch (error) {
      friendAcceptMessage = error instanceof Error ? error.message : 'フレンド申請を承認できませんでした';
      friendAcceptSuccess = false;
    } finally {
      friendAcceptLoading = false;
    }
  };

  const submitFriendRemove = async () => {
    if (friendRemoveLoading) return;
    friendRemoveLoading = true;
    try {
      await postCommand(`/friend remove ${friendTargetName}`);
      friendRemoveMessage = 'フレンドを解除しました。';
      friendRemoveSuccess = true;
    } catch (error) {
      friendRemoveMessage = error instanceof Error ? error.message : 'フレンドを解除できませんでした';
      friendRemoveSuccess = false;
    } finally {
      friendRemoveLoading = false;
    }
  };

  const submitFriendMessage = async () => {
    if (friendMessageLoading) return;
    friendMessageLoading = true;
    try {
      await postCommand(`/friend message ${friendTargetName} ${friendMessageText}`);
      friendMessageFeedback = 'メッセージを送信しました。';
      friendMessageSuccess = true;
    } catch (error) {
      friendMessageFeedback = error instanceof Error ? error.message : 'メッセージを送信できませんでした';
      friendMessageSuccess = false;
    } finally {
      friendMessageLoading = false;
    }
  };

  const executeCommand = async () => {
    if (commandLoading) return;
    commandLoading = true;
    try {
      commandResult = await postCommand(commandText);
    } catch (error) {
      commandResult = error instanceof Error ? error.message : 'コマンドを実行できませんでした';
    } finally {
      commandLoading = false;
    }
  };

  const focusWorld = async (world: RuntimeWorldEntry) => {
    if (!$status.running) return;
    worldsError = '';
    try {
      const target = world.focusTarget ?? world.sessionId;
      await postFocusWorld(target);
      selectedWorldId = world.sessionId;
      localStorage.setItem(WORLD_STORAGE_KEY, world.sessionId);
      await Promise.all([refreshRuntimeInfo(), refreshWorlds()]);
    } catch (error) {
      worldsError = error instanceof Error ? error.message : 'フォーカスに失敗しました';
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
      </div>
    </div>
    <div class="topbar-controls">
      <label class="field">
        <span>プロファイル選択</span>
        <select bind:value={selectedConfig} disabled={$status.running || actionInProgress}>
          {#each $configs as config}
            <option value={config.path}>{config.name}</option>
          {/each}
        </select>
      </label>
      {#each resourceMetrics as metric}
        <div class="resource-capsule">
          <span>{metric.label}</span>
          <strong>{metric.value}</strong>
        </div>
      {/each}
      <div class="status-indicators">
        <div class={$status.running ? 'online' : 'offline'}>
          <span class="dot"></span>
          {$status.running ? '起動中' : '停止中'}
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
          <button type="button" on:click={() => refreshWorlds()} disabled={!$status.running || worldsLoading}>
            手動更新
          </button>
        </div>
        {#if worldsError}
          <p class="feedback">{worldsError}</p>
        {/if}
        <div class="session-list">
          {#if runtimeWorlds?.data?.length}
            {#each runtimeWorlds.data as world}
              <button
                type="button"
                class="session"
                class:active={selectedWorldId === world.sessionId}
                class:focused={world.focused}
                on:click={() => focusWorld(world)}
                disabled={worldsLoading || !$status.running}
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
        <div class="log-container" bind:this={logContainer}>
          {#if !$logs.length}
            <p class="empty">まだログがありません。</p>
          {:else}
            {#each $logs.slice(-LOG_DISPLAY_LIMIT) as log}
              <div class:stderr={log.level === 'stderr'}>
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
        {#if errorMessage}
          <div class="panel notice error">{errorMessage}</div>
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
              <div class="card status-card" aria-busy={runtimeLoading}>
                <div class="section-header">
                  <h2>/status</h2>
                  <button type="button" on:click={refreshRuntimeInfo} disabled={!$status.running || runtimeLoading}>
                    再取得
                  </button>
                </div>
                {#if !$status.running}
                  <p class="empty">サーバーが起動すると状態が表示されます。</p>
                {:else if runtimeStatus}
                  <form class="status-form" on:submit|preventDefault={() => {}}>
                    <label>
                      <span>セッション名</span>
                      <div class="field-row">
                        <input type="text" bind:value={sessionNameInput} />
                        <button type="button" on:click={applySessionName} disabled={statusActionLoading.name}>
                          適用
                        </button>
                      </div>
                      {#if statusActionFeedback.name}
                        <span class="feedback" class:success={statusActionFeedback.name.success}>{statusActionFeedback.name.message}</span>
                      {/if}
                    </label>

                    <label>
                      <span>SessionID</span>
                      <div class="field-row">
                        <input class="muted" type="text" value={runtimeStatus.data.sessionId ?? ''} readonly />
                        <button type="button" on:click={copySessionId}>
                          コピー
                        </button>
                      </div>
                      {#if statusActionFeedback.sessionId}
                        <span class="feedback" class:success={statusActionFeedback.sessionId.success}>
                          {statusActionFeedback.sessionId.message}
                        </span>
                      {/if}
                    </label>

                    <label>
                      <span>人数 (Present / Users / Max)</span>
                      <div class="field-row">
                        <span class="muted">{runtimeStatus.data.presentUsers ?? '-'}</span>
                        <span class="slash">/</span>
                        <span class="muted">{runtimeStatus.data.currentUsers ?? '-'}</span>
                        <span class="slash">/</span>
                        <input type="number" min="0" bind:value={maxUsersInput} />
                        <button type="button" on:click={applyMaxUsers} disabled={statusActionLoading.maxUsers}>
                          適用
                        </button>
                      </div>
                      {#if statusActionFeedback.maxUsers}
                        <span class="feedback" class:success={statusActionFeedback.maxUsers.success}>{statusActionFeedback.maxUsers.message}</span>
                      {/if}
                    </label>

                    <label>
                      <span>アクセスレベル</span>
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
                      {#if statusActionFeedback.accessLevel}
                        <span class="feedback" class:success={statusActionFeedback.accessLevel.success}>{statusActionFeedback.accessLevel.message}</span>
                      {/if}
                    </label>

                    <label class="checkbox-field">
                      <input
                        type="checkbox"
                        checked={hiddenFromListingInput}
                        on:change={(event) => handleHiddenFromListingChange((event.target as HTMLInputElement).checked)}
                        disabled={statusActionLoading.hidden}
                      />
                      <span>リスト非表示にする</span>
                    </label>
                    {#if statusActionFeedback.hidden}
                      <span class="feedback" class:success={statusActionFeedback.hidden.success}>{statusActionFeedback.hidden.message}</span>
                    {/if}

                    <label>
                      <span>説明</span>
                      <textarea rows="4" bind:value={sessionDescriptionInput}></textarea>
                      <div class="field-row end">
                        <button type="button" on:click={applyDescription} disabled={statusActionLoading.description}>
                          適用
                        </button>
                      </div>
                      {#if statusActionFeedback.description}
                        <span class="feedback" class:success={statusActionFeedback.description.success}>
                          {statusActionFeedback.description.message}
                        </span>
                      {/if}
                    </label>

                    <div class="action-buttons">
                      <button type="button" on:click={() => executeWorldCommand('save')} disabled={statusActionLoading.save}>
                        ワールドを保存
                      </button>
                      <button type="button" on:click={() => executeWorldCommand('close')} disabled={statusActionLoading.close}>
                        ワールドを閉じる
                      </button>
                    </div>
                  </form>
                {:else}
                  <p class="empty">読み込み中...</p>
                {/if}
              </div>

              <div class="card users-card" aria-busy={runtimeLoading}>
                <div class="section-header">
                  <h2>/users</h2>
                </div>
                {#if !$status.running}
                  <p class="empty">サーバーが起動するとユーザーが表示されます。</p>
                {:else if runtimeUsers}
                  {#if runtimeUsers.data.length}
                    <table>
                      <thead>
                        <tr>
                          <th>ユーザー</th>
                          <th>在席</th>
                          <th>Role</th>
                          <th>Silenced</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {#each runtimeUsers.data as user}
                          <tr>
                            <td>
                              <strong>{user.name}</strong>
                              <span class="sub">{user.id}</span>
                              {#if userActionFeedback[user.name]}
                                <span
                                  class="feedback"
                                  class:success={userActionFeedback[user.name]?.success}
                                >
                                  {userActionFeedback[user.name]?.message}
                                </span>
                              {/if}
                            </td>
                            <td>{user.present ? '在席' : '離席'}</td>
                            <td>
                              <select
                                value={userRoleSelections[user.name] ?? user.role}
                                on:change={(event) => {
                                  const value = (event.target as HTMLSelectElement).value;
                                  userRoleSelections = { ...userRoleSelections, [user.name]: value };
                                  updateUserRole(user.name, value);
                                }}
                                disabled={userActionLoading[`${user.name}-role`]}
                              >
                                {#each ROLE_OPTIONS as option}
                                  <option value={option}>{option}</option>
                                {/each}
                              </select>
                            </td>
                            <td>
                              <select
                                value={user.silenced ? 'silence' : 'unsilence'}
                                on:change={(event) => {
                                  const value = (event.target as HTMLSelectElement).value as 'silence' | 'unsilence';
                                  sendUserAction(user.name, value);
                                }}
                                disabled={userActionLoading[`${user.name}-silence`] || userActionLoading[`${user.name}-unsilence`]}
                              >
                                <option value="silence">ミュート</option>
                                <option value="unsilence">ボイス許可</option>
                              </select>
                            </td>
                            <td>
                              <div class="user-actions">
                                {#each USER_ACTIONS as action}
                                  {#if action.key === 'silence' || action.key === 'unsilence'}
                                  {:else}
                                    <button
                                      type="button"
                                      on:click={() => sendUserAction(user.name, action.key)}
                                      disabled={userActionLoading[`${user.name}-${action.key}`]}
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
          </section>

          <section class="panel" class:active={activeTab === 'newWorld'}>
            <div class="panel-grid two">
              <div class="card form-card">
                <h2>テンプレートから起動</h2>
                <form on:submit|preventDefault={submitTemplate}>
                  <label class="field">
                    <span>テンプレート名</span>
                    <input type="text" bind:value={templateName} placeholder="例: Grid" />
                  </label>
                  <button type="submit" disabled={!$status.running || templateLoading}>送信</button>
                  {#if templateMessage}
                    <p class="feedback" class:success={templateSuccess}>{templateMessage}</p>
                  {/if}
                </form>
              </div>
              <div class="card form-card">
                <h2>URLから起動</h2>
                <form on:submit|preventDefault={submitWorldUrl}>
                  <label class="field">
                    <span>Resonite URL</span>
                    <input type="text" bind:value={worldUrl} placeholder="resrec://..." />
                  </label>
                  <button type="submit" disabled={!$status.running || worldUrlLoading}>送信</button>
                  {#if worldUrlMessage}
                    <p class="feedback" class:success={worldUrlSuccess}>{worldUrlMessage}</p>
                  {/if}
                </form>
              </div>
            </div>
          </section>

          <section class="panel" class:active={activeTab === 'friends'}>
            <div class="panel-grid one">
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

            <div class="panel-grid two">
              <div class="card form-card">
                <h2>フレンド申請を送る</h2>
                <form on:submit|preventDefault={submitFriendSend}>
                  <button type="submit" disabled={!$status.running || friendSendLoading}>送信</button>
                  {#if friendSendMessage}
                    <p class="feedback" class:success={friendSendSuccess}>{friendSendMessage}</p>
                  {/if}
                </form>
              </div>
              <div class="card form-card">
                <h2>申請を承認する</h2>
                <form on:submit|preventDefault={submitFriendAccept}>
                  <button type="submit" disabled={!$status.running || friendAcceptLoading}>承認</button>
                  {#if friendAcceptMessage}
                    <p class="feedback" class:success={friendAcceptSuccess}>{friendAcceptMessage}</p>
                  {/if}
                </form>
              </div>
              <div class="card form-card">
                <h2>フレンド解除</h2>
                <form on:submit|preventDefault={submitFriendRemove}>
                  <button type="submit" disabled={!$status.running || friendRemoveLoading}>解除</button>
                  {#if friendRemoveMessage}
                    <p class="feedback" class:success={friendRemoveSuccess}>{friendRemoveMessage}</p>
                  {/if}
                </form>
              </div>
              <div class="card form-card">
                <h2>メッセージ送信</h2>
                <form on:submit|preventDefault={submitFriendMessage}>
                  <label class="field">
                    <span>メッセージ</span>
                    <textarea rows="3" bind:value={friendMessageText}></textarea>
                  </label>
                  <button type="submit" disabled={!$status.running || friendMessageLoading}>送信</button>
                  {#if friendMessageFeedback}
                    <p class="feedback" class:success={friendMessageSuccess}>{friendMessageFeedback}</p>
                  {/if}
                </form>
              </div>
            </div>
          </section>

          <section class="panel" class:active={activeTab === 'settings'}>
            <div class="panel-grid one">
              <div class="card form-card">
                <h2>設定ファイル管理</h2>
                <p>新しく追加した設定ファイルはこのボタンで一覧を更新できます。</p>
                <button type="button" on:click={refreshConfigsOnly}>設定ファイルを再取得</button>
                <p class="info">現在の件数: {$configs.length}</p>
              </div>
            </div>
          </section>

          <section class="panel" class:active={activeTab === 'commands'}>
            <div class="panel-grid one">
              <div class="card command-card">
                <div class="section-header">
                  <h2>コマンドコンソール</h2>
                </div>
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
          </section>
        </div>
      {/if}
    </main>
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
  }

  .topbar-controls .field {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    font-size: 0.85rem;
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
    color: #ffffff;
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
    gap: 1rem;
  }

  .sidebar h2 {
    font-size: 1rem;
    color: #61d1fa;
    margin: 0;
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
    color: #61d1fa;
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
    background: #61d1fa;
    color: #11151d;
    box-shadow: 0 0 12px rgba(97, 209, 250, 0.35);
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
    border-color: rgba(255, 118, 118, 0.45);
    color: #ff7676;
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

  .logs-card .log-container div.stderr {
    border-left: 2px solid #ff7676;
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
    background: rgba(17, 21, 29, 0.65);
    border-radius: 0.6rem;
    padding: 0.9rem;
    font-family: 'JetBrains Mono', monospace;
    font-size: 0.85rem;
    white-space: pre-wrap;
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

  .feedback {
    font-size: 0.85rem;
    color: #ff7676;
    margin: 0;
  }

  .feedback.success {
    color: #59eb5c;
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
    border: 1px solid rgba(97, 209, 250, 0.3);
    background: #2b2f35;
    color: #61d1fa;
    font-weight: 600;
    font-size: 0.8rem;
  }

  .field-row button:hover:enabled {
    background: #34404c;
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
    background: #2b2f35;
    color: #61d1fa;
    font-weight: 600;
    transition: background 0.15s ease, transform 0.15s ease;
  }

  .action-buttons button:hover:enabled {
    background: #34404c;
  }

  .status-card button,
  .status-card input,
  .status-card textarea,
  .runtime-card button,
  .command-card button {
    padding: 0.55rem 1.1rem;
    border-radius: 0.65rem;
    border: 1px solid rgba(97, 209, 250, 0.35);
    background: #2b2f35;
    color: #61d1fa;
    font-weight: 600;
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
    display: grid;
    gap: 1rem;
  }

  .status-card label {
    display: grid;
    gap: 0.5rem;
    font-size: 0.85rem;
  }

  .status-card label span {
    color: #9aa3b3;
    font-weight: 600;
  }

  .status-card .field-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .status-card .field-row .slash {
    color: #6a7286;
  }

  .status-card .field-row.end {
    justify-content: flex-end;
  }

  .status-card .checkbox-field {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    color: #cfd6e4;
  }

  .status-card .checkbox-field input {
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
</style>
