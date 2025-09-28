<script lang="ts">
  import { onMount, afterUpdate } from 'svelte';
  import { createServerStores, getStatus, getLogs, getConfigs, startServer, stopServer } from '$lib';

  const { status, logs, configs, setConfigs, setStatus, setLogs, clearLogs } = createServerStores();

  let initialLoading = true;
  let selectedConfig: string | undefined;
  let errorMessage = '';
  let actionInProgress = false;
  let autoScroll = true;
  let maxDisplayLogs = 500;
  let logContainer: HTMLDivElement | null = null;

  const STORAGE_KEY = 'mrhc:selectedConfig';

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
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : '初期データ取得に失敗しました';
    } finally {
      initialLoading = false;
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
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : 'サーバー停止に失敗しました';
    } finally {
      actionInProgress = false;
    }
  };
</script>

<svelte:head>
  <title>Headless Controller Dashboard</title>
</svelte:head>

<div class="page">
  <div class="header">
    <h1>MarkN Resonite Headless Controller</h1>
    <p>ヘッドレスサーバーの起動・停止とログをブラウザから操作します。</p>
  </div>

  {#if initialLoading}
    <div class="card">
      <p>読み込み中...</p>
    </div>
  {:else}
    {#if errorMessage}
      <div class="card error">
        <p>{errorMessage}</p>
      </div>
    {/if}

    {#if !$configs.length}
      <div class="card warning">
        <p>`config/headless` に設定ファイルが見つかりません。JSONファイルを配置して再度読み込んでください。</p>
      </div>
    {/if}

    <div class="grid">
      <section class="card">
        <h2>サーバー操作</h2>
        <div class="status">
          <span class:running={$status.running} class:stopped={!$status.running}>
            {$status.running ? '起動中' : '停止中'}
          </span>
          {#if $status.pid}
            <span>PID: {$status.pid}</span>
          {/if}
          {#if $status.startedAt}
            <span>起動時刻: {new Date($status.startedAt).toLocaleString()}</span>
          {/if}
          {#if $status.configPath}
            <span>設定ファイル: {$status.configPath}</span>
          {/if}
          {#if $status.exitCode !== undefined}
            <span>終了コード: {$status.exitCode ?? 'null'}</span>
          {/if}
          {#if $status.signal}
            <span>終了シグナル: {$status.signal}</span>
          {/if}
        </div>

        <label class="field">
          <span>設定ファイル</span>
          <select bind:value={selectedConfig} disabled={$status.running || actionInProgress}>
            {#each $configs as config}
              <option value={config.path}>{config.name}</option>
            {/each}
          </select>
        </label>

        <div class="options">
          <label>
            表示件数
            <select bind:value={maxDisplayLogs}>
              <option value={200}>200</option>
              <option value={500}>500</option>
              <option value={1000}>1000</option>
            </select>
          </label>
          <label class="checkbox">
            <input type="checkbox" bind:checked={autoScroll} /> 自動スクロール
          </label>
          <button type="button" on:click={clearLogs}>ログをクリア</button>
        </div>

        <div class="actions">
          <button on:click={handleStart} disabled={$status.running || actionInProgress || !$configs.length}>
            起動
          </button>
          <button on:click={handleStop} disabled={!$status.running || actionInProgress}>
            停止
          </button>
        </div>
      </section>

      <section class="card logs">
        <h2>ライブログ</h2>
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
  {/if}
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
    padding: 2rem;
  }

  .header h1 {
    font-size: 1.8rem;
    margin-bottom: 0.25rem;
  }

  .card {
    background: var(--color-surface-100);
    border-radius: 1rem;
    padding: 1.5rem;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.1);
  }

  .card.error {
    border-left: 4px solid var(--color-error-500);
    color: var(--color-error-500);
  }

  .card.warning {
    border-left: 4px solid var(--color-warning-500);
    color: var(--color-warning-500);
  }

  .grid {
    display: grid;
    gap: 1.5rem;
    grid-template-columns: minmax(0, 360px) minmax(0, 1fr);
  }

  .status {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    margin-bottom: 1rem;
  }

  .status span.running {
    color: var(--color-success-500);
    font-weight: 600;
  }

  .status span.stopped {
    color: var(--color-error-500);
    font-weight: 600;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  select, button {
    padding: 0.5rem 0.75rem;
    border-radius: 0.5rem;
    border: 1px solid var(--color-surface-300);
    background: var(--color-surface-0);
    font-size: 0.9rem;
  }

  select[disabled], button[disabled] {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .options {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin-bottom: 1rem;
    align-items: center;
  }

  .options label {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.85rem;
  }

  .options .checkbox {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }

  .options button {
    min-width: 110px;
  }

  .actions {
    display: flex;
    gap: 1rem;
  }

  button {
    min-width: 96px;
    cursor: pointer;
    transition: background 0.2s;
  }

  button:hover:enabled {
    background: var(--color-secondary-500);
    color: var(--color-surface-900);
  }

  .logs {
    display: flex;
    flex-direction: column;
  }

  .log-container {
    background: var(--color-surface-0);
    border-radius: 0.75rem;
    padding: 0.75rem;
    height: 480px;
    overflow: auto;
    font-family: 'JetBrains Mono', 'Courier New', monospace;
    font-size: 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .log-container div {
    display: flex;
    gap: 0.75rem;
    align-items: flex-start;
  }

  .log-container time {
    color: var(--color-surface-400);
    flex: 0 0 auto;
  }

  .log-container pre {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    flex: 1 1 auto;
  }

  .log-container div.stderr {
    border-left: 3px solid var(--color-error-500);
    padding-left: 0.5rem;
    color: var(--color-error-500);
  }

  .log-container .empty {
    color: var(--color-surface-400);
    text-align: center;
    margin-top: 2rem;
  }

  @media (max-width: 960px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
