<script lang="ts">
  import { tick } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { codeText, msg } from '../i18n';
  import type { ConfirmPrompt } from '../confirm/prompts';

  let {
    prompt,
    onconfirm,
    onclose,
    errorText = (err: unknown) => codeText(err),
    returnFocus,
  }: {
    prompt: ConfirmPrompt;
    /** The action. While it runs the dialog cannot be dismissed; when it rejects, the dialog stays open with the error. */
    onconfirm: () => void | Promise<void>;
    /** The dialog is done: the action resolved, or the user cancelled. */
    onclose: () => void;
    /**
     * Text for a failed action, from the domain's error table. Pass it whenever the action may reject
     * (ConfirmModal.callers.test.ts); the default only maps a backend code that has a message, else "error".
     */
    errorText?: (err: unknown) => string;
    /** Where focus goes on close when the element that opened the dialog is gone (a menu item). */
    returnFocus?: () => HTMLElement | null | undefined;
  } = $props();

  const uid = $props.id();
  const textId = `${uid}-text`;
  const destructive = $derived((prompt.tone ?? 'danger') === 'danger');

  let running = $state(false);
  let error = $state('');
  let confirmSlot = $state<HTMLElement>();

  async function confirm() {
    if (running) return;
    running = true;
    error = '';
    try {
      await onconfirm();
    } catch (err) {
      console.error('confirm action', err);
      error = errorText(err);
      running = false;
      await tick();
      confirmSlot?.querySelector('button')?.focus();
      return;
    }
    running = false;
    onclose();
  }

  function cancel() {
    if (!running) onclose();
  }
</script>

<Modal open title={prompt.title} width="44rem" dismissible={!running} busy={running} describedby={textId} {returnFocus} onclose={cancel}>
  <p class="text" id={textId}>{prompt.text}</p>
  {#if prompt.note}
    <p class="note">{prompt.note}</p>
  {/if}
  <div role="alert">
    {#if error}<p class="error">{error}</p>{/if}
  </div>
  {#snippet footer()}
    <span class="action" data-autofocus={destructive ? '' : undefined}>
      <Button disabled={running} onclick={cancel}>{prompt.cancel ?? msg('common.cancel')}</Button>
    </span>
    <span class="action" data-autofocus={destructive ? undefined : ''} bind:this={confirmSlot}>
      <Button variant={prompt.tone ?? 'danger'} disabled={running} onclick={confirm}>
        {running ? (prompt.busy ?? prompt.confirm) : error ? msg('common.retry') : prompt.confirm}
      </Button>
    </span>
  {/snippet}
</Modal>

<style>
  .text {
    color: var(--text-2);
    line-height: 1.55;
  }

  .note {
    margin: var(--space-4) 0 0;
    color: var(--warning);
    line-height: 1.55;
  }

  .error {
    margin: var(--space-4) 0 0;
    padding: var(--space-2) var(--space-3);
    border-left: 2px solid var(--danger);
    border-radius: var(--radius-sm);
    background: var(--danger-subtle);
    color: var(--text);
    font-size: var(--font-sm);
    line-height: 1.5;
    user-select: text;
  }

  .action {
    display: contents;
  }
</style>
