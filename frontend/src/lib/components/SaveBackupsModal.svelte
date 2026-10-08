<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import Modal from './Modal.svelte';
  import SavesCandidatesModal from './SavesCandidatesModal.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import { deleteSaveBackupPrompt, restoreSavesPrompt } from '../confirm/prompts';
  import { chooseSavesDir } from '../game/actions/saves';
  import { errorCode, msg } from '../i18n';
  import { saveBackupErrorText, snapshotKindLabel, snapshotWhen } from '../savebackup/messages';
  import { locateSaves, setSavesDir, type SavesResult } from '../services/library';
  import {
    createBackup,
    deleteBackup,
    listBackups,
    onBackupEvent,
    restoreBackup,
    type Snapshot,
  } from '../services/savebackup';
  import { openFolder } from '../services/settings';
  import { runningGames } from '../stores/library';
  import { closeSaveBackups, saveBackupTarget } from '../stores/savebackup';
  import { toast } from '../stores/toasts';
  import { errorMessage } from '../utils/errors';
  import { bytesSize } from '../utils/format';

  interface Pending {
    kind: 'restore' | 'delete';
    snapshot: Snapshot;
  }

  const target = $derived($saveBackupTarget);
  const gameRunning = $derived(target ? $runningGames.has(target.gameId) : false);

  let snapshots = $state<Snapshot[]>([]);
  let listLoading = $state(false);
  let listError = $state('');
  let saves = $state<SavesResult | null>(null);
  let savesLoading = $state(false);
  let savesError = $state('');
  let busy = $state(false);
  let creating = $state(false);
  let failure = $state('');
  let pending = $state<Pending | null>(null);
  let candidatesOpen = $state(false);

  let loadedFor = '';
  let listSeq = 0;
  let savesSeq = 0;

  $effect(() => {
    const current = $saveBackupTarget;
    untrack(() => {
      if (!current) {
        loadedFor = '';
        listSeq++;
        savesSeq++;
        return;
      }
      if (loadedFor === current.gameId) return;
      loadedFor = current.gameId;
      snapshots = [];
      listError = '';
      saves = null;
      savesError = '';
      failure = '';
      pending = null;
      candidatesOpen = false;
      void loadList(current.gameId);
      void loadSaves(current.gameId);
    });
  });

  $effect(() => {
    const current = $saveBackupTarget;
    if (!current) return;
    return onBackupEvent((event) => {
      if (event.gameId === current.gameId) void loadList(current.gameId);
    });
  });

  async function loadList(gameId: string) {
    const seq = ++listSeq;
    listLoading = true;
    try {
      const list = await listBackups(gameId);
      if (seq !== listSeq) return;
      snapshots = list;
      listError = '';
    } catch (err) {
      if (seq !== listSeq) return;
      listError = saveBackupErrorText(err, msg('saves.listFailed'));
    } finally {
      if (seq === listSeq) listLoading = false;
    }
  }

  async function loadSaves(gameId: string) {
    const seq = ++savesSeq;
    savesLoading = true;
    try {
      const found = await locateSaves(gameId);
      if (seq !== savesSeq) return;
      saves = found;
      savesError = '';
    } catch (err) {
      if (seq !== savesSeq) return;
      saves = null;
      savesError = saveBackupErrorText(err, errorMessage(err));
    } finally {
      if (seq === savesSeq) savesLoading = false;
    }
  }

  async function perform(action: () => Promise<void>, fallback: (err: unknown) => string) {
    if (busy) return;
    busy = true;
    failure = '';
    try {
      await action();
    } catch (err) {
      failure = saveBackupErrorText(err, fallback(err));
    } finally {
      busy = false;
      creating = false;
    }
  }

  const backupFailure = () => msg('savebackup.fallback');

  function create() {
    if (!target || busy) return;
    const { gameId } = target;
    creating = true;
    return perform(async () => {
      try {
        await createBackup(gameId);
        toast(msg('saves.created'), 'success');
      } catch (err) {
        if (errorCode(err) !== 'savebackup.rotation_failed') throw err;
        toast(msg('savebackup.rotation_failed'));
      } finally {
        await loadList(gameId);
      }
    }, backupFailure);
  }

  async function restore(snapshot: Snapshot) {
    if (!target) return;
    const { gameId } = target;
    await perform(async () => {
      try {
        await restoreBackup(gameId, snapshot.id);
        toast(msg('saves.restored'), 'success');
      } catch (err) {
        const code = errorCode(err);
        if (code === 'savebackup.cleanup_failed') {
          toast(msg('saves.restoredWithWarning', { reason: msg('savebackup.cleanup_failed') }));
        } else if (code === 'savebackup.rotation_failed') {
          toast(msg('saves.restoredRotationFailed'));
        } else {
          throw err;
        }
      } finally {
        await loadList(gameId);
      }
    }, backupFailure);
  }

  async function remove(snapshot: Snapshot) {
    if (!target) return;
    const { gameId } = target;
    await perform(async () => {
      try {
        await deleteBackup(gameId, snapshot.id);
        toast(msg('saves.deleted'), 'success');
      } finally {
        await loadList(gameId);
      }
    }, backupFailure);
  }

  async function runPending() {
    const current = pending;
    if (!current) return;
    if (current.kind === 'restore') await restore(current.snapshot);
    else await remove(current.snapshot);
  }

  function openSaves() {
    const path = saves?.path;
    if (!path) return;
    return perform(() => openFolder(path), errorMessage);
  }

  function changeFolder() {
    if (!target) return;
    const { gameId, title } = target;
    return perform(async () => {
      const dir = await chooseSavesDir(gameId, title);
      if (dir) await loadSaves(gameId);
    }, errorMessage);
  }

  function useCandidate(dir: string) {
    if (!target) return;
    const { gameId } = target;
    candidatesOpen = false;
    return perform(async () => {
      await setSavesDir(gameId, dir);
      await loadSaves(gameId);
    }, errorMessage);
  }

  function browseFromCandidates() {
    candidatesOpen = false;
    void changeFolder();
  }
