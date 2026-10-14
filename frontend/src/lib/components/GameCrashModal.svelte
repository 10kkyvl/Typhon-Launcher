<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import { crashReasonText } from '../redist/messages';
  import { closeCrash, crashTarget } from '../stores/crash';
  import { libraryGames } from '../stores/library';
  import { openRedist } from '../stores/redist';
  import { navigate } from '../stores/router';
  import { verify } from '../stores/updates';

  const record = $derived($crashTarget);
  const title = $derived(record ? ($libraryGames.find((g) => g.id === record.gameId)?.title ?? '') : '');
  const reason = $derived(record ? crashReasonText(record.reasonCode) : '');
  const code = $derived(record ? `0x${(record.exitCode >>> 0).toString(16).toUpperCase()}` : '');

  let open = $state(false);

  $effect(() => {
    open = record !== null;
  });

  function dismiss() {
    open = false;
    closeCrash();
  }

  function checkFiles() {
    if (!record) return;
    const id = record.gameId;
    dismiss();
    navigate('game', { id });
    void verify(id);
  }

  function installLibs() {
    if (!record) return;
    const id = record.gameId;
    dismiss();
    openRedist(id);
  }
</script>

<Modal bind:open title={msg('redist.crashTitle')} width="48rem" onclose={dismiss}>
  {#if record}
    <div class="body">
      <p>{msg('redist.crashGame', { title, seconds: Math.round(record.sessionSeconds) })}</p>
      {#if reason}
        <p>{msg('redist.crashReason', { reason })}</p>
      {/if}
      {#if record.exitKnown}
        <p class="code">{msg('redist.crashCode', { code })}</p>
      {/if}
      {#if record.count > 1}
        <p class="repeat">{msg('redist.crashRepeat', { count: record.count })}</p>
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    <Button onclick={dismiss}>{msg('common.close')}</Button>
    <Button onclick={installLibs}>{msg('redist.crashLibs')}</Button>
    <Button variant="primary" onclick={checkFiles}>{msg('redist.crashVerify')}</Button>
  {/snippet}
</Modal>

<style>
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  p {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .code {
    color: var(--text-3);
  }

  .repeat {
    color: var(--warning);
  }
</style>
