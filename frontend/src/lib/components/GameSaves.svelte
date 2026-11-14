<script lang="ts">
  import { untrack } from 'svelte';
  import GameSavesBackups from './GameSavesBackups.svelte';
  import GameSavesCloud, { type CloudAvailability } from './GameSavesCloud.svelte';
  import GameSavesFolder from './GameSavesFolder.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import SavesCandidatesModal from './SavesCandidatesModal.svelte';
  import { deleteCloudSavesPrompt, pullCloudSavesPrompt, deleteSaveBackupPrompt, restoreSavesPrompt } from '../confirm/prompts';
  import { chooseSavesDir } from '../game/actions/saves';
  import { cloudErrorText } from '../cloudsaves/messages';
  import { errorCode, msg } from '../i18n';
  import { saveBackupErrorText, snapshotWhen } from '../savebackup/messages';
  import { cloudDeleteRemote, cloudPullRemote, cloudSyncNow } from '../services/cloudsaves';
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
  const availability = $derived<CloudAvailability>(!cloudEnabled ? 'off' : !cloudSignedIn ? 'signed-out' : !canonicalId ? 'no-catalog' : 'on');

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

<div class="saves">
  {#if withBackups}
    <GameSavesFolder
      {saves}
      loading={savesLoading}
      error={savesError}
      title={target.title}
      {busy}
      {acting}
      {changeId}
      onopen={openSaves}
      onchange={changeFolder}
      onpick={() => (candidatesOpen = true)}
      onsource={openSource}
    />
  {/if}

  <div class="field">
    <span class="field-label">{msg('cloudsaves.sectionTitle')}</span>
    <GameSavesCloud
      {availability}
      game={cloudGame}
      failure={cloudFailure}
      busy={cloudBusy}
      running={gameRunning}
      onsettings={openCloudSettings}
      onsync={cloudSync}
      onresolve={() => openCloudConflict(gameId)}
      onpull={() => (cloudPullAsked = true)}
      ondelete={() => (cloudDeleteAsked = true)}
    />
  </div>

  {#if withBackups}
    <GameSavesBackups
      {snapshots}
      loading={listLoading}
      error={listError}
      {failure}
      running={gameRunning}
      {busy}
      {creating}
      canCreate={Boolean(saves?.path)}
      {createId}
      bind:list
      oncreate={create}
      onrestore={(snapshot) => (pending = { kind: 'restore', snapshot })}
      ondelete={askToDelete}
    />
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

  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