</script>

<Modal
  open={!!target}
  title={target ? msg('saves.title', { title: target.title }) : msg('games.actionSavesBackups')}
  width="64rem"
  onclose={closeSaveBackups}
>
  {#if target}
    <div class="saves">
      <div class="field">
        <span class="field-label">{msg('saves.folderLabel')}</span>
        {#if savesLoading && !saves && !savesError}
          <p class="hint">{msg('saves.folderLoading')}</p>
        {:else if saves?.path}
          <p class="path">{saves.path}</p>
          <div class="actions">
            <Button size="sm" disabled={busy} onclick={openSaves}>{msg('saves.open')}</Button>
            <Button size="sm" disabled={busy} onclick={changeFolder}>{msg('saves.change')}</Button>
          </div>
        {:else if savesError}
          <p class="failure">{savesError}</p>
          <div class="actions">
            <Button size="sm" disabled={busy} onclick={changeFolder}>{msg('saves.pickFolder')}</Button>
          </div>
        {:else if saves?.candidates && saves.candidates.length > 0}
          <p class="hint">{msg('ui.savesMultipleCandidates', { title: target.title })}</p>
          <div class="actions">
            <Button size="sm" disabled={busy} onclick={() => (candidatesOpen = true)}>
              {msg('saves.pickFolder')}
            </Button>
          </div>
        {:else}
          <p class="hint">{saves && saves.unreadable > 0 ? msg('ui.savesPartialUnreadable') : msg('ui.savesNotFound')}</p>
          <div class="actions">
            <Button size="sm" disabled={busy} onclick={changeFolder}>{msg('saves.pickFolder')}</Button>
          </div>
        {/if}
      </div>

      <div class="list">
        <div class="list-head">
          <span class="field-label">{msg('saves.listTitle')}</span>
          <Button variant="primary" size="sm" disabled={busy || !saves?.path} onclick={create}>
            {creating ? msg('saves.creating') : msg('saves.createBackup')}
          </Button>
        </div>

        {#if failure}
          <p class="failure">{failure}</p>
        {/if}
        {#if listError}
          <p class="failure">{listError}</p>
        {/if}
        {#if gameRunning}
          <p class="warning">{msg('saves.gameRunning')}</p>
        {/if}

        {#if snapshots.length > 0}
          <ul class="snapshots">
            {#each snapshots as snapshot (snapshot.id)}
              <li class="snapshot">
                <div class="snapshot-main">
                  <span class="snapshot-when">{snapshotWhen(snapshot)}</span>
                  {#if snapshot.broken}
                    <div class="snapshot-broken">
                      <StatusBadge kind="danger" label={msg('saves.broken')} />
                      {#if snapshot.problem}
                        <span class="snapshot-problem">{snapshot.problem}</span>
                      {/if}
                    </div>
                  {:else}
                    <span class="snapshot-meta">
                      {snapshotKindLabel(snapshot.kind)} · {bytesSize(snapshot.sizeBytes)} · {msg('saves.filesCount', { count: snapshot.files })}
                    </span>
                  {/if}
                </div>
                <div class="actions">
                  {#if !snapshot.broken}
                    <Button size="sm" disabled={busy || gameRunning} onclick={() => (pending = { kind: 'restore', snapshot })}>
                      {msg('saves.restore')}
                    </Button>
                  {/if}
                  <Button size="sm" variant="danger" disabled={busy} onclick={() => (pending = { kind: 'delete', snapshot })}>
                    {msg('saves.delete')}
                  </Button>
                </div>
              </li>
            {/each}
          </ul>
        {:else if listLoading}
          <p class="hint">{msg('common.loading')}</p>
        {:else if !listError}
          <div class="empty">
            <span class="empty-title">{msg('saves.empty')}</span>
            <span class="hint">{msg('saves.emptyHint')}</span>
          </div>
        {/if}
      </div>
    </div>
  {/if}

  {#snippet footer()}
    <Button onclick={closeSaveBackups}>{msg('common.close')}</Button>
  {/snippet}
</Modal>

{#if target && saves?.candidates && saves.candidates.length > 0}
  <SavesCandidatesModal
    bind:open={candidatesOpen}
    title={target.title}
    candidates={saves.candidates}
    onpick={useCandidate}
    onbrowse={browseFromCandidates}
  />
{/if}

{#if pending}
  <ConfirmModal
    prompt={pending.kind === 'restore' ? restoreSavesPrompt() : deleteSaveBackupPrompt(snapshotWhen(pending.snapshot))}
    onconfirm={runPending}
    onclose={() => (pending = null)}
  />
{/if}

<style>
  .saves {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .field,
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .path {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .hint {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .failure {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .warning {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--warning);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .list-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .snapshots {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .snapshot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .snapshot-main {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-width: 0;
  }

  .snapshot-when {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .snapshot-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .snapshot-broken {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .snapshot-problem {
    font-size: var(--font-xs);
    color: var(--danger);
    word-break: break-word;
  }

  .empty {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: var(--space-5) 0;
    text-align: center;
  }

  .empty-title {
    font-size: var(--font-md);
    color: var(--text);
  }
</style>
