<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import Notice from './Notice.svelte';
  import SavesCandidatesModal from './SavesCandidatesModal.svelte';
  import Spinner from './Spinner.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import TextLink from './TextLink.svelte';
  import { deleteCloudSavesPrompt, pullCloudSavesPrompt, deleteSaveBackupPrompt, restoreSavesPrompt } from '../confirm/prompts';
  import { chooseSavesDir } from '../game/actions/saves';
  import { cloudCodeText, cloudErrorText, cloudStateKind, cloudStateLabel } from '../cloudsaves/messages';
  import { errorCode, msg } from '../i18n';
  import { saveBackupErrorText, snapshotKindLabel, snapshotPlayedAfter, snapshotWhen } from '../savebackup/messages';
  import { cloudDeleteRemote, cloudPullRemote, cloudSyncNow } from '../services/cloudsaves';
  import { saveAttribution } from '../game/extras';
  import { metadataErrorText } from '../metadata/metadataErrors';
  import { openReferenceLink } from '../services/metadata';
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
  import { cloudStatus, openCloudConflict, refreshCloudStatus } from '../stores/cloudsaves';
  import { libraryGames, runningGames } from '../stores/library';
  import { navigate } from '../stores/router';
  import { settings } from '../stores/settings';
  import { closeSaveBackups, saveBackupsEnabled } from '../stores/savebackup';
  import { toast } from '../stores/toasts';
  import { authState } from '../stores/user';
  import { focusHandOff } from '../utils/focusHandOff';
  import { bytesSize, dateTime } from '../utils/format';

  interface Pending {
    kind: 'restore' | 'delete';
    snapshot: Snapshot;
    /** The backup whose Delete takes focus when this one leaves the list. */
    next?: string;
  }

  /**
   * The saves of one library game: where they are, their cloud copy and its actions, and the local
   * backups with create, restore and delete. Mounted by the "Saves…" dialog and the game page's Saves
   * tab. Without backups (`backups`, else whether they are on) it is the cloud part alone and asks
   * nothing about the folder or the backups. `busy` is true while a folder or backup action runs: the
   * dialog is not dismissed meanwhile.
   */
  let {
    gameId,
    backups,
    busy = $bindable(false),
  }: { gameId: string; backups?: boolean; busy?: boolean } = $props();

  const withBackups = $derived(backups ?? $saveBackupsEnabled);

  const uid = $props.id();
  const changeId = `${uid}-change`;
  const createId = `${uid}-create`;
  let list = $state<HTMLElement>();

  const game = $derived($libraryGames.find((g) => g.id === gameId));
  // What an action was started for: the game as it was at the press.
  const target = $derived({ gameId, title: game?.title ?? '' });
  const gameRunning = $derived($runningGames.has(gameId));
  const canonicalId = $derived(game?.canonicalGameId ?? '');
  const cloudEnabled = $derived($settings?.cloudSaves ?? false);
  const cloudSignedIn = $derived($authState === 'authenticated');
  const cloudGame = $derived($cloudStatus?.games.find((g) => g.gameId === gameId));
  const cloudReason = $derived(cloudGame?.code ? cloudCodeText(cloudGame.code, '') : '');
  const cloudSyncedAt = $derived.by(() => {
    const value = cloudGame?.syncedAt ? new Date(cloudGame.syncedAt) : null;
    return value && !Number.isNaN(value.getTime()) && value.getFullYear() > 1 ? dateTime(value) : '';
  });

  let snapshots = $state<Snapshot[]>([]);
  let listLoading = $state(false);
  let listError = $state('');
  let saves = $state<SavesResult | null>(null);
  let savesLoading = $state(false);
  let savesError = $state('');
  // Which folder button set `busy`: that one stays focused and busy, the others wait.
  let acting = $state<'' | 'open' | 'folder'>('');
  let creating = $state(false);
  let failure = $state('');
  let pending = $state<Pending | null>(null);
  let candidatesOpen = $state(false);
  let cloudBusy = $state(false);
  let cloudFailure = $state('');
  let cloudDeleteAsked = $state(false);
  let cloudPullAsked = $state(false);

  // The game shown, and its turn: an answer to an action or a load of an earlier game is dropped, so a
  // view whose id changes without a remount starts the new game from scratch.
  let loadedFor = '';
  let backupsFor = '';
  let turn = 0;
  let listSeq = 0;
  let savesSeq = 0;

  $effect(() => {
    const id = gameId;
    untrack(() => {
      if (loadedFor === id) return;
      loadedFor = id;
      turn++;
      busy = false;
      acting = '';
      creating = false;
      cloudBusy = false;
      snapshots = [];
      listError = '';
      saves = null;
      savesError = '';
      failure = '';
      pending = null;
      candidatesOpen = false;
      cloudFailure = '';
      cloudDeleteAsked = false;
      cloudPullAsked = false;
      backupsFor = '';
      void refreshCloudStatus();
    });
  });

  // Backups that turn out to be on after the view came (the switch loads at startup) are loaded then.
  $effect(() => {
    const id = gameId;
    if (!withBackups) return;
    untrack(() => {
      if (backupsFor === id) return;
      backupsFor = id;
      void loadList(id);
      void loadSaves(id);
    });
  });

  $effect(() => {
    const id = gameId;
    if (!withBackups) return;
    return onBackupEvent((event) => {
      if (event.gameId === id) void loadList(id);
    });
  });

  async function loadList(gameId: string) {
    if (gameId !== loadedFor) return;
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
    if (gameId !== loadedFor) return;
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
      savesError = saveBackupErrorText(err, msg('saves.folderFailed'));
    } finally {
      if (seq === savesSeq) savesLoading = false;
    }
  }

  async function perform(action: () => Promise<void>, fallback: (err: unknown) => string, by: '' | 'open' | 'folder' = '') {
    if (busy) return;
    const mine = turn;
    busy = true;
    acting = by;
    failure = '';
    try {
      await action();
    } catch (err) {
      if (mine === turn) failure = saveBackupErrorText(err, fallback(err));
    } finally {
      if (mine === turn) {
        busy = false;
        acting = '';
        creating = false;
      }
    }
  }

  const backupFailure = () => msg('savebackup.fallback');
  const folderNotSet = () => msg('saves.folderNotSet');

  function create() {
    if (busy) return;
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

  async function cloudSync() {
    if (cloudBusy) return;
    const { gameId } = target;
    const mine = turn;
    cloudBusy = true;
    cloudFailure = '';
    try {
      await cloudSyncNow(gameId);
      toast(msg('cloudsaves.syncDone'), 'success');
    } catch (err) {
      if (mine === turn) cloudFailure = cloudErrorText(err);
    } finally {
      if (mine === turn) cloudBusy = false;
      await refreshCloudStatus();
    }
  }

  async function cloudPull() {
    if (cloudBusy) return;
    const { gameId } = target;
    const mine = turn;
    cloudBusy = true;
    cloudFailure = '';
    try {
      await cloudPullRemote(gameId);
      toast(msg('cloudsaves.pulled'), 'success');
    } catch (err) {
      if (mine === turn) cloudFailure = cloudErrorText(err);
    } finally {
      if (mine === turn) cloudBusy = false;
      await refreshCloudStatus();
      await loadList(gameId);
    }
  }

  async function cloudDelete() {
    if (cloudBusy) return;
    const { gameId } = target;
    const mine = turn;
    cloudBusy = true;
    cloudFailure = '';
    try {
      await cloudDeleteRemote(gameId);
      toast(msg('cloudsaves.deleted'), 'success');
    } catch (err) {
      if (mine === turn) cloudFailure = cloudErrorText(err);
    } finally {
      if (mine === turn) cloudBusy = false;
      await refreshCloudStatus();
    }
  }

  function openCloudSettings() {
    closeSaveBackups();
    navigate('settings', { tab: 'downloads' });
  }

  const attribution = $derived(saveAttribution(saves?.hint));

  async function openSource() {
    const id = canonicalId;
    if (!id) return;
    failure = '';
    try {
      await openReferenceLink(id, 'pcgamingwiki');
    } catch (err) {
      failure = metadataErrorText(err);
    }
  }

  function openSaves() {
    const path = saves?.path;
    if (!path) return;
    return perform(() => openFolder(path), () => msg('saves.openFailed'), 'open');
  }

  // A chosen folder replaces "Choose folder" with the folder and its Change: focus moves on to it.
  async function changeFolder() {
    const { gameId, title } = target;
    const handOff = focusHandOff();
    await perform(async () => {
      const dir = await chooseSavesDir(gameId, title);
      if (dir) await loadSaves(gameId);
    }, folderNotSet, 'folder');
    await handOff(() => document.getElementById(changeId));
  }

  async function useCandidate(dir: string) {
    const { gameId } = target;
    const handOff = focusHandOff();
    candidatesOpen = false;
    await perform(async () => {
      await setSavesDir(gameId, dir);
      await loadSaves(gameId);
    }, folderNotSet);
    await handOff(() => document.getElementById(changeId));
  }

  function askToDelete(snapshot: Snapshot) {
    const at = snapshots.findIndex((s) => s.id === snapshot.id);
    const next = snapshots[at + 1] ?? snapshots[at - 1];
    pending = { kind: 'delete', snapshot, next: next?.id };
  }

  // A deleted backup's row goes with its Delete: focus moves to the next backup's, else to "Back up now".
  function afterDelete(): HTMLElement | null | undefined {
    const next = pending?.next;
    const row = next ? [...(list?.children ?? [])].find((el) => el instanceof HTMLElement && el.dataset.snapshot === next) : undefined;
    const del = row ? [...row.querySelectorAll<HTMLElement>('button')].at(-1) : undefined;
    return del ?? document.getElementById(createId);
  }

  function browseFromCandidates() {
    candidatesOpen = false;
    void changeFolder();
  }
</script>

{#snippet source()}
  {#if attribution}
    <p class="source">
      {msg('saves.hintPrefix')}
      <TextLink disabled={busy} onclick={openSource}>{attribution.source}</TextLink>
      {attribution.via
        ? msg('saves.hintSuffix', { via: attribution.via, license: attribution.license })
        : msg('saves.hintSuffixNoVia', { license: attribution.license })}
    </p>
  {/if}
{/snippet}

<div class="saves">
  {#if withBackups}
    <div class="field">
      <span class="field-label">{msg('saves.folderLabel')}</span>
      {#if savesLoading && !saves && !savesError}
        <Spinner size="sm" label={msg('saves.folderLoading')} />
      {:else if saves?.path}
        <p class="path">{saves.path}</p>
        {@render source()}
        <div class="actions">
          <Button size="sm" disabled={busy && acting !== 'open'} busy={acting === 'open'} onclick={openSaves}>{msg('saves.open')}</Button>
          <Button id={changeId} size="sm" disabled={busy && acting !== 'folder'} busy={acting === 'folder'} onclick={changeFolder}>{msg('saves.change')}</Button>
        </div>
      {:else if savesError}
        <Notice tone="danger" compact>{savesError}</Notice>
        <div class="actions">
          <Button size="sm" disabled={busy && acting !== 'folder'} busy={acting === 'folder'} onclick={changeFolder}>{msg('saves.pickFolder')}</Button>
        </div>
      {:else if saves?.candidates && saves.candidates.length > 0}
        <p class="hint">{msg('ui.savesMultipleCandidates', { title: target.title })}</p>
        {@render source()}
        <div class="actions">
          <Button size="sm" disabled={busy} onclick={() => (candidatesOpen = true)}>
            {msg('saves.pickFolder')}
          </Button>
        </div>
      {:else}
        <p class="hint">{saves && saves.unreadable > 0 ? msg('ui.savesPartialUnreadable') : msg('ui.savesNotFound')}</p>
        {#if saves?.hintsError}
          <p class="hint">{msg('saves.hintsFailed')}</p>
        {/if}
        <div class="actions">
          <Button size="sm" disabled={busy && acting !== 'folder'} busy={acting === 'folder'} onclick={changeFolder}>{msg('saves.pickFolder')}</Button>
        </div>
      {/if}
    </div>
  {/if}

  <div class="field">
    <span class="field-label">{msg('cloudsaves.sectionTitle')}</span>
    {#if !cloudEnabled}
      <p class="hint">{msg('cloudsaves.disabledHint')}</p>
      <div class="actions">
        <Button size="sm" onclick={openCloudSettings}>{msg('cloudsaves.openSettings')}</Button>
      </div>
    {:else if !cloudSignedIn}
      <p class="hint">{msg('cloudsaves.needsAccount')}</p>
    {:else if !canonicalId}
      <p class="hint">{cloudCodeText('cloud.no_canonical_id', msg('cloudsaves.fallback'))}</p>
    {:else}
      <div class="cloud-state">
        {#if cloudGame}
          <StatusBadge kind={cloudStateKind(cloudGame.state)} label={cloudStateLabel(cloudGame.state)} />
        {:else}
          <StatusBadge kind="neutral" label={msg('cloudsaves.stateNone')} />
        {/if}
        {#if cloudSyncedAt}
          <span class="snapshot-meta">{msg('cloudsaves.syncedAt', { date: cloudSyncedAt })}</span>
        {/if}
      </div>
      {#if cloudReason}
        <Notice tone={cloudGame?.state === 'failed' ? 'danger' : 'info'} compact announce={false}>{cloudReason}</Notice>
      {/if}
      {#if cloudFailure}
        <Notice tone="danger" compact>{cloudFailure}</Notice>
      {/if}
      <div class="actions">
        <Button size="sm" busy={cloudBusy} onclick={cloudSync}>
          {cloudBusy
            ? msg('cloudsaves.syncing')
            : cloudGame?.code === 'cloud.excluded'
              ? msg('cloudsaves.enableAgain')
              : msg('cloudsaves.syncNow')}
        </Button>
        {#if cloudGame?.state === 'conflict'}
          <Button size="sm" variant="primary" disabled={cloudBusy} onclick={() => openCloudConflict(gameId)}>
            {msg('cloudsaves.resolve')}
          </Button>
        {/if}
        {#if cloudGame && cloudGame.state !== 'conflict' && cloudGame.code !== 'cloud.excluded'}
          <Button size="sm" disabled={cloudBusy || gameRunning} onclick={() => (cloudPullAsked = true)}>
            {msg('cloudsaves.pull')}
          </Button>
        {/if}
        <Button size="sm" variant="danger" disabled={cloudBusy} onclick={() => (cloudDeleteAsked = true)}>
          {msg('cloudsaves.deleteRemote')}
        </Button>
      </div>
    {/if}
  </div>

  {#if withBackups}
    <div class="list">
      <div class="list-head">
        <span class="field-label">{msg('saves.listTitle')}</span>
        <Button id={createId} variant="primary" size="sm" disabled={(busy && !creating) || !saves?.path} busy={creating} onclick={create}>
          {creating ? msg('saves.creating') : msg('saves.createBackup')}
        </Button>
      </div>

      {#if failure}
        <Notice tone="danger" compact>{failure}</Notice>
      {/if}
      {#if listError}
        <Notice tone="danger" compact>{listError}</Notice>
      {/if}
      {#if gameRunning}
        <Notice tone="warning" compact announce={false}>{msg('saves.gameRunning')}</Notice>
      {/if}
      {#if snapshots.some((s) => s.large)}
        <Notice tone="warning" compact announce={false}>{msg('saves.largeWarning')}</Notice>
      {/if}

      {#if snapshots.length > 0}
        <ul class="snapshots" bind:this={list}>
          {#each snapshots as snapshot (snapshot.id)}
            <li class="snapshot" data-snapshot={snapshot.id}>
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
                  {#if snapshotPlayedAfter(snapshot)}
                    <span class="snapshot-meta">{snapshotPlayedAfter(snapshot)}</span>
                  {/if}
                {/if}
              </div>
              <div class="actions">
                {#if !snapshot.broken}
                  <Button size="sm" disabled={busy || gameRunning} onclick={() => (pending = { kind: 'restore', snapshot })}>
                    {msg('saves.restore')}
                  </Button>
                {/if}
                <Button size="sm" variant="danger" disabled={busy} onclick={() => askToDelete(snapshot)}>
                  {msg('saves.delete')}
                </Button>
              </div>
            </li>
          {/each}
        </ul>
      {:else if listLoading}
        <Spinner size="sm" label={msg('common.loading')} />
      {:else if !listError}
        <div class="no-saves">
          <EmptyState size="inline" align="center" lead title={msg('saves.empty')} description={msg('saves.emptyHint')} />
        </div>
      {/if}
    </div>
  {/if}
</div>


{#if saves?.candidates && saves.candidates.length > 0}
  <SavesCandidatesModal
    bind:open={candidatesOpen}
    title={target.title}
    candidates={saves.candidates}
    onpick={useCandidate}
    onbrowse={browseFromCandidates}
  />
{/if}

{#if cloudPullAsked}
  <ConfirmModal prompt={pullCloudSavesPrompt()} onconfirm={cloudPull} onclose={() => (cloudPullAsked = false)} />
{/if}

{#if cloudDeleteAsked}
  <ConfirmModal prompt={deleteCloudSavesPrompt()} onconfirm={cloudDelete} onclose={() => (cloudDeleteAsked = false)} />
{/if}

{#if pending}
  <ConfirmModal
    prompt={pending.kind === 'restore' ? restoreSavesPrompt() : deleteSaveBackupPrompt(snapshotWhen(pending.snapshot))}
    onconfirm={runPending}
    onclose={() => (pending = null)}
    errorText={(err) => saveBackupErrorText(err)}
    returnFocus={afterDelete}
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

  .source {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
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

  .cloud-state {

    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
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
    /* The dialog's own surface, so that on the game page a damaged backup's reason reads over the art. */
    background: var(--surface-2);
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

  .no-saves {
    padding: var(--space-5) 0;
  }
</style>
