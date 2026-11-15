<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { FileUp } from '@lucide/svelte';
  import {
    cancelFetchMetadata,
    discardMetadata,
    fetchMetadata,
    selectTorrentFile,
    startDownloadFrom,
    type DownloadOrigin,
    type TorrentInfo,
  } from '../services/downloads';
  import { offerElevateAhead } from '../services/install';
  import { selectFolder } from '../services/settings';
  import { appInfo, elevationSupported, type AppInfo } from '../services/system';
  import { installErrorText } from '../install/installErrors';
  import { settings } from '../stores/settings';
  import { toast } from '../stores/toasts';
  import { bytesSize } from '../utils/format';
  import { focusHandOff } from '../utils/focusHandOff';
  import { focusQuietly } from '../layers/focus';
  import { msg } from '../i18n';
  import { errorCode } from '../i18n/errors';
  import { platformBadge } from '../library/emulation';
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import ConsoleTag from './ConsoleTag.svelte';
  import EmulatorHint from './EmulatorHint.svelte';
  import LibrarySetupModal from './LibrarySetupModal.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';

  let {
    open = $bindable(false),
    initialSource = '',
    origin = undefined,
  }: { open?: boolean; initialSource?: string; origin?: DownloadOrigin } = $props();
  const uid = $props.id();

  let step = $state<'source' | 'loading' | 'files'>('source');
  let source = $state('');
  let info = $state<TorrentInfo | null>(null);
  let destination = $state('');
  let selected = $state<boolean[]>([]);
  let starting = $state(false);
  let autoInstall = $state(false);
  let elevateAhead = $state(false);
  let platform = $state<AppInfo | null>(null);
  let pendingHash = '';
  let token = 0;
  // The source of the fetch in flight: a Retry fetches a remembered source whatever the field holds.
  let fetching = '';
  // What the files step was fetched from: a Start that finds nothing waiting in Go fetches it again.
  let fetchedFrom = '';
  // The file choices a fetch after such a Start keeps, by path; null for a fetch the player asked for.
  let kept: Map<string, boolean> | null = null;
  // After that fetch Start has to be pressed again.
  let restartNeeded = $state(false);
  // A failed fetch, said on the source step with a Retry of the same source: why it failed, whether
  // it was the fetch after a Start (which keeps `kept` for the Retry), and what it fetched.
  let fetchError = $state('');
  let refetchFailed = $state(false);
  let failedFrom = '';
  let retrying = $state(false);

  const consolePlatform = $derived(origin?.platform ?? '');
  const libraryReady = $derived(Boolean($settings?.libraryPath));
  const canContinue = $derived(source.trim().startsWith('magnet:'));
  const selectedCount = $derived(selected.filter(Boolean).length);
  const selectedSize = $derived(
    (info?.files ?? []).reduce((sum, file, i) => (selected[i] ? sum + file.size : sum), 0),
  );
  const selectedPaths = $derived((info?.files ?? []).filter((_, i) => selected[i]).map((file) => file.path));
  const canElevateAhead = $derived(
    offerElevateAhead({
      elevationSupported: platform !== null && elevationSupported(platform),
      autoInstall,
      paths: selectedPaths,
    }),
  );

  // Asks Go to drop a fetch or a fetched torrent the player has left. A refusal is logged, not shown:
  // the dialog is closing or has moved on, and nothing in it can act on it.
  function drop(call: Promise<unknown>, what: string) {
    call.catch((err: unknown) => console.error(`add download: ${what}`, err));
  }

  function reset() {
    token++;
    if (step === 'loading') {
      drop(cancelFetchMetadata(fetching), 'cancel the metadata fetch');
    }
    if (pendingHash) {
      drop(discardMetadata(pendingHash), 'discard the fetched torrent');
      pendingHash = '';
    }
    step = 'source';
    source = '';
    info = null;
    selected = [];
    starting = false;
    fetchedFrom = '';
    kept = null;
    restartNeeded = false;
    fetchError = '';
    refetchFailed = false;
    failedFrom = '';
    retrying = false;
  }

  $effect(() => {
    const isOpen = open;
    const preset = initialSource;
    const ready = libraryReady;
    untrack(() => {
      reset();
      if (isOpen && ready) {
        const current = get(settings);
        destination = current?.downloadsPath ?? '';
        autoInstall = current?.autoInstall ?? false;
        elevateAhead = current?.elevateAhead ?? false;
        loadPlatform();
        if (preset) {
          source = preset;
          ask(preset);
        }
      }
    });
  });

  // A failed lookup hides the pre-elevation checkbox instead of guessing:
  // without it the install waits for the user, which is the safe side.
  async function loadPlatform() {
    try {
      platform = await appInfo();
    } catch (err) {
      platform = null;
      toast(installErrorText(err), 'danger');
    }
  }

  // A fetch after a Start that found nothing waiting (`kept` set) keeps the files the player chose.
  // A failure, of any fetch, is said in the dialog with a Retry; the failure stays on screen while the
  // Retry runs. It answers whether the files step came, or undefined when the dialog moved on meanwhile.
  async function proceed(value: string): Promise<boolean | undefined> {
    const current = ++token;
    const choices = kept;
    fetching = value;
    step = 'loading';
    try {
      const result = await fetchMetadata(value);
      if (!open || current !== token) {
        drop(discardMetadata(result.infoHash), 'discard a torrent fetched for nothing');
        return undefined;
      }
      pendingHash = result.infoHash;
      fetchedFrom = value;
      info = result;
      selected = result.files.map((file) => choices?.get(file.path) ?? true);
      kept = null;
      fetchError = '';
      refetchFailed = false;
      restartNeeded = choices !== null;
      step = 'files';
      return true;
    } catch (err) {
      if (!open || current !== token) return undefined;
      fetchError = installErrorText(err);
      refetchFailed = choices !== null;
      failedFrom = value;
      step = 'source';
      return false;
    }
  }

  // A fetch the player asked for starts anew.
  function ask(value: string) {
    kept = null;
    fetchError = '';
    refetchFailed = false;
    restartNeeded = false;
    return proceed(value);
  }

  const retryFetch = () => proceed(failedFrom);

  // While the fetch runs the files step and its Start are gone: focus waits on the dialog itself,
  // and comes to Start when the files are back, unless the player has moved it meanwhile.
  async function fetchAgain(choices: Map<string, boolean>) {
    const root = document.getElementById(`${uid}-start`)?.closest('[role="dialog"]')?.closest<HTMLElement>('[tabindex]') ?? null;
    const handOff = focusHandOff();
    kept = choices;
    const fetched = proceed(fetchedFrom);
    await handOff(() => root);
    if (!(await fetched)) return;
    await tick();
    const active = document.activeElement;
    if (active && active !== document.body && active !== root && active.isConnected) return;
    const start = document.getElementById(`${uid}-start`);
    if (start) focusQuietly(start);
  }

  async function pickTorrent() {
    try {
      const path = await selectTorrentFile();
      if (!path) return;
      source = path;
      await ask(path);
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
    const files = info.files;
    const indices = selected.map((on, i) => (on ? i : -1)).filter((i) => i >= 0);
    restartNeeded = false;
    starting = true;
    try {
      await startDownloadFrom(
        info.infoHash,
        destination,
        indices,
        consolePlatform
          ? { ...origin }
          : { ...(origin ?? {}), autoInstall, elevateAhead: autoInstall && canElevateAhead && elevateAhead },
      );
      pendingHash = '';
      open = false;
    } catch (err) {
      starting = false;
      // Go holds nothing for this torrent any more (its client went with a network change): one fetch
      // of the same source brings it back. Any other failure leaves it waiting for the next Start.
      if (open && errorCode(err) === 'download.metadata_required') {
        pendingHash = '';
        await fetchAgain(new Map(files.map((file, i) => [file.path, Boolean(selected[i])])));
        return;
      }
      toast(installErrorText(err), 'danger');
    }
    starting = false;
  }
</script>

{#if !libraryReady}
  <LibrarySetupModal bind:open />
{:else}
<Modal
  dismissible={!starting}
  bind:open
  title={consolePlatform && step === 'files'
    ? msg('emul.downloadTitle', { console: platformBadge(consolePlatform) })
    : msg('modals.addDownloadTitle')}
  width={step === 'files' && !consolePlatform ? '62rem' : '48rem'}
>
  {#if fetchError || retrying}
    <div class="said">
      <Notice
        tone="danger"
        compact
        title={refetchFailed ? msg('modals.addDownloadRefetchFailed') : msg('modals.addDownloadFetchFailed')}
        bind:retrying
        retry={{ label: msg('common.retry'), run: retryFetch, focus: () => document.getElementById(`${uid}-start`) }}
      >
        {fetchError}
      </Notice>
    </div>
  {/if}
  {#if restartNeeded && step === 'files'}
    <div class="said">
      <Notice tone="info" compact>{msg('modals.addDownloadRestartNeeded')}</Notice>
    </div>
  {/if}
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
    <div class="loading" role="status">
      <Spinner size="lg" />
      <span class="loading-text">{msg('modals.addDownloadFetchingMetadata')}</span>
      <span class="loading-note">{msg('modals.addDownloadFetchingNote')}</span>
    </div>
  {:else if info && consolePlatform}
    <div class="files">
      <div class="torrent">
        <span class="torrent-name">{info.name}</span>
        <span class="torrent-size">{bytesSize(info.totalBytes)}</span>
      </div>
      <div class="console">
        <ConsoleTag platform={consolePlatform} />
        {#if origin?.title}<span class="console-title">{origin.title}</span>{/if}
      </div>
      <p class="note">{msg('emul.downloadNote')}</p>
      <p class="note">{msg('emul.downloadAuto')}</p>
      <EmulatorHint platform={consolePlatform} />
    </div>
  {:else if info}
    <div class="files">
      <div class="torrent">
        <span class="torrent-name">{info.name}</span>
        <span class="torrent-size">{bytesSize(info.totalBytes)}</span>
      </div>
      <div class="dest">
        <span class="field-label" id="{uid}-destination-label">{msg('modals.addDownloadDestinationLabel')}</span>
        <div class="dest-controls">
          <input class="input sm" type="text" readonly aria-labelledby="{uid}-destination-label" value={destination} />
          <Button size="sm" onclick={browseDestination}>{msg('modals.addDownloadBrowse')}</Button>
        </div>
      </div>
      <div class="file-list">
        {#each info.files as file, i (i)}
          <Checkbox bind:checked={selected[i]} label={file.path} aside={bytesSize(file.size)} />
        {/each}
      </div>
      <div class="options">
        <Checkbox
          bind:checked={autoInstall}
          label={msg('modals.addDownloadAutoInstallLabel')}
          description={msg('modals.addDownloadAutoInstallSub')}
        />
        {#if canElevateAhead}
          <div class="nested">
            <Checkbox
              bind:checked={elevateAhead}
              label={msg('modals.addDownloadElevateAheadLabel')}
              description={msg('modals.addDownloadElevateAheadSub')}
            />
          </div>
        {/if}
      </div>
    </div>
  {/if}

  {#snippet footer()}
    {#if step === 'source'}
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={!canContinue} onclick={() => ask(source.trim())}>{msg('common.continue')}</Button>
    {:else if step === 'files'}
      <span class="summary">
        {msg('downloads.selected', { count: selectedCount, size: bytesSize(selectedSize) })}
      </span>
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button id="{uid}-start" variant="primary" disabled={selectedCount === 0} busy={starting} onclick={start}>{msg('modals.addDownloadStart')}</Button>
    {/if}
  {/snippet}
</Modal>
{/if}

<style>
  .said {
    margin-bottom: var(--space-4);
  }

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

  .console {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .console-title {
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .note {
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
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

  .options {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .nested {
    margin-left: var(--space-5);
  }

  .summary {
    margin-right: auto;
    align-self: center;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }
</style>
