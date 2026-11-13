<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import { cloudErrorText } from '../cloudsaves/messages';
  import { msg } from '../i18n';
  import { untrack } from 'svelte';
  import { cloudResolveConflict } from '../services/cloudsaves';
  import {
    closeCloudConflict,
    cloudConflictTarget,
    cloudStatus,
    fetchCloudStatus,
    finishCloudConflict,
    refreshCloudStatus,
    type CloudConflictTarget,
  } from '../stores/cloudsaves';
  import { toast } from '../stores/toasts';
  import { bytesSize, dateTime } from '../utils/format';

  const target = $derived($cloudConflictTarget);
  const gameId = $derived(target?.gameId ?? '');
  const conflict = $derived(gameId ? $cloudStatus?.games.find((g) => g.gameId === gameId)?.conflict : undefined);

  let busy = $state(false);
  // The choice being applied: its button stays focused and busy, the other waits.
  let kept = $state<'mine' | 'remote' | ''>('');
  let failure = $state('');
  // Why the details are not shown; empty while they load.
  let detailsFailure = $state('');
  let retrying = $state(false);

  $effect(() => {
    if (!target) {
      failure = '';
    }
  });

  // A conflict recorded before this session comes with no event, and a launch it held back opens the
  // dialog by itself: the details are loaded here whenever the status does not list them yet. Every
  // opening starts from the loading state, whatever a load or a Retry of the one before left behind.
  $effect(() => {
    const opening = target;
    detailsFailure = '';
    retrying = false;
    if (opening && !untrack(() => conflict)) void loadDetails(opening);
  });

  // True when the details are there; undefined when the dialog has closed or moved on meanwhile.
  async function loadDetails(opening: CloudConflictTarget | null): Promise<boolean | undefined> {
    if (!opening) return undefined;
    let reason = '';
    try {
      await fetchCloudStatus();
    } catch (err) {
      console.error('cloud conflict details', err);
      reason = cloudErrorText(err, msg('cloudsaves.statusFailed'));
    }
    if (target !== opening) return undefined;
    if (conflict) {
      detailsFailure = '';
      return true;
    }
    detailsFailure = reason || msg('cloudsaves.statusFailed');
    return false;
  }

  function when(iso: string): string {
    const value = new Date(iso);
    return Number.isNaN(value.getTime()) ? iso : dateTime(value);
  }

  async function resolve(keepLocal: boolean) {
    const current = target;
    if (!current || busy) return;
    busy = true;
    kept = keepLocal ? 'mine' : 'remote';
    failure = '';
    try {
      await cloudResolveConflict(current.gameId, keepLocal);
      toast(msg(keepLocal ? 'cloudsaves.conflictKeptMine' : 'cloudsaves.conflictTookRemote'), 'success');
      finishCloudConflict(current);
    } catch (err) {
      failure = cloudErrorText(err);
    } finally {
      busy = false;
      kept = '';
      await refreshCloudStatus();
    }
  }
</script>

<Modal
  dismissible={!busy}
  open={!!target}
  title={msg('cloudsaves.conflictTitle')}
  width="56rem"
  onclose={closeCloudConflict}
>
  {#if target}
    <p class="intro">{msg('cloudsaves.conflictIntro', { title: target.title })}</p>
    {#if conflict}
      <div class="columns">
        <div class="column">
          <span class="column-title">{msg('cloudsaves.conflictLocal')}</span>
          <span class="line">{msg('cloudsaves.conflictUpdated')}: {when(conflict.localUpdatedAt)}</span>
          <span class="line">{msg('cloudsaves.conflictSize')}: {bytesSize(conflict.localSize)}</span>
        </div>
        <div class="column">
          <span class="column-title">{msg('cloudsaves.conflictRemote')}</span>
          <span class="line">{msg('cloudsaves.conflictUpdated')}: {when(conflict.remoteUpdatedAt)}</span>
          <span class="line">{msg('cloudsaves.conflictSize')}: {bytesSize(conflict.remoteSize)}</span>
          {#if conflict.remoteDevice}
            <span class="line">{msg('cloudsaves.conflictDevice')}: {conflict.remoteDevice}</span>
          {/if}
        </div>
      </div>
      <div class="notice">
        <Notice tone="warning" compact announce={false}>
          <p>{msg('cloudsaves.conflictKeepMineWarning')}</p>
          <p>{msg('cloudsaves.conflictTakeRemoteWarning')}</p>
        </Notice>
      </div>
    {:else if detailsFailure || retrying}
      <Notice
        tone="danger"
        compact
        bind:retrying
        retry={{ label: msg('common.retry'), run: () => loadDetails(target) }}
      >
        {detailsFailure}
      </Notice>
    {:else}
      <Spinner size="sm" label={msg('cloudsaves.conflictLoading')} />
    {/if}
    {#if failure}
      <div class="notice"><Notice tone="danger" compact>{failure}</Notice></div>
    {/if}
  {/if}

  {#snippet footer()}
    <Button disabled={busy} onclick={closeCloudConflict}>{msg('cloudsaves.conflictLater')}</Button>
    <Button disabled={(busy && kept !== 'mine') || !conflict} busy={kept === 'mine'} onclick={() => resolve(true)}>
      {msg('cloudsaves.conflictKeepMine')}
    </Button>
    <Button variant="primary" disabled={(busy && kept !== 'remote') || !conflict} busy={kept === 'remote'} onclick={() => resolve(false)}>
      {msg('cloudsaves.conflictTakeRemote')}
    </Button>
  {/snippet}
</Modal>

<style>
  .intro {
    margin: 0 0 var(--space-4);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .columns {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-4);
  }

  .column {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .column-title {
    font-size: var(--font-md);
    color: var(--text);
  }

  .line {
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-word;
  }

  .notice {
    margin: var(--space-4) 0 0;
  }

  .notice p {
    margin: 0;
  }

  .notice p + p {
    margin-top: var(--space-2);
  }
</style>
