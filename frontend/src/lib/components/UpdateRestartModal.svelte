<script lang="ts">
  import { RotateCcw } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { handoffFailed, requestApply, requestHandoff, selfUpdateApplying, selfUpdateStatus } from '../stores/selfupdate';
  import { updateShow } from '../stores/updateShow';
  import { msg } from '../i18n';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  // This dialog's confirmation started a seamless restart: if Go rolls it back, the choice comes back
  // here once the show has gone home, so the player answers it where they asked.
  let waiting = $state(false);

  $effect(() => {
    if (!waiting || $selfUpdateApplying || $updateShow) return;
    waiting = false;
    if ($handoffFailed) open = true;
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

<Modal bind:open title={msg($handoffFailed ? 'ui.handoffFailedTitle' : 'ui.restartLauncherTitle')}>
  <p class="modal-text">
    {$handoffFailed
      ? msg('state.selfupdateHandoffFailed')
      : msg('ui.restartLauncherWarning', { version: $selfUpdateStatus.availableVersion ?? '' })}
  </p>
  {#snippet footer()}
    <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    {#if $handoffFailed}
      <Button onclick={restartInstead}>{msg('ui.updateWithRestart')}</Button>
      <Button variant="primary" onclick={confirm}>{msg('common.retry')}</Button>
    {:else}
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
