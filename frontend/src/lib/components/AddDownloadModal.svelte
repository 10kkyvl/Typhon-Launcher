<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte';
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
  import PathField from './PathField.svelte';
  import Spinner from './Spinner.svelte';
  import TextField from './TextField.svelte';

  /**
   * Adds a download from a magnet link or a .torrent file: the files are fetched first (nothing is
   * sent while the link is typed or pasted), then the chosen files start downloading into the folder.
   * A refused Start is said at the folder when it is about the folder, else in the dialog, by code.
   * The dialog is reopened and unmounted with its page: a Start answered after that closes nothing,
   * and a refusal is said once in a toast naming the torrent (the downloads store says an added one).
   */
  // Codes of a Start refused because of the destination folder: said at the folder field.
  const FOLDER_CODES = new Set([
    'download.destination_permission',
    'download.destination_unavailable',
    'download.empty_destination',
    'download.not_enough_space',
    'download.no_free_space',
  ]);

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
  // Why the torrent file could not be picked, why the folder could not be chosen or was refused, and
  // why a Start was refused for another reason.
  let sourceFailure = $state('');
  let folderFailure = $state('');
  let startFailure = $state('');
  const startFailureId = `${uid}-start-failure`;
  let folderBox = $state<HTMLElement>();

  // One per opening: a Start, a picker or a fetch answered after the dialog closed or reopened is not
  // this opening's. A destroyed dialog hears nothing either.
  let opening = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

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
    sourceFailure = '';
    folderFailure = '';
    startFailure = '';
  }

  $effect(() => {
    const isOpen = open;
    const preset = initialSource;
    const ready = libraryReady;
    untrack(() => {
      opening += 1;
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
    const at = opening;
    sourceFailure = '';
    let path: string;
    try {
      path = await selectTorrentFile();
    } catch (err) {
      if (alive && at === opening) sourceFailure = installErrorText(err);
      return;
    }
    if (!path || !alive || at !== opening) return;
    source = path;
    await ask(path);
  }

  async function browseDestination() {
    const at = opening;
    try {
      const path = await selectFolder(msg('modals.addDownloadChooseFolder'));
      if (path && alive && at === opening) {
        destination = path;
        folderFailure = '';
      }
    } catch (err) {
      console.warn('add download: folder picker', err);
      if (alive && at === opening) folderFailure = msg('modals.addDownloadFolderDialogFailed');
    }
  }

  async function start() {
    if (!info || starting || selectedCount === 0) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const files = info.files;
    const name = info.name;
    const hash = info.infoHash;
    const indices = selected.map((on, i) => (on ? i : -1)).filter((i) => i >= 0);
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    restartNeeded = false;
    folderFailure = '';
    startFailure = '';
    starting = true;
    // The torrent is the Start's while it runs: a reset meanwhile (the dialog closed or reopened by its
    // page) must not discard what Go is starting.
    pendingHash = '';
    try {
      await startDownloadFrom(
        hash,
        destination,
        indices,
        consolePlatform
          ? { ...origin }
          : { ...(origin ?? {}), autoInstall, elevateAhead: autoInstall && canElevateAhead && elevateAhead },
      );
      if (!heard()) return;
      starting = false;
      open = false;
    } catch (err) {
      const code = errorCode(err);
      if (!heard()) {
        // Go still holds the fetched torrent unless it is the one it lost: nobody will press Start on it.
        if (code !== 'download.metadata_required') drop(discardMetadata(hash), 'discard a torrent left by a late refusal');
        toast(msg('modals.addDownloadRefusedFor', { title: name, reason: installErrorText(err) }), 'danger');
        return;
      }
      starting = false;
      // Go holds nothing for this torrent any more (its client went with a network change): one fetch
      // of the same source brings it back. Any other failure leaves it waiting for the next Start.
      if (open && code === 'download.metadata_required') {
        await fetchAgain(new Map(files.map((file, i) => [file.path, Boolean(selected[i])])));
        return;
      }
      pendingHash = hash;
      const text = installErrorText(err);
      if (FOLDER_CODES.has(code)) folderFailure = text;
      else startFailure = text;
      await hand(() => (folderFailure ? (folderBox?.querySelector<HTMLElement>('button') ?? null) : document.getElementById(startFailureId)));
    }
  }

  const continueReason = $derived(canContinue ? '' : msg('modals.addDownloadNeedsMagnet'));
  const heldReason = $derived(starting ? msg('modals.actionRunning') : '');
  const startReason = $derived(starting || selectedCount > 0 ? '' : msg('modals.addDownloadNoFiles'));

  // While a Start runs, the choices it sent stay what the files step shows.
  function choose(change: () => void) {
    if (!starting) change();
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
      <TextField label={msg('modals.addDownloadMagnetLabel')} placeholder="magnet:?xt=urn:btih:…" bind:value={source} error={sourceFailure} />

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
      <div class="dest" bind:this={folderBox}>
        <PathField
          label={msg('modals.addDownloadDestinationLabel')}
          value={destination}
          browseLabel={msg('modals.addDownloadBrowse')}
          onbrowse={browseDestination}
          error={folderFailure}
          disabledReason={heldReason || undefined}
        />
      </div>
      <div class="file-list">
        {#each info.files as file, i (i)}
          <Checkbox
            checked={selected[i]}
            onchange={(on) => choose(() => (selected[i] = on))}
            label={file.path}
            aside={bytesSize(file.size)}
            disabled={starting}
            reason={heldReason}
          />
        {/each}
      </div>
      <div class="options">
        <Checkbox
          checked={autoInstall}
          onchange={(on) => choose(() => (autoInstall = on))}
          label={msg('modals.addDownloadAutoInstallLabel')}
          description={msg('modals.addDownloadAutoInstallSub')}
          disabled={starting}
          reason={heldReason}
        />
        {#if canElevateAhead}
          <div class="nested">
            <Checkbox
              checked={elevateAhead}
              onchange={(on) => choose(() => (elevateAhead = on))}
              label={msg('modals.addDownloadElevateAheadLabel')}
              description={msg('modals.addDownloadElevateAheadSub')}
              disabled={starting}
              reason={heldReason}
            />
          </div>
        {/if}
      </div>
      {#if startFailure}
        <Notice tone="danger" compact id={startFailureId} tabindex={-1}>{startFailure}</Notice>
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    {#if step === 'source'}
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={!canContinue} reason={continueReason} onclick={() => ask(source.trim())}>{msg('common.continue')}</Button>
    {:else if step === 'files'}
      <span class="summary">
        {msg('downloads.selected', { count: selectedCount, size: bytesSize(selectedSize) })}
      </span>
      <Button disabled={starting} reason={heldReason} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button id="{uid}-start" variant="primary" disabled={!!startReason} reason={startReason} busy={starting} onclick={start}>{msg('modals.addDownloadStart')}</Button>
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
    overflow-wrap: anywhere;
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
