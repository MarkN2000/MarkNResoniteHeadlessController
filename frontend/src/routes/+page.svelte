<script lang="ts">
  import { onMount, afterUpdate } from 'svelte';
  import {
    createServerStores,
    getStatus,
    getLogs,
    getConfigs,
    getRuntimeStatus,
    getRuntimeUsers,
    postCommand,
    startServer,
    stopServer,
    type RuntimeStatusData,
    type RuntimeUsersData
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
  let autoScroll = true;
  let maxDisplayLogs = 500;
  let logContainer: HTMLDivElement | null = null;
  let runtimeStatus: RuntimeStatusData | null = null;
  let runtimeUsers: RuntimeUsersData | null = null;
  let runtimeLoading = false;
  let commandText = '';
  let commandResult = '';
  let commandLoading = false;
  let wasRunning = false;

  const STORAGE_KEY = 'mrhc:selectedConfig';

  let templateName = '';
  let templateMessage = '';
  let templateSuccess = false;
  let templateLoading = false;

  let worldUrl = '';
  let worldUrlMessage = '';
  let worldUrlSuccess = false;
  let worldUrlLoading = false;

  let friendSendName = '';
  let friendSendMessage = '';
  let friendSendSuccess = false;
  let friendSendLoading = false;

  let friendAcceptName = '';
  let friendAcceptMessage = '';
  let friendAcceptSuccess = false;
  let friendAcceptLoading = false;

  let friendRemoveName = '';
  let friendRemoveMessage = '';
  let friendRemoveSuccess = false;
  let friendRemoveLoading = false;

  let friendMessageName = '';
  let friendMessageText = '';
  let friendMessageFeedback = '';
  let friendMessageSuccess = false;
  let friendMessageLoading = false;

  const resourceMetrics = [
    { label: 'CPU', value: '--- %' },
    { label: 'メモリ', value: '--- GB' }
  ];

  let runtimeSessions: Array<{ name: string; sessionId: string; users: number; maxUsers: number }> = [];

  $: runtimeSessions = runtimeStatus?.data?.name
    ? [
        {
          name: runtimeStatus.data.name ?? '未取得',
          sessionId: runtimeStatus.data.sessionId ?? 'N/A',
          users: runtimeStatus.data.currentUsers ?? 0,
          maxUsers: runtimeStatus.data.maxUsers ?? 0
        }
      ]
    : [];

  const refreshInitialData = async () => {
    try {
      const [statusValue, logEntries, configList] = await Promise.all([
        getStatus(),
        getLogs(maxDisplayLogs),
        getConfigs()
      ]);
      setStatus(statusValue);
      setLogs(logEntries);
      setConfigs(configList);
      const storedConfig = localStorage.getItem(STORAGE_KEY);
      const defaultConfig = configList.find(item => item.path === storedConfig) ?? configList[0];
      selectedConfig = defaultConfig?.path;
      if ($status.running) {
        await refreshRuntimeInfo();
      }
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : '初期データ取得に失敗しました';
    } finally {
      initialLoading = false;
    }
  };

  const refreshConfigsOnly = async () => {
    try {
      const configList = await getConfigs();
      setConfigs(configList);
      if (configList.length && !configList.some(item => item.path === selectedConfig)) {
        selectedConfig = configList[0].path;
      }
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : '設定ファイルの取得に失敗しました';
    }
  };

  onMount(() => {
    refreshInitialData();
  });

  afterUpdate(() => {
    if (autoScroll && logContainer) {
      logContainer.scrollTop = logContainer.scrollHeight;
    }
  });

  const handleStart = async () => {
    actionInProgress = true;
    errorMessage = '';
    try {
      await startServer(selectedConfig);
      if (selectedConfig) {
        localStorage.setItem(STORAGE_KEY, selectedConfig);
      }
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : 'サーバー起動に失敗しました';
    } finally {
      actionInProgress = false;
    }
  };

  const handleStop = async () => {
    actionInProgress = true;
    errorMessage = '';
    try {
      await stopServer();
      runtimeStatus = null;
      runtimeUsers = null;
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : 'サーバー停止に失敗しました';
    } finally {
      actionInProgress = false;
    }
  };

  const refreshRuntimeInfo = async () => {
    if (!$status.running) {
      runtimeStatus = null;
      runtimeUsers = null;
      return;
    }
    runtimeLoading = true;
    try {
      const [statusResult, usersResult] = await Promise.all([getRuntimeStatus(), getRuntimeUsers()]);
      runtimeStatus = statusResult;
      runtimeUsers = usersResult;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'ランタイム情報の取得に失敗しました';
      runtimeStatus = { raw: message, data: { tags: [], users: [] } };
      runtimeUsers = { raw: '', data: [] };
    } finally {
      runtimeLoading = false;
    }
  };

  const executeCommand = async () => {
    const trimmed = commandText.trim();
    commandResult = '';
    if (!trimmed) return;
    commandLoading = true;
    try {
      const response = await postCommand(trimmed);
      commandResult = response.raw || '(出力なし)';
      commandText = '';
      if ($status.running) {
        await refreshRuntimeInfo();
      }
    } catch (error) {
      commandResult = error instanceof Error ? error.message : 'コマンド実行中にエラーが発生しました';
    } finally {
      commandLoading = false;
    }
  };

  const submitTemplate = async () => {
    const value = templateName.trim();
    templateMessage = '';
    templateSuccess = false;
    if (!value) {
      templateMessage = 'テンプレート名を入力してください';
      return;
    }
    templateLoading = true;
    try {
      await postCommand(`startworldtemplate "${value}"`);
      templateSuccess = true;
      templateMessage = `テンプレート「${value}」で起動コマンドを送信しました。`;
      templateName = '';
    } catch (error) {
      templateSuccess = false;
      templateMessage = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
    } finally {
      templateLoading = false;
    }
  };

  const submitWorldUrl = async () => {
    const value = worldUrl.trim();
    worldUrlMessage = '';
    worldUrlSuccess = false;
    if (!value) {
      worldUrlMessage = 'ResoniteワールドURLを入力してください';
      return;
    }
    worldUrlLoading = true;
    try {
      await postCommand(`startworldurl "${value}"`);
      worldUrlSuccess = true;
      worldUrlMessage = 'URLから起動コマンドを送信しました。';
      worldUrl = '';
    } catch (error) {
      worldUrlSuccess = false;
      worldUrlMessage = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
    } finally {
      worldUrlLoading = false;
    }
  };

  const submitFriendSend = async () => {
    const value = friendSendName.trim();
    friendSendMessage = '';
    friendSendSuccess = false;
    if (!value) {
      friendSendMessage = 'フレンド名を入力してください';
      return;
    }
    friendSendLoading = true;
    try {
      await postCommand(`sendfriendrequest "${value}"`);
      friendSendSuccess = true;
      friendSendMessage = 'フレンドリクエストを送信しました。';
      friendSendName = '';
    } catch (error) {
      friendSendSuccess = false;
      friendSendMessage = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
    } finally {
      friendSendLoading = false;
    }
  };

  const submitFriendAccept = async () => {
    const value = friendAcceptName.trim();
    friendAcceptMessage = '';
    friendAcceptSuccess = false;
    if (!value) {
      friendAcceptMessage = 'フレンド名を入力してください';
      return;
    }
    friendAcceptLoading = true;
    try {
      await postCommand(`acceptfriendrequest "${value}"`);
      friendAcceptSuccess = true;
      friendAcceptMessage = 'フレンドリクエストを承認しました。';
      friendAcceptName = '';
    } catch (error) {
      friendAcceptSuccess = false;
      friendAcceptMessage = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
    } finally {
      friendAcceptLoading = false;
    }
  };

  const submitFriendRemove = async () => {
    const value = friendRemoveName.trim();
    friendRemoveMessage = '';
    friendRemoveSuccess = false;
    if (!value) {
      friendRemoveMessage = 'フレンド名を入力してください';
      return;
    }
    friendRemoveLoading = true;
    try {
      await postCommand(`removefriend "${value}"`);
      friendRemoveSuccess = true;
      friendRemoveMessage = 'フレンドを解除しました。';
      friendRemoveName = '';
    } catch (error) {
      friendRemoveSuccess = false;
      friendRemoveMessage = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
    } finally {
      friendRemoveLoading = false;
    }
  };

  const submitFriendMessage = async () => {
    const name = friendMessageName.trim();
    const text = friendMessageText.trim();
    friendMessageFeedback = '';
    friendMessageSuccess = false;
    if (!name || !text) {
      friendMessageFeedback = 'フレンド名とメッセージを入力してください';
      return;
    }
    friendMessageLoading = true;
    try {
      await postCommand(`message "${name}" "${text}"`);
      friendMessageSuccess = true;
      friendMessageFeedback = 'メッセージを送信しました。';
      friendMessageName = '';
      friendMessageText = '';
    } catch (error) {
      friendMessageSuccess = false;
      friendMessageFeedback = error instanceof Error ? error.message : 'コマンド送信に失敗しました';
    } finally {
      friendMessageLoading = false;
    }
  };

  $: if ($status.running && !wasRunning) {
    refreshRuntimeInfo();
  }

  $: wasRunning = $status.running;
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
        <p>Resoniteヘッドレスをローカルネットワークから快適に管理</p>
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
      <section class="logs-card compact">
        <div class="section-header">
          <h2>ライログ</h2>
        </div>
        <div class="log-container" bind:this={logContainer}>
          {#if !$logs.length}
            <p class="empty">まだログがありません。</p>
          {:else}
            {#each $logs.slice(-maxDisplayLogs) as log}
              <div class:stderr={log.level === 'stderr'}>
                <time>{new Date(log.timestamp).toLocaleTimeString()}</time>
                <pre>{log.message}</pre>
              </div>
            {/each}
          {/if}
        </div>
      </section>

      <section class="session-card">
        <h2>セッション一覧</h2>
        <div class="session-list">
          {#if runtimeSessions.length}
            {#each runtimeSessions as session}
              <div class="session">
                <div>
                  <strong>{session.name}</strong>
                  <span>{session.sessionId}</span>
                </div>
                <div class="counts">{session.users} / {session.maxUsers}</div>
              </div>
            {/each}
          {:else}
            <p class="empty">アクティブなセッションがありません。</p>
          {/if}
          <button type="button" on:click={refreshRuntimeInfo} disabled={!$status.running || runtimeLoading}>
            最新のセッションを取得
          </button>
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
            <div class="panel-grid">
              <div class="card summary-card">
                <h2>設定と操作</h2>
                {#if !$configs.length}
                  <p class="warning">`config/headless` に設定ファイルが見つかりません。JSONファイルを配置してください。</p>
                {/if}
                <label class="field">
                  <span>設定ファイル</span>
                  <select bind:value={selectedConfig} disabled={$status.running || actionInProgress}>
                    {#each $configs as config}
                      <option value={config.path}>{config.name}</option>
                    {/each}
                  </select>
                </label>
                <div class="metrics">
                  <div><span>PID</span><strong>{$status.pid ?? '-'}</strong></div>
                  <div><span>終了コード</span><strong>{$status.exitCode ?? '-'}</strong></div>
                  <div><span>シグナル</span><strong>{$status.signal ?? '-'}</strong></div>
                </div>
                <div class="options">
                  <label>
                    表示件数
                    <select bind:value={maxDisplayLogs}>
                      <option value={200}>200</option>
                      <option value={500}>500</option>
                      <option value={1000}>1000</option>
                    </select>
                  </label>
                  <button type="button" on:click={clearLogs}>ログをクリア</button>
                </div>
              </div>

              <div class="card runtime-card" aria-busy={runtimeLoading}>
                <div class="section-header">
                  <h2>ランタイム情報</h2>
                  <button type="button" on:click={refreshRuntimeInfo} disabled={!$status.running || runtimeLoading}>
                    更新
                  </button>
                </div>
                {#if !$status.running}
                  <p class="empty">サーバーが起動すると状態が表示されます。</p>
                {:else if runtimeStatus}
                  <div class="runtime-grid">
                    <section>
                      <h3>/status</h3>
                      <dl>
                        {#if runtimeStatus.data.name}<div><dt>セッション名</dt><dd>{runtimeStatus.data.name}</dd></div>{/if}
                        {#if runtimeStatus.data.sessionId}<div><dt>SessionID</dt><dd>{runtimeStatus.data.sessionId}</dd></div>{/if}
                        {#if runtimeStatus.data.currentUsers !== undefined}
                          <div><dt>接続ユーザー</dt><dd>{runtimeStatus.data.currentUsers} / {runtimeStatus.data.maxUsers ?? '-'}</dd></div>
                        {/if}
                        {#if runtimeStatus.data.presentUsers !== undefined}
                          <div><dt>在席ユーザー</dt><dd>{runtimeStatus.data.presentUsers}</dd></div>
                        {/if}
                        {#if runtimeStatus.data.uptime}<div><dt>起動時間</dt><dd>{runtimeStatus.data.uptime}</dd></div>{/if}
                        {#if runtimeStatus.data.accessLevel}<div><dt>アクセスレベル</dt><dd>{runtimeStatus.data.accessLevel}</dd></div>{/if}
                        {#if runtimeStatus.data.hiddenFromListing !== undefined}
                          <div><dt>リスト非表示</dt><dd>{runtimeStatus.data.hiddenFromListing ? 'はい' : 'いいえ'}</dd></div>
                        {/if}
                        {#if runtimeStatus.data.mobileFriendly !== undefined}
                          <div><dt>モバイル対応</dt><dd>{runtimeStatus.data.mobileFriendly ? 'はい' : 'いいえ'}</dd></div>
                        {/if}
                        {#if runtimeStatus.data.tags.length}
                          <div><dt>タグ</dt><dd>{runtimeStatus.data.tags.join(', ')}</dd></div>
                        {/if}
                        {#if runtimeStatus.data.users.length}
                          <div><dt>参加ユーザー</dt><dd>{runtimeStatus.data.users.join(', ')}</dd></div>
                        {/if}
                        {#if runtimeStatus.data.description}
                          <div class="full"><dt>説明</dt><dd><pre>{runtimeStatus.data.description}</pre></dd></div>
                        {/if}
                      </dl>
                    </section>
                    <section>
                      <h3>/users</h3>
                      {#if runtimeUsers?.data?.length}
                        <table>
                          <thead>
                            <tr>
                              <th>ユーザー</th>
                              <th>Role</th>
                              <th>在席</th>
                              <th>Ping</th>
                              <th>FPS</th>
                              <th>Silenced</th>
                            </tr>
                          </thead>
                          <tbody>
                            {#each runtimeUsers.data as user}
                              <tr>
                                <td>
                                  <strong>{user.name}</strong>
                                  <span class="sub">{user.id}</span>
                                </td>
                                <td>{user.role}</td>
                                <td>{user.present ? '在席' : '離席'}</td>
                                <td>{user.pingMs}</td>
                                <td>{user.fps}</td>
                                <td>{user.silenced ? 'はい' : 'いいえ'}</td>
                              </tr>
                            {/each}
                          </tbody>
                        </table>
                      {:else}
                        <p class="empty">ユーザー情報が取得できませんでした。</p>
                      {/if}
                    </section>
                  </div>
                {:else}
                  <p class="empty">読み込み中...</p>
                {/if}
              </div>

              <section class="card logs-card">
                <div class="section-header">
                  <h2>ライブログ</h2>
                  <label class="checkbox">
                    <input type="checkbox" bind:checked={autoScroll} />
                    自動スクロール
                  </label>
                </div>
                <div class="log-container" bind:this={logContainer}>
                  {#if !$logs.length}
                    <p class="empty">まだログがありません。</p>
                  {:else}
                    {#each $logs.slice(-maxDisplayLogs) as log}
                      <div class:stderr={log.level === 'stderr'}>
                        <time>{new Date(log.timestamp).toLocaleTimeString()}</time>
                        <pre>{log.message}</pre>
                      </div>
                    {/each}
                  {/if}
                </div>
              </section>
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
            <div class="panel-grid two">
              <div class="card form-card">
                <h2>フレンド申請を送る</h2>
                <form on:submit|preventDefault={submitFriendSend}>
                  <label class="field">
                    <span>フレンド名</span>
                    <input type="text" bind:value={friendSendName} placeholder="ユーザー名" />
                  </label>
                  <button type="submit" disabled={!$status.running || friendSendLoading}>送信</button>
                  {#if friendSendMessage}
                    <p class="feedback" class:success={friendSendSuccess}>{friendSendMessage}</p>
                  {/if}
                </form>
              </div>
              <div class="card form-card">
                <h2>申請を承認する</h2>
                <form on:submit|preventDefault={submitFriendAccept}>
                  <label class="field">
                    <span>フレンド名</span>
                    <input type="text" bind:value={friendAcceptName} placeholder="ユーザー名" />
                  </label>
                  <button type="submit" disabled={!$status.running || friendAcceptLoading}>承認</button>
                  {#if friendAcceptMessage}
                    <p class="feedback" class:success={friendAcceptSuccess}>{friendAcceptMessage}</p>
                  {/if}
                </form>
              </div>
              <div class="card form-card">
                <h2>フレンド解除</h2>
                <form on:submit|preventDefault={submitFriendRemove}>
                  <label class="field">
                    <span>フレンド名</span>
                    <input type="text" bind:value={friendRemoveName} placeholder="ユーザー名" />
                  </label>
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
                    <span>フレンド名</span>
                    <input type="text" bind:value={friendMessageName} placeholder="ユーザー名" />
                  </label>
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
            <div class="panel-grid two">
              <div class="card form-card">
                <h2>ログ表示設定</h2>
                <label class="field">
                  <span>表示件数</span>
                  <select bind:value={maxDisplayLogs}>
                    <option value={200}>200</option>
                    <option value={500}>500</option>
                    <option value={1000}>1000</option>
                  </select>
                </label>
                <button type="button" on:click={clearLogs}>ログをクリア</button>
              </div>
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
    font-size: 1.4rem;
    margin: 0 0 0.25rem;
  }

  .brand p {
    margin: 0;
    color: #86888b;
    font-size: 0.9rem;
  }

  .logo {
    width: 3rem;
    height: 3rem;
    border-radius: 0.75rem;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(135deg, #f8f770, #ba64f2);
    color: #11151d;
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
    padding: 0.55rem 0.75rem;
    border-radius: 0.6rem;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(17, 21, 29, 0.65);
    color: inherit;
  }

  .resource-capsule {
    background: rgba(23, 27, 38, 0.85);
    border-radius: 0.75rem;
    border: 1px solid rgba(97, 209, 250, 0.3);
    padding: 0.5rem 0.75rem;
    display: grid;
    gap: 0.35rem;
    font-size: 0.8rem;
    min-width: 120px;
    text-align: center;
  }

  .resource-capsule span {
    color: #61d1fa;
  }

  .resource-capsule strong {
    font-size: 1rem;
  }

  .status-indicators {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .status-indicators .dot {
    width: 0.75rem;
    height: 0.75rem;
    border-radius: 999px;
    margin-right: 0.35rem;
    display: inline-block;
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
    padding: 0.55rem 1.1rem;
    border-radius: 0.65rem;
    border: 1px solid rgba(97, 209, 250, 0.4);
    background: rgba(97, 209, 250, 0.18);
    color: #e1e1e0;
    font-weight: 600;
  }

  .status-indicators button.danger {
    border-color: rgba(255, 118, 118, 0.45);
    background: rgba(255, 118, 118, 0.2);
  }

  .content {
    display: grid;
    grid-template-columns: 320px 1fr;
    min-height: 0;
  }

  .sidebar {
    background: #171b26;
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
    background: rgba(17, 21, 29, 0.7);
    padding: 0.75rem;
    border-radius: 0.75rem;
    border: 1px solid rgba(97, 209, 250, 0.18);
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    font-size: 0.85rem;
  }

  .session strong {
    display: block;
    margin-bottom: 0.15rem;
  }

  .session span {
    color: #86888b;
    font-size: 0.7rem;
  }

  .session .counts {
    color: #61d1fa;
    font-weight: 600;
  }

  .session-list button {
    padding: 0.5rem 0.75rem;
    border-radius: 0.6rem;
    border: 1px solid rgba(97, 209, 250, 0.3);
    background: rgba(17, 21, 29, 0.7);
    color: #61d1fa;
    font-weight: 600;
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
    padding: 0.6rem 1rem;
    border-radius: 0.75rem;
    border: 1px solid transparent;
    background: rgba(32, 35, 47, 0.85);
    color: #e1e1e0;
    font-weight: 600;
    transition: background 0.15s ease, color 0.15s ease;
  }

  .tab-bar button:hover {
    background: rgba(97, 209, 250, 0.22);
    color: #61d1fa;
  }

  .tab-bar button.active {
    border-color: rgba(186, 100, 242, 0.45);
    background: rgba(186, 100, 242, 0.45);
    color: #11151d;
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
    background: rgba(17, 21, 29, 0.7);
    padding: 0.85rem;
    border-radius: 0.75rem;
    border: 1px solid rgba(97, 209, 250, 0.2);
  }

  .summary-card .metrics span {
    color: #86888b;
    font-size: 0.75rem;
  }

  .summary-card .metrics strong {
    display: block;
    margin-top: 0.35rem;
    font-size: 1.05rem;
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

  .runtime-grid {
    display: grid;
    gap: 1.5rem;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
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
    background: rgba(17, 21, 29, 0.4);
    border-radius: 0.75rem;
    overflow: hidden;
  }

  th,
  td {
    padding: 0.75rem;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
    text-align: left;
  }

  th {
    font-size: 0.75rem;
    color: #86888b;
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
    padding: 0.75rem;
    max-height: 55vh;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    font-size: 0.75rem;
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

  .logs-card time {
    color: #86888b;
    font-size: 0.7rem;
    width: 3.5rem;
    flex: 0 0 auto;
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
    border: 1px solid rgba(97, 209, 250, 0.35);
    background: rgba(17, 21, 29, 0.7);
    color: #61d1fa;
    font-weight: 600;
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
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(17, 21, 29, 0.65);
    color: inherit;
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
