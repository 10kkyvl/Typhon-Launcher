<script lang="ts">
  import { RotateCcw } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { handoffFailed, handoffUndoFailed, requestApply, requestHandoff, selfUpdateApplying, selfUpdateStatus } from '../stores/selfupdate';
  import { updateShow } from '../stores/updateShow';
  import { msg } from '../i18n';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  // This dialog's confirmation started a seamless restart: if Go rolls it back, the choice comes back
  // here once the show has gone home, so the player answers it where they asked. An undo Go could not
  // make is said here too, with no way to try again: Go refuses one until the next start.
  let waiting = $state(false);

  $effect(() => {
    if (!waiting || $selfUpdateApplying || $updateShow) return;
    waiting = false;
    if ($handoffFailed || $handoffUndoFailed) open = true;
  });

  function confirm() {
    open = false;
    waiting = true;
    void requestHandoff();
  }

  function restartInstead() {
    open = false;
    waiting = false;
    void requestApply();
  }
</script>

<Modal bind:open title={msg($handoffFailed || $handoffUndoFailed ? 'ui.handoffFailedTitle' : 'ui.restartLauncherTitle')}>
  <p class="modal-text">
    {#if $handoffUndoFailed}
      {msg('state.selfupdateHandoffUndoFailed')}
    {:else if $handoffFailed}
      {msg('state.selfupdateHandoffFailed')}
    {:else}
      {msg('ui.restartLauncherWarning', { version: $selfUpdateStatus.availableVersion ?? '' })}
    {/if}
  </p>
  {#snippet footer()}
    {#if $handoffUndoFailed}
      <Button variant="primary" onclick={() => (open = false)}>{msg('common.gotIt')}</Button>
    {:else if $handoffFailed}
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button onclick={restartInstead}>{msg('ui.updateWithRestart')}</Button>
      <Button variant="primary" onclick={confirm}>{msg('common.retry')}</Button>
    {:else}
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button variant="primary" onclick={confirm}>
        <RotateCcw size="1.5rem" strokeWidth={1.8} />
        {msg('ui.restartAndUpdate')}
      </Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .modal-text {
    color: var(--text-2);
    line-height: 1.5;
  }
</style>
