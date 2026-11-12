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

  // The choice the caller is applying while `busy`: that button stays focused and busy, the other waits.
  let chosen = $state<'keep' | 'delete'>('delete');
</script>

<Modal dismissible={!busy} bind:open title={msg('cloudsaves.disableTitle')} width="44rem">
  <p class="text">{msg('cloudsaves.disableText')}</p>
  <p class="hint">{msg('cloudsaves.disableKeepHint')}</p>

  {#snippet footer()}
    <Button disabled={busy} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button
      disabled={busy && chosen !== 'keep'}
      busy={busy && chosen === 'keep'}
      onclick={() => {
        chosen = 'keep';
        onkeep();
      }}
    >
      {msg('cloudsaves.disableKeep')}
    </Button>
    <Button
      variant="danger"
      disabled={busy && chosen !== 'delete'}
      busy={busy && chosen === 'delete'}
      onclick={() => {
        chosen = 'delete';
        ondelete();
      }}
    >
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
