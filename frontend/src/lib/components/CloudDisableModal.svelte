<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { msg } from '../i18n';

  /**
   * The question before cloud saves are turned off: keep the copies in the cloud, or delete them.
   * Delete is destructive: the focus opens on Cancel, never on it. A press sends its own callback once;
   * while it runs, and while the caller says it is `busy`, the pressed button is busy and the others are
   * held with the reason, and the dialog cannot be dismissed. The caller closes the dialog once cloud
   * saves are off: an answer that leaves this opening open turned nothing off and deleted nothing, and
   * the dialog says so.
   */
  let {
    open = $bindable(false),
    busy = false,
    ondelete,
    onkeep,
  }: {
    open?: boolean;
    busy?: boolean;
    ondelete: () => Promise<unknown>;
    onkeep: () => Promise<unknown>;
  } = $props();

  const uid = $props.id();
  const failureId = `${uid}-failure`;

  // The choice being applied: that button stays focused and busy, the others wait.
  let chosen = $state<'keep' | 'delete' | ''>('');
  let failed = $state<'keep' | 'delete' | ''>('');
  // Each opening: an answer to an earlier one paints nothing.
  let opening = 0;
  $effect(() => {
    if (!open) return;
    untrack(() => {
      opening += 1;
      chosen = '';
      failed = '';
    });
  });

  const held = $derived(busy || chosen !== '');
  const reason = $derived(held ? msg('cloudsaves.disableBusy') : '');

  async function choose(choice: 'keep' | 'delete') {
    if (held) return;
    const mine = opening;
    chosen = choice;
    failed = '';
    try {
      await (choice === 'delete' ? ondelete() : onkeep());
    } catch (err) {
      console.error('cloud saves disable', err);
    }
    if (mine !== opening) return;
    chosen = '';
    if (open) failed = choice;
  }

  const pressed = (choice: 'keep' | 'delete') => chosen === choice;
</script>

<Modal dismissible={!held} bind:open busy={held} title={msg('cloudsaves.disableTitle')} width="44rem">
  <p class="text">{msg('cloudsaves.disableText')}</p>
  <p class="hint">{msg('cloudsaves.disableKeepHint')}</p>
  {#if failed}
    <div class="notice"><Notice tone="danger" compact id={failureId}>{msg('cloudsaves.disableFailed')}</Notice></div>
  {/if}

  {#snippet footer()}
    <Button data-autofocus disabled={held} {reason} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button
      disabled={held && !pressed('keep')}
      reason={pressed('keep') ? '' : reason}
      busy={pressed('keep')}
      aria-describedby={failed === 'keep' ? failureId : undefined}
      onclick={() => choose('keep')}
    >
      {msg('cloudsaves.disableKeep')}
    </Button>
    <Button
      variant="danger"
      disabled={held && !pressed('delete')}
      reason={pressed('delete') ? '' : reason}
      busy={pressed('delete')}
      aria-describedby={failed === 'delete' ? failureId : undefined}
      onclick={() => choose('delete')}
    >
      {pressed('delete') ? msg('cloudsaves.disableDeleting') : msg('cloudsaves.disableDelete')}
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

  .notice {
    margin-top: var(--space-3);
  }
</style>
