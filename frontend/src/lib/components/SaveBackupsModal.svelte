<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import Modal from './Modal.svelte';
  import SavesCandidatesModal from './SavesCandidatesModal.svelte';
  import StatusBadge from './StatusBadge.svelte';
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
  import { closeSaveBackups, saveBackupTarget } from '../stores/savebackup';
  import { toast } from '../stores/toasts';
  import { authState } from '../stores/user';
  import { errorMessage } from '../utils/errors';
  import { bytesSize, dateTime } from '../utils/format';

  interface Pending {
    kind: 'restore' | 'delete';
    snapshot: Snapshot;
  }

  const target = $derived($saveBackupTarget);
  const gameRunning = $derived(target ? $runningGames.has(target.gameId) : false);
  const canonicalId = $derived(
    target ? ($libraryGames.find((g) => g.id === target.gameId)?.canonicalGameId ?? '') : '',
  );
  const cloudEnabled = $derived($settings?.cloudSaves ?? false);
  const cloudSignedIn = $derived($authState === 'authenticated');
  const cloudGame = $derived(target ? $cloudStatus?.games.find((g) => g.gameId === target.gameId) : undefined);
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
  let busy = $state(false);
  let creating = $state(false);
  let failure = $state('');
  let pending = $state<Pending | null>(null);
  let candidatesOpen = $state(false);
  let cloudBusy = $state(false);
  let cloudFailure = $state('');
  let cloudDeleteAsked = $state(false);
  let cloudPullAsked = $state(false);

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
      cloudFailure = '';
      cloudDeleteAsked = false;
      cloudPullAsked = false;
      void refreshCloudStatus();
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

  async function cloudSync() {
    if (!target || cloudBusy) return;
    const { gameId } = target;
    cloudBusy = true;
    cloudFailure = '';
    try {
      await cloudSyncNow(gameId);
      toast(msg('cloudsaves.syncDone'), 'success');
    } catch (err) {
      cloudFailure = cloudErrorText(err);
    } finally {
      cloudBusy = false;
      await refreshCloudStatus();
    }
  }

  async function cloudPull() {
    if (!target || cloudBusy) return;
    const { gameId } = target;
    cloudBusy = true;
    cloudFailure = '';
    try {
      await cloudPullRemote(gameId);
      toast(msg('cloudsaves.pulled'), 'success');
    } catch (err) {
      cloudFailure = cloudErrorText(err);
    } finally {
      cloudBusy = false;
      await refreshCloudStatus();
      await loadList(gameId);
    }
  }

  async function cloudDelete() {
    if (!target || cloudBusy) return;
    const { gameId } = target;
    cloudBusy = true;
    cloudFailure = '';
    try {
      await cloudDeleteRemote(gameId);
      toast(msg('cloudsaves.deleted'), 'success');
    } catch (err) {
      cloudFailure = cloudErrorText(err);
    } finally {
      cloudBusy = false;
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

{#snippet source()}
  {#if attribution}
    <p class="source">
      {msg('saves.hintPrefix')}
      <button class="source-link" disabled={busy} onclick={openSource}>{attribution.source}</button>
      {attribution.via
        ? msg('saves.hintSuffix', { via: attribution.via, license: attribution.license })
        : msg('saves.hintSuffixNoVia', { license: attribution.license })}
    </p>
  {/if}
{/snippet}

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
          {@render source()}
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
            <Button size="sm" disabled={busy} onclick={changeFolder}>{msg('saves.pickFolder')}</Button>
          </div>
        {/if}
      </div>

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
              <span class="hint">{msg('cloudsaves.stateNone')}</span>
            {/if}
            {#if cloudSyncedAt}
              <span class="snapshot-meta">{msg('cloudsaves.syncedAt', { date: cloudSyncedAt })}</span>
            {/if}
          </div>
          {#if cloudReason}
            <p class={cloudGame?.state === 'failed' ? 'failure' : 'hint'}>{cloudReason}</p>
          {/if}
          {#if cloudFailure}
            <p class="failure">{cloudFailure}</p>
          {/if}
          <div class="actions">
            <Button size="sm" disabled={cloudBusy} onclick={cloudSync}>
              {cloudBusy
                ? msg('cloudsaves.syncing')
                : cloudGame?.code === 'cloud.excluded'
                  ? msg('cloudsaves.enableAgain')
                  : msg('cloudsaves.syncNow')}
            </Button>
            {#if cloudGame?.state === 'conflict'}
              <Button size="sm" variant="primary" disabled={cloudBusy} onclick={() => target && openCloudConflict(target.gameId)}>
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
        {#if snapshots.some((s) => s.large)}
          <p class="warning">{msg('saves.largeWarning')}</p>
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

  .source-link {
    padding: 0;
    color: var(--accent);
    text-decoration: underline;
    font-size: inherit;
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
