<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import { cloudErrorText, cloudUnreachable } from '../cloudsaves/messages';
  import { errorCode, msg } from '../i18n';
  import { untrack } from 'svelte';
  import { cloudResolveConflict } from '../services/cloudsaves';
  import {
    allowConflictLaunch,
    closeCloudConflict,
    cloudConflictLaunch,
    cloudConflictTarget,
    cloudStatus,
    fetchCloudStatus,
    finishCloudConflict,
    refreshCloudStatus,
    type CloudConflictTarget,
  } from '../stores/cloudsaves';
  import { toast } from '../stores/toasts';
  import { bytesSize, dateTime } from '../utils/format';
  import { focusHandOff } from '../utils/focusHandOff';

  const target = $derived($cloudConflictTarget);
  const gameId = $derived(target?.gameId ?? '');
  const conflict = $derived(gameId ? $cloudStatus?.games.find((g) => g.gameId === gameId)?.conflict : undefined);
  const launch = $derived($cloudConflictLaunch);

  const uid = $props.id();
  const mineId = `${uid}-mine`;
  const noteId = `${uid}-note`;

  let busy = $state(false);
  // The choice being applied: its button stays focused and busy, the other waits.
  let kept = $state<'mine' | 'remote' | ''>('');
  let allowing = $state(false);
  let failure = $state('');
  // Why the details are not shown; empty while they load.
  let detailsFailure = $state('');
  let retrying = $state(false);

  // Without the cloud no copy can be chosen. The dialog is offline from a fact of this opening: a choice
  // or a details load that could not reach it (`unreachable`, with the choice to try again), or, until
  // the player tries again (`statusPassed`), a status whose last pass could not reach the cloud
  // (`error`) or could not confirm the session (`pending`: a pass that cannot tell whose account is
  // signed in, as in a launcher started without a connection; the server answers no listing "busy").
  let unreachable = $state(false);
  let lastChoice = $state<boolean | null>(null);
  let statusPassed = $state(false);
  let offlineRetrying = $state(false);
  const statusOffline = $derived(cloudUnreachable($cloudStatus?.error ?? '') || $cloudStatus?.pending === true);
  const offline = $derived(unreachable || (!statusPassed && statusOffline) || offlineRetrying);

  // A conflict recorded before this session comes with no event, and a launch it held back opens the
  // dialog by itself: the details are loaded here whenever the status does not list them yet. Every
  // opening starts from the loading state and from the choice, whatever the one before left behind.
  $effect(() => {
    const opening = target;
    failure = '';
    detailsFailure = '';
    retrying = false;
    unreachable = false;
    lastChoice = null;
    statusPassed = false;
    offlineRetrying = false;
    if (opening && !untrack(() => conflict)) void loadDetails(opening);
  });

  // True when the details are there; undefined when the dialog has closed or moved on meanwhile.
  async function loadDetails(opening: CloudConflictTarget | null): Promise<boolean | undefined> {
    if (!opening) return undefined;
    let reason = '';
    let code = '';
    try {
      await fetchCloudStatus();
    } catch (err) {
      console.error('cloud conflict details', err);
      code = errorCode(err);
      reason = cloudErrorText(err, msg('cloudsaves.statusFailed'));
    }
    if (target !== opening) return undefined;
    if (cloudUnreachable(code)) unreachable = true;
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

  // Applies a choice: 'offline' when the cloud could not be reached, undefined when there was nothing to
  // do or the dialog moved on meanwhile.
  async function apply(keepLocal: boolean): Promise<'done' | 'offline' | 'failed' | undefined> {
    const current = target;
    if (!current || busy) return undefined;
    busy = true;
    kept = keepLocal ? 'mine' : 'remote';
    failure = '';
    try {
      await cloudResolveConflict(current.gameId, keepLocal);
      toast(msg(keepLocal ? 'cloudsaves.conflictKeptMine' : 'cloudsaves.conflictTookRemote'), 'success');
      finishCloudConflict(current);
      return 'done';
    } catch (err) {
      if (target !== current) return undefined;
      if (cloudUnreachable(errorCode(err))) {
        unreachable = true;
        lastChoice = keepLocal;
        return 'offline';
      }
      failure = cloudErrorText(err);
      return 'failed';
    } finally {
      busy = false;
      kept = '';
      await refreshCloudStatus();
    }
  }

  // The pressed choice goes away with the offline state: focus it held moves on to try again.
  async function choose(keepLocal: boolean) {
    const handOff = focusHandOff();
    if ((await apply(keepLocal)) === 'offline') await handOff(() => offlineBox?.querySelector('button'));
  }

  let offlineBox = $state<HTMLElement>();

  // Try again repeats the choice that could not reach the cloud; with none, it gives the choice back.
  async function tryAgain(opening: CloudConflictTarget | null): Promise<boolean | undefined> {
    if (!opening || target !== opening) return undefined;
    if (busy) return false;
    statusPassed = true;
    const choice = lastChoice;
    if (choice === null) {
      unreachable = false;
      return true;
    }
    const result = await apply(choice);
    if (result === undefined) return target === opening ? false : undefined;
    if (result === 'offline') return false;
    unreachable = false;
    lastChoice = null;
    return true;
  }

  async function playHere() {
    const current = target;
    if (!current || busy) return;
    busy = true;
    allowing = true;
    failure = '';
    try {
      await allowConflictLaunch(current);
    } catch (err) {
      if (target === current) failure = cloudErrorText(err);
    } finally {
      busy = false;
      allowing = false;
    }
  }
</script>

<Modal
  dismissible={!busy && !allowing}
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
      {#if offline}
        <div class="notice" bind:this={offlineBox}>
          <Notice
            tone="warning"
            compact
            bind:retrying={offlineRetrying}
            retry={{ label: msg('common.retry'), run: () => tryAgain(target), focus: () => document.getElementById(mineId) }}
          >
            {msg('cloudsaves.conflictOffline', { reason: msg('cloud.network') })}
          </Notice>
        </div>
      {:else}
        <div class="notice">
          <Notice tone="warning" compact announce={false}>
            <p>{msg('cloudsaves.conflictKeepMineWarning')}</p>
            <p>{msg('cloudsaves.conflictTakeRemoteWarning')}</p>
          </Notice>
        </div>
      {/if}
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
    {#if offline && launch}
      <p class="note" id={noteId}>{msg('cloudsaves.conflictPlayHereNote')}</p>
    {/if}
    {#if failure}
      <div class="notice"><Notice tone="danger" compact>{failure}</Notice></div>
    {/if}
  {/if}

  {#snippet footer()}
    <Button disabled={busy} onclick={closeCloudConflict}>{msg('cloudsaves.conflictLater')}</Button>
    {#if offline}
      {#if launch}
        <Button variant="primary" disabled={busy && !allowing} busy={allowing} aria-describedby={noteId} onclick={playHere}>
          {msg('cloudsaves.conflictPlayHere')}
        </Button>
      {/if}
    {:else}
      <Button id={mineId} disabled={(busy && kept !== 'mine') || !conflict} busy={kept === 'mine'} onclick={() => choose(true)}>
        {msg('cloudsaves.conflictKeepMine')}
      </Button>
      <Button variant="primary" disabled={(busy && kept !== 'remote') || !conflict} busy={kept === 'remote'} onclick={() => choose(false)}>
        {msg('cloudsaves.conflictTakeRemote')}
      </Button>
    {/if}
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

  .note {
    margin: var(--space-3) 0 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }
</style>
