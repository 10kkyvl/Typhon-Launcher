<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { cloudErrorText } from '../cloudsaves/messages';
  import { msg } from '../i18n';
  import { cloudResolveConflict } from '../services/cloudsaves';
  import { closeCloudConflict, cloudConflictTarget, cloudStatus, refreshCloudStatus } from '../stores/cloudsaves';
  import { toast } from '../stores/toasts';
  import { bytesSize, dateTime } from '../utils/format';

  const target = $derived($cloudConflictTarget);
  const conflict = $derived(target ? $cloudStatus?.games.find((g) => g.gameId === target.gameId)?.conflict : undefined);

  let busy = $state(false);
  let failure = $state('');

  $effect(() => {
    if (!target) {
      failure = '';
    }
  });

  function when(iso: string): string {
    const value = new Date(iso);
    return Number.isNaN(value.getTime()) ? iso : dateTime(value);
  }

  async function resolve(keepLocal: boolean) {
    if (!target || busy) return;
    const { gameId } = target;
    busy = true;
    failure = '';
    try {
      await cloudResolveConflict(gameId, keepLocal);
      toast(msg(keepLocal ? 'cloudsaves.conflictKeptMine' : 'cloudsaves.conflictTookRemote'), 'success');
      closeCloudConflict();
    } catch (err) {
      failure = cloudErrorText(err);
    } finally {
      busy = false;
      await refreshCloudStatus();
    }
  }
</script>

<Modal
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
      <p class="warning">{msg('cloudsaves.conflictKeepMineWarning')}</p>
      <p class="warning">{msg('cloudsaves.conflictTakeRemoteWarning')}</p>
    {:else}
      <p class="hint">{msg('cloudsaves.conflictLoading')}</p>
    {/if}
    {#if failure}
      <p class="failure">{failure}</p>
    {/if}
  {/if}

  {#snippet footer()}
    <Button disabled={busy} onclick={closeCloudConflict}>{msg('cloudsaves.conflictLater')}</Button>
    <Button disabled={busy || !conflict} onclick={() => resolve(true)}>{msg('cloudsaves.conflictKeepMine')}</Button>
    <Button variant="primary" disabled={busy || !conflict} onclick={() => resolve(false)}>
      {msg('cloudsaves.conflictTakeRemote')}
    </Button>
  {/snippet}
</Modal>

<style>
  .intro,
  .hint {
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

  .warning {
    margin: var(--space-4) 0 0;
    font-size: var(--font-xs);
    color: var(--warning);
  }

  .failure {
    margin: var(--space-3) 0 0;
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
