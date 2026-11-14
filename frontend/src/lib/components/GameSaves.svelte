<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
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
  import { closeSaveBackups, runSavesWork, saveBackupsEnabled, savesInFlight, type SavesInFlight, type SavesWork } from '../stores/savebackup';
  import { toast } from '../stores/toasts';
  import { authState } from '../stores/user';
  import { focusableIn } from '../layers/focus';
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
   * dialog is not dismissed meanwhile. A backup or cloud action runs in the store (`savesInFlight`), so
   * a view mounted while it runs (the dialog opened again, the tab entered again) shows it busy, starts
   * no second one and loads the list again once it ends.
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
  const syncId = `${uid}-sync`;
  let list = $state<HTMLElement>();
  let root = $state<HTMLElement>();

  const game = $derived($libraryGames.find((g) => g.id === gameId));
  // What an action was started for: the game as it was at the press.
  const target = $derived({ gameId, title: game?.title ?? '' });
  const gameRunning = $derived($runningGames.has(gameId));
  const canonicalId = $derived(game?.canonicalGameId ?? '');
  const cloudEnabled = $derived($settings?.cloudSaves ?? false);
  const cloudSignedIn = $derived($authState === 'authenticated');
  const cloudGame = $derived($cloudStatus?.games.find((g) => g.gameId === gameId));
  const availability = $derived<CloudAvailability>(!cloudEnabled ? 'off' : !cloudSignedIn ? 'signed-out' : !canonicalId ? 'no-catalog' : 'on');

  // Resolve goes once the conflict is settled: focus it held (or that came back to it from the
  // conflict dialog) moves on to Sync now, else to the view's first control.
  const resolvable = $derived(availability === 'on' && cloudGame?.state === 'conflict');
  let wasResolvable = false;
  $effect.pre(() => {
    const now = resolvable;
    untrack(() => {
      if (wasResolvable && !now) void focusHandOff()(() => document.getElementById(syncId) ?? (root ? focusableIn(root)[0] : undefined));
      wasResolvable = now;
    });
  });

  let snapshots = $state<Snapshot[]>([]);
  let listLoading = $state(false);
  let listError = $state('');
  let saves = $state<SavesResult | null>(null);
  let savesLoading = $state(false);
  let savesError = $state('');
  // A folder action of this view runs; `acting` is its button, which stays focused and busy.
  let folderBusy = $state(false);
  let acting = $state<'' | 'open' | 'folder'>('');
  let failure = $state('');
  let pending = $state<Pending | null>(null);
  let candidatesOpen = $state(false);
  let cloudFailure = $state('');

  // This view, as who started an action: one started elsewhere loads the list again when it ends.
  const me = Symbol('saves view');
  const flight = $derived($savesInFlight.get(gameId));
  const backupWork = $derived(flight?.backup);
  const creating = $derived(backupWork?.kind === 'create');
  const cloudBusy = $derived(Boolean(flight?.cloud));
  const working = $derived(Boolean(backupWork) || folderBusy);

  $effect(() => {
    busy = working;
  });

  let alive = true;
  onDestroy(() => {
    alive = false;
  });
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
      folderBusy = false;
      acting = '';
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

  // An action started by another view (one closed meanwhile) changed the backups: load them again.
  let seen: SavesInFlight | undefined;
  let seenFor = '';
  $effect(() => {
    const now = flight;
    const id = gameId;
    const loads = withBackups;
    untrack(() => {
      const before = seenFor === id ? seen : undefined;
      seen = now;
      seenFor = id;
      const ended = (work: SavesWork | undefined, after: SavesWork | undefined) => Boolean(work) && work?.by !== me && after !== work;
      const changed = ended(before?.backup, now?.backup) || (before?.cloud?.kind === 'pull' && ended(before.cloud, now?.cloud));
      if (loads && changed) void loadList(id);
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
    if (!alive || gameId !== loadedFor) return;
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
    if (!alive || gameId !== loadedFor) return;
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
    if (working) return;
    const mine = turn;
    folderBusy = true;
    acting = by;
    failure = '';
    try {
      await action();
    } catch (err) {
      if (mine === turn) failure = saveBackupErrorText(err, fallback(err));
    } finally {
      if (mine === turn) {
        folderBusy = false;
        acting = '';
      }
    }
  }

  // A backup action, in the game's backup lane: refused while one runs there, from this view or another.
  // A failure this view no longer shows (it went, or moved to another game) is said in a toast.
  async function backupAction(gameId: string, work: Omit<SavesWork, 'by'>, action: () => Promise<void>) {
    if (working) return;
    const mine = turn;
    const { title } = target;
    failure = '';
    try {
      await runSavesWork(gameId, 'backup', { ...work, by: me }, action);
    } catch (err) {
      const text = saveBackupErrorText(err, backupFailure());
      if (alive && mine === turn) failure = text;
      else toast(msg('saves.failedFor', { title, reason: text }), 'danger');
    }
  }

  // A cloud action, in the game's cloud lane; the status is loaded again whatever the outcome.
  async function cloudAction(work: SavesWork['kind'], action: (gameId: string) => Promise<void>, done: () => Promise<void> = async () => {}) {
    if (cloudBusy) return;
    const { gameId, title } = target;
    const mine = turn;
    cloudFailure = '';
    try {
      await runSavesWork(gameId, 'cloud', { kind: work, by: me }, () => action(gameId));
    } catch (err) {
      const text = cloudErrorText(err);
      if (alive && mine === turn) cloudFailure = text;
      else toast(msg('cloudsaves.failedFor', { title, reason: text }), 'danger');
    } finally {
      await refreshCloudStatus();
      await done();
    }
  }

  const backupFailure = () => msg('savebackup.fallback');
  const folderNotSet = () => msg('saves.folderNotSet');

  function create() {
    const { gameId } = target;
    return backupAction(gameId, { kind: 'create' }, async () => {
      try {
        await createBackup(gameId);
        toast(msg('saves.created'), 'success');
      } catch (err) {
        if (errorCode(err) !== 'savebackup.rotation_failed') throw err;
        toast(msg('savebackup.rotation_failed'));
      } finally {
        await loadList(gameId);
      }
    });
  }

  async function restore(snapshot: Snapshot) {
    const { gameId } = target;
    await backupAction(gameId, { kind: 'restore', snapshot: snapshot.id }, async () => {
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
    });
  }

  async function remove(snapshot: Snapshot) {
    const { gameId } = target;
    await backupAction(gameId, { kind: 'delete', snapshot: snapshot.id }, async () => {
      try {
        await deleteBackup(gameId, snapshot.id);
        toast(msg('saves.deleted'), 'success');
      } finally {
        await loadList(gameId);
      }
    });
  }

  async function runPending() {
    const current = pending;
    if (!current) return;
    if (current.kind === 'restore') await restore(current.snapshot);
    else await remove(current.snapshot);
  }

  const cloudSync = () =>
    cloudAction('sync', async (gameId) => {
      await cloudSyncNow(gameId);
      toast(msg('cloudsaves.syncDone'), 'success');
    });

  const cloudPull = () => {
    const { gameId } = target;
    return cloudAction(
      'pull',
      async () => {
        await cloudPullRemote(gameId);
        toast(msg('cloudsaves.pulled'), 'success');
      },
      () => loadList(gameId),
    );
  };

  const cloudDelete = () =>
    cloudAction('delete-remote', async (gameId) => {
      await cloudDeleteRemote(gameId);
      toast(msg('cloudsaves.deleted'), 'success');
    });

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

<div class="saves" bind:this={root}>
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
      {syncId}
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
      working={backupWork}
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
  <ConfirmModal prompt={pullCloudSavesPrompt()} onconfirm={cloudPull} onclose={() => (cloudPullAsked = false)} errorText={(err) => cloudErrorText(err)} />
{/if}

{#if cloudDeleteAsked}
  <ConfirmModal prompt={deleteCloudSavesPrompt()} onconfirm={cloudDelete} onclose={() => (cloudDeleteAsked = false)} errorText={(err) => cloudErrorText(err)} />
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
