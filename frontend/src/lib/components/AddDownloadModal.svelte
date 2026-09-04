<script lang="ts">
  import { untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { Check, FileUp } from '@lucide/svelte';
  import {
    discardMetadata,
    fetchMetadata,
    selectTorrentFile,
    startDownloadFrom,
    type DownloadOrigin,
    type TorrentInfo,
  } from '../services/downloads';
  import { selectFolder } from '../services/settings';
  import { installErrorText } from '../install/installErrors';
  import { settings } from '../stores/settings';
  import { toast } from '../stores/toasts';
  import { bytesSize } from '../utils/format';
  import { msg } from '../i18n';
  import Button from './Button.svelte';
  import LibrarySetupModal from './LibrarySetupModal.svelte';
  import Modal from './Modal.svelte';

  let {
    open = $bindable(false),
    initialSource = '',
    origin = undefined,
  }: { open?: boolean; initialSource?: string; origin?: DownloadOrigin } = $props();

  let step = $state<'source' | 'loading' | 'files'>('source');
  let source = $state('');
  let info = $state<TorrentInfo | null>(null);
  let destination = $state('');
  let selected = $state<boolean[]>([]);
  let starting = $state(false);
  let pendingHash = '';
  let token = 0;

  const libraryReady = $derived(Boolean($settings?.libraryPath));
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
    const preset = initialSource;
    const ready = libraryReady;
    untrack(() => {
      reset();
      if (isOpen && ready) {
        destination = get(settings)?.downloadsPath ?? '';
        if (preset) {
          source = preset;
          proceed(preset);
        }
      }
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
      toast(installErrorText(err), 'danger');
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
      toast(installErrorText(err), 'danger');
    }
  }

  async function browseDestination() {
    try {
      const path = await selectFolder(msg('modals.addDownloadChooseFolder'));
      if (path) destination = path;
    } catch {
      toast(msg('modals.addDownloadFolderDialogFailed'), 'danger');
    }
  }

  async function start() {
    if (!info) return;
    const indices = selected.map((on, i) => (on ? i : -1)).filter((i) => i >= 0);
    starting = true;
    try {
      await startDownloadFrom(info.infoHash, destination, indices, origin ?? {});
      pendingHash = '';
      open = false;
    } catch (err) {
      toast(installErrorText(err), 'danger');
    }
    starting = false;
  }
</script>

{#if !libraryReady}
  <LibrarySetupModal bind:open />
{:else}
<Modal bind:open title={msg('modals.addDownloadTitle')} width={step === 'files' ? '62rem' : '48rem'}>
  {#if step === 'source'}
    <div class="source">
      <label class="field">
        <span class="field-label">{msg('modals.addDownloadMagnetLabel')}</span>
        <input class="input" type="text" placeholder="magnet:?xt=urn:btih:…" bind:value={source} />
      </label>
      <div class="or">
        <span class="line"></span>
        <span class="or-text">{msg('modals.addDownloadOr')}</span>
        <span class="line"></span>
      </div>
      <Button onclick={pickTorrent}>
        <FileUp size="1.6rem" strokeWidth={1.8} />
        {msg('modals.addDownloadPickTorrent')}
      </Button>
    </div>
  {:else if step === 'loading'}
    <div class="loading">
      <span class="spinner"></span>
      <span class="loading-text">{msg('modals.addDownloadFetchingMetadata')}</span>
      <span class="loading-note">{msg('modals.addDownloadFetchingNote')}</span>
    </div>
  {:else if info}
    <div class="files">
      <div class="torrent">
        <span class="torrent-name">{info.name}</span>
        <span class="torrent-size">{bytesSize(info.totalBytes)}</span>
      </div>
      <div class="dest">
        <span class="field-label">{msg('modals.addDownloadDestinationLabel')}</span>
        <div class="dest-controls">
          <input class="input sm" type="text" readonly value={destination} />
          <Button size="sm" onclick={browseDestination}>{msg('modals.addDownloadBrowse')}</Button>
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
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={!canContinue} onclick={() => proceed(source.trim())}>{msg('common.continue')}</Button>
    {:else if step === 'files'}
      <span class="summary">
        {msg('downloads.selected', { count: selectedCount, size: bytesSize(selectedSize) })}
      </span>
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={selectedCount === 0 || starting} onclick={start}>{msg('modals.addDownloadStart')}</Button>
    {/if}
  {/snippet}
</Modal>
{/if}

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
    font-size: var(--font-xs);
    color: var(--text-3);
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
    font-size: var(--font-xs);
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
    font-size: var(--font-md);
    font-weight: 500;
  }

  .loading-note {
    font-size: var(--font-xs);
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
    font-size: var(--font-lg);
    font-weight: 600;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .torrent-size {
    font-size: var(--font-sm);
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
    align-items: center;
    gap: 0.8rem;
  }

  .dest-controls .input {
    flex: 1;
  }

  .file-list {
    display: flex;
    flex-direction: column;
    max-height: 28rem;
    overflow-y: auto;
    padding: var(--space-1);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .file-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.7rem 0.8rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .file-row:hover {
    background: var(--hover);
  }

  .box {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.8rem;
    height: 1.8rem;
    flex-shrink: 0;
    border-radius: var(--radius-xs);
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
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-size {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }

  .summary {
    margin-right: auto;
    align-self: center;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
