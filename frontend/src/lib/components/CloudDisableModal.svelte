<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    busy = false,
    ondelete,
    onkeep,
  }: {
    open?: boolean;
    busy?: boolean;
    ondelete: () => void;
    onkeep: () => void;
  } = $props();
</script>

<Modal bind:open title={msg('cloudsaves.disableTitle')} width="44rem">
  <p class="text">{msg('cloudsaves.disableText')}</p>
  <p class="hint">{msg('cloudsaves.disableKeepHint')}</p>

  {#snippet footer()}
    <Button disabled={busy} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button disabled={busy} onclick={onkeep}>{msg('cloudsaves.disableKeep')}</Button>
    <Button variant="danger" disabled={busy} onclick={ondelete}>
      {busy ? msg('cloudsaves.disableDeleting') : msg('cloudsaves.disableDelete')}
    </Button>
  {/snippet}
</Modal>

<style>
  .text {
    margin: 0 0 var(--space-3);
    font-size: var(--font-md);
    color: var(--text);
  }

  .hint {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }
</style>
