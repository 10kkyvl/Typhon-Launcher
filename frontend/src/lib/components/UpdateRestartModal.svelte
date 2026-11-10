<script lang="ts">
  import { RotateCcw } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { requestApply, selfUpdateStatus } from '../stores/selfupdate';
  import { msg } from '../i18n';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  function confirmApply() {
    open = false;
    requestApply();
  }
</script>

<Modal bind:open title={msg('ui.restartLauncherTitle')}>
  <p class="modal-text">
    {msg('ui.restartLauncherWarning', { version: $selfUpdateStatus.availableVersion ?? '' })}
  </p>
  {#snippet footer()}
    <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" onclick={confirmApply}>
      <RotateCcw size="1.5rem" strokeWidth={1.8} />
      {msg('ui.restartAndUpdate')}
    </Button>
  {/snippet}
</Modal>

<style>
  .modal-text {
    color: var(--text-2);
    line-height: 1.5;
  }
</style>
