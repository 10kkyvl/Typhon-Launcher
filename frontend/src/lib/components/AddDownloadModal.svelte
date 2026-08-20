<script lang="ts">
  import { untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { Check, FileUp } from '@lucide/svelte';
  import {
    discardMetadata,
    fetchMetadata,
    selectTorrentFile,
    startDownload,
    type TorrentInfo,
  } from '../services/downloads';
  import { selectFolder } from '../services/settings';
  import { errorMessage } from '../stores/downloads';
  import { settings } from '../stores/settings';
  import { toast } from '../stores/toasts';
  import { bytesSize, plural } from '../utils/format';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  let step = $state<'source' | 'loading' | 'files'>('source');
  let source = $state('');
  let info = $state<TorrentInfo | null>(null);
  let destination = $state('');
  let selected = $state<boolean[]>([]);
  let starting = $state(false);
  let pendingHash = '';
  let token = 0;

  const canContinue = $derived(source.trim().startsWith('magnet:'));
  const selectedCount = $derived(selected.filter(Boolean).length);
  const selectedSize = $derived(
    (info?.files ?? []).reduce((sum, file, i) => (selected[i] ? sum + file.size : sum), 0),
  );

  function reset() {
    token++;
    if (pendingHash) {
      discardMetadata(pendingHash);
      pendingHash = '';
    }
    step = 'source';
    source = '';
    info = null;
    selected = [];
    starting = false;
  }

  $effect(() => {
    const isOpen = open;
    untrack(() => {
      reset();
      if (isOpen) destination = get(settings)?.downloadsPath ?? '';
    });
  });

  async function proceed(value: string) {
    const current = ++token;
    step = 'loading';
    try {
      const result = await fetchMetadata(value);
      if (!open || current !== token) {
        discardMetadata(result.infoHash);
        return;
      }
      pendingHash = result.infoHash;
      info = result;
      selected = result.files.map(() => true);
      step = 'files';
    } catch (err) {
      if (!open || current !== token) return;
      toast(errorMessage(err), 'danger');
      step = 'source';
    }
  }

  async function pickTorrent() {
    try {
      const path = await selectTorrentFile();
      if (!path) return;
      source = path;
      await proceed(path);
    } catch (err) {
      toast(errorMessage(err), 'danger');
    }
  }

  async function browseDestination() {
    try {
      const path = await selectFolder('Выберите папку загрузки');
      if (path) destination = path;
    } catch {
      toast('Не удалось открыть диалог выбора папки', 'danger');
    }
  }

  async function start() {
    if (!info) return;
    const indices = selected.map((on, i) => (on ? i : -1)).filter((i) => i >= 0);
    starting = true;
    try {
      await startDownload(info.infoHash, destination, indices);
      pendingHash = '';
      open = false;
    } catch (err) {
      toast(errorMessage(err), 'danger');
    }
    starting = false;
  }
</script>

<Modal bind:open title="Добавить загрузку" width={step === 'files' ? '62rem' : '48rem'}>
  {#if step === 'source'}
    <div class="source">
      <label class="field">
        <span class="field-label">Magnet-ссылка</span>
        <input type="text" placeholder="magnet:?xt=urn:btih:…" bind:value={source} />
      </label>
      <div class="or">
        <span class="line"></span>
        <span class="or-text">или</span>
        <span class="line"></span>
      </div>
      <Button onclick={pickTorrent}>
        <FileUp size="1.6rem" strokeWidth={1.8} />
        Выбрать .torrent файл
      </Button>
    </div>
  {:else if step === 'loading'}
    <div class="loading">
      <span class="spinner"></span>
      <span class="loading-text">Получение метаданных…</span>
      <span class="loading-note">Это может занять до полутора минут.</span>
    </div>
  {:else if info}
    <div class="files">
      <div class="torrent">
        <span class="torrent-name">{info.name}</span>
        <span class="torrent-size">{bytesSize(info.totalBytes)}</span>
      </div>
      <div class="dest">
        <span class="field-label">Папка назначения</span>
        <div class="dest-controls">
          <input type="text" readonly value={destination} />
          <Button size="sm" onclick={browseDestination}>Обзор</Button>
        </div>
      </div>
      <div class="file-list">
        {#each info.files as file, i (i)}
          <button
            class="file-row"
            role="checkbox"
            aria-checked={selected[i]}
            onclick={() => (selected[i] = !selected[i])}
          >
            <span class="box" class:on={selected[i]}>
              {#if selected[i]}<Check size="1.3rem" strokeWidth={2.6} />{/if}
            </span>
            <span class="file-path">{file.path}</span>
            <span class="file-size">{bytesSize(file.size)}</span>
          </button>
        {/each}
      </div>
    </div>
  {/if}

  {#snippet footer()}
    {#if step === 'source'}
      <Button onclick={() => (open = false)}>Отмена</Button>
      <Button variant="primary" disabled={!canContinue} onclick={() => proceed(source.trim())}>Продолжить</Button>
    {:else if step === 'files'}
      <span class="summary">
        Выбрано: {selectedCount}
        {plural(selectedCount, 'файл', 'файла', 'файлов')}, {bytesSize(selectedSize)}
      </span>
      <Button onclick={() => (open = false)}>Отмена</Button>
      <Button variant="primary" disabled={selectedCount === 0 || starting} onclick={start}>Начать загрузку</Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .source {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .field-label {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  input {
    width: 100%;
    height: var(--control-md);
    padding: 0 1.2rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: 1.5rem;
    color: var(--text);
    outline: none;
    transition: border-color var(--dur) var(--ease);
  }

  input:focus {
    border-color: rgba(104, 117, 232, 0.55);
  }

  input[readonly] {
    color: var(--text-2);
  }

  .or {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .line {
    flex: 1;
    height: 1px;
    background: var(--border);
  }

  .or-text {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1.2rem;
    padding: 3.2rem 0;
  }

  .spinner {
    width: 3.2rem;
    height: 3.2rem;
    border-radius: 50%;
    border: 2px solid rgba(255, 255, 255, 0.1);
    border-top-color: var(--accent);
    animation: spin 800ms linear infinite;
  }

  .loading-text {
    font-size: 1.5rem;
    font-weight: 500;
  }

  .loading-note {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .files {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .torrent {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .torrent-name {
    font-size: 1.6rem;
    font-weight: 600;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .torrent-size {
    font-size: 1.4rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }

  .dest {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .dest-controls {
    display: flex;
    gap: 0.8rem;
  }

  .file-list {
    display: flex;
    flex-direction: column;
    max-height: 28rem;
    overflow-y: auto;
    padding: 0.4rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .file-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.7rem 0.8rem;
    border-radius: 0.7rem;
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .file-row:hover {
    background: rgba(255, 255, 255, 0.04);
  }

  .box {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.8rem;
    height: 1.8rem;
    flex-shrink: 0;
    border-radius: 0.5rem;
    border: 1px solid var(--border-strong);
    background: var(--surface-2);
    color: #fff;
    transition:
      background var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease);
  }

  .box.on {
    background: var(--accent);
    border-color: var(--accent);
  }

  .file-path {
    flex: 1;
    min-width: 0;
    font-size: 1.4rem;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-size {
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }

  .summary {
    margin-right: auto;
    align-self: center;
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
