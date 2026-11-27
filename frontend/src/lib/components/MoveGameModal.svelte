<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { HardDrive, ShieldCheck } from '@lucide/svelte';
  import { REASONS, moveErrorText } from '../relocate/moveMessages';
  import { movePercent, moveSummary } from '../relocate/moveText';
  import { inWails } from '../services/backend';
  import { cancelMove, moveGame, moveGameTarget, selectMoveTargetFolder, type MoveJob } from '../services/relocate';
  import { getStorageInfoFor, type StorageInfo } from '../services/system';
  import { libraryGames } from '../stores/library';
  import { closeMoveGame, moves, moveTarget } from '../stores/relocate';
  import { navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { bytesSize } from '../utils/format';
  import { focusHandOff } from '../utils/focusHandOff';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import PathField from './PathField.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import { errorCode, msg } from '../i18n';

  /**
   * Moves one game's folder into another folder (relocate.Service.MoveGame). The chosen folder is
   * checked by Go first (MoveGameTarget: the folder the game would land in, or why it cannot); a refusal
   * of the folder or of the start is said at the field, by code. A running move goes on in the
   * background when the dialog closes; its Cancel says what it leaves (relocate.Service.Cancel stops the
   * copy and its check, the game stays where it was; once the files are in the new place the move
   * finishes). An answer for an earlier opening, or a dialog that is gone, paints nothing and closes
   * nothing: a start is then said once in a toast naming its game.
   */
  const uid = $props.id();
  const keepId = `${uid}-keep`;

  const game = $derived($moveTarget ? ($libraryGames.find((g) => g.id === $moveTarget?.gameId) ?? null) : null);

  let jobId = $state<string | null>(null);
  let startedJob = $state<MoveJob | null>(null);
  const job = $derived(jobId ? ($moves.find((j) => j.id === jobId) ?? startedJob) : null);
  const jobFailed = $derived(job?.stage === 'failed' ? job : null);
  const jobActive = $derived(job && job.stage !== 'failed' && job.stage !== 'cancelled' && job.stage !== 'done');

  let picked = $state('');
  let diskInfo = $state<StorageInfo | null>(null);
  let diskUnknown = $state(false);
  let finalPath = $state('');
  let resolving = $state(false);
  let starting = $state(false);
  let cancelling = $state(false);
  // Why the folder or the start was refused, said at the field; why a cancel was refused, at the progress.
  let failure = $state('');
  let cancelFailure = $state('');
  let fieldBox = $state<HTMLElement>();

  // One per opening: the dialog stays mounted in the shell and is opened for one game after another.
  let opening = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  // Before the dialog draws: an opening that finds the game's move running draws its progress first,
  // and the layer gives focus to "continue in background", not to a form that is about to go.
  $effect.pre(() => {
    const target = $moveTarget;
    untrack(() => {
      opening += 1;
      picked = '';
      finalPath = '';
      diskInfo = null;
      diskUnknown = false;
      resolving = false;
      starting = false;
      cancelling = false;
      failure = '';
      cancelFailure = '';
      jobId = null;
      startedJob = null;
      if (!target) return;
      const existing = $moves.find(
        (j) => j.scope === 'game' && j.gameId === target.gameId && j.stage !== 'done' && j.stage !== 'cancelled' && j.stage !== 'failed',
      );
      if (existing) jobId = existing.id;
    });
  });

  $effect(() => {
    if (job?.stage === 'done') closeMoveGame();
  });

  const browseButton = () => fieldBox?.querySelector<HTMLElement>('button') ?? null;

  function named(what: 'start' | 'cancel', title: string, err: unknown): string {
    const key = REASONS[errorCode(err)];
    if (what === 'start') {
      return key ? msg('modals.moveGameStartFailedForReason', { title, reason: msg(key) }) : msg('modals.moveGameStartFailedFor', { title });
    }
    return key ? msg('modals.moveGameCancelFailedForReason', { title, reason: msg(key) }) : msg('modals.moveGameCancelFailedFor', { title });
  }

  async function pickFolder() {
    if (!inWails) {
      toast(msg('modals.moveGameDesktopOnly'));
      return;
    }
    const at = opening;
    const heard = () => alive && at === opening;
    const id = game?.id;
    if (!id) return;
    let folder: string;
    try {
      folder = await selectMoveTargetFolder();
    } catch (err) {
      if (heard()) failure = moveErrorText(err, msg('modals.moveGameFolderDialogFailed'));
      return;
    }
    if (!heard() || !folder) return;
    picked = folder;
    finalPath = '';
    failure = '';
    diskInfo = null;
    diskUnknown = false;
    resolving = true;
    const current = () => heard() && picked === folder;
    await Promise.all([resolveTarget(id, folder, current), readDisk(folder, current)]);
  }

  async function resolveTarget(id: string, folder: string, current: () => boolean) {
    try {
      const path = await moveGameTarget(id, folder);
      if (current()) finalPath = path;
    } catch (err) {
      if (current()) failure = moveErrorText(err, msg('modals.moveGameStartFailed'));
    } finally {
      if (current()) resolving = false;
    }
  }

  async function readDisk(folder: string, current: () => boolean) {
    try {
      const info = await getStorageInfoFor(folder);
      if (current()) diskInfo = info;
    } catch (err) {
      // The free space is a fact of the folder here; Go measures it again when the move starts and
      // refuses with relocate.free_space_unknown if it still cannot.
      console.warn('move dialog: free space', err);
      if (current()) diskUnknown = true;
    }
  }

  async function start() {
    if (!game || !picked || !finalPath || starting || resolving) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const id = game.id;
    const title = game.title;
    const parent = picked;
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    starting = true;
    failure = '';
    try {
      const created = await moveGame(id, parent);
      if (!heard()) {
        toast(msg('modals.moveGameStartedFor', { title }));
        return;
      }
      starting = false;
      jobId = created.id;
      startedJob = created;
      await hand(() => document.getElementById(keepId));
    } catch (err) {
      if (!heard()) {
        toast(named('start', title, err), 'danger');
        return;
      }
      starting = false;
      failure = moveErrorText(err, msg('modals.moveGameStartFailed'));
      await hand(browseButton);
    }
  }

  async function cancel() {
    if (!job || cancelling) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const id = job.id;
    const title = job.title || game?.title || '';
    cancelling = true;
    cancelFailure = '';
    try {
      await cancelMove(id);
    } catch (err) {
      if (heard()) cancelFailure = moveErrorText(err, msg('modals.moveGameCancelFailed'));
      else toast(named('cancel', title, err), 'danger');
    } finally {
      if (heard()) cancelling = false;
    }
  }

  function retry() {
    const hand = focusHandOff();
    jobId = null;
    startedJob = null;
    failure = '';
    void hand(browseButton);
  }

  function close() {
    closeMoveGame();
  }

  function contactSupport() {
    closeMoveGame();
    navigate('support', { view: 'new', topic: 'other' });
  }

  const startReason = $derived.by(() => {
    if (starting) return '';
    if (!picked) return msg('transfers.moveErrNoTarget');
    if (resolving) return msg('modals.moveGameChecking');
    if (!finalPath) return failure || msg('modals.moveGameStartFailed');
    return '';
  });
</script>

{#snippet folderFacts()}
  {#if resolving}
    <span class="fact">{msg('modals.moveGameChecking')}</span>
  {/if}
  {#if diskInfo}
    <span class="disk">
      <HardDrive size="1.5rem" strokeWidth={1.8} aria-hidden="true" />
      <span class="disk-name">{msg('modals.moveGameDiskName', { volume: diskInfo.volume || '—' })}{diskInfo.filesystem ? ` · ${diskInfo.filesystem}` : ''}</span>
      <span class="disk-meta">{msg('modals.moveGameDiskFree', { free: bytesSize(diskInfo.freeBytes), total: bytesSize(diskInfo.totalBytes) })}</span>
    </span>
  {:else if diskUnknown}
    <span class="fact">{msg('modals.moveGameFreeSpaceUnknown')}</span>
  {/if}
{/snippet}

<!-- dismissible while resolving: it only reads where the game would go; closing drops the answer. -->
<Modal dismissible={!(starting || cancelling)} open={!!$moveTarget} title={msg('modals.moveGameTitle')} width="52rem" onclose={close}>
  {#if game}
    <div class="move">
      <div class="field">
        <span class="field-label">{msg('modals.moveGameCurrentlyIn')}</span>
        <p class="path">{game.installDir}</p>
        {#if game.sizeBytes > 0}
          <span class="field-sub">{msg('modals.moveGameSize', { size: bytesSize(game.sizeBytes) })}</span>
        {/if}
      </div>

      {#if jobActive && job}
        <div class="progress">
          <ProgressBar value={movePercent(job)} label={game.title} />
          <div class="progress-text">
            <span>{moveSummary(job)}</span>
            <span>{movePercent(job)}%</span>
          </div>
          {#if job.currentFile}
            <span class="field-sub mono">{job.currentFile}</span>
          {/if}
          <p class="hint">{msg('modals.moveGameCancelNote')}</p>
          {#if cancelFailure}
            <Notice tone="danger" compact>{cancelFailure}</Notice>
          {/if}
        </div>
      {:else}
        {#if jobFailed}
          <Notice tone="danger" compact>{moveErrorText(jobFailed.error, msg('modals.moveGameFailed'))}</Notice>
          {#if errorCode(jobFailed.error) === 'relocate.ambiguous_recovery'}
            <div class="support">
              <Button size="sm" onclick={contactSupport}>{msg('tickets.writeSupport')}</Button>
            </div>
          {/if}
        {/if}

        <div class="field" bind:this={fieldBox}>
          <PathField
            label={msg('modals.moveGameNewFolder')}
            value={picked}
            placeholder={msg('modals.moveGameFolderPlaceholder')}
            browseLabel={msg('modals.moveGameBrowse')}
            onbrowse={pickFolder}
            error={failure}
            disabledReason={starting ? msg('modals.actionRunning') : undefined}
            facts={picked ? folderFacts : undefined}
          />
        </div>

        {#if finalPath}
          <p class="final">{msg('modals.moveGameWillMoveTo', { path: finalPath })}</p>
        {/if}

        <Notice compact announce={false}>
          {#snippet icon()}<ShieldCheck size="1.6rem" strokeWidth={1.8} />{/snippet}
          {msg('modals.moveGameSafetyText')}
        </Notice>

        <p class="hint">{msg('modals.moveGameHint')}</p>
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    {#if jobActive && job}
      <Button id={keepId} data-autofocus onclick={close}>{msg('modals.moveGameInBackground')}</Button>
      <Button variant="danger" busy={cancelling} onclick={cancel}>
        {cancelling ? msg('modals.moveGameCancelling') : msg('modals.moveGameCancelMove')}
      </Button>
    {:else}
      <Button disabled={starting} reason={starting ? msg('modals.actionRunning') : ''} onclick={close}>{msg('common.cancel')}</Button>
      {#if jobFailed}
        <Button variant="primary" onclick={retry}>{msg('modals.moveGameRetry')}</Button>
      {:else}
        <Button variant="primary" disabled={!!startReason} reason={startReason} busy={starting} onclick={start}>
          {starting ? msg('modals.moveGameStarting') : msg('modals.moveGameStart')}
        </Button>
      {/if}
    {/if}
  {/snippet}
</Modal>

<style>
  .move {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .field-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .mono {
    font-family: var(--font-mono);
    font-variant-ligatures: none;
    word-break: break-all;
  }

  .path {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .disk {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-2);
  }

  .fact {
    display: block;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .disk-name {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .disk-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .final {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .progress {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .progress-text {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .support {
    margin-top: var(--space-2);
  }
</style>
