<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import type { ConfirmPrompt } from '../confirm/prompts';

  let {
    prompt,
    onconfirm,
    onclose,
  }: {
    prompt: ConfirmPrompt;
    onconfirm: () => void | Promise<void>;
    onclose: () => void;
  } = $props();

  let running = $state(false);

  async function confirm() {
    if (running) return;
    running = true;
    try {
      await onconfirm();
      onclose();
    } finally {
      running = false;
    }
  }
</script>

<Modal open title={prompt.title} width="44rem" {onclose}>
  <p class="text">{prompt.text}</p>
  {#if prompt.note}
    <p class="note">{prompt.note}</p>
  {/if}
  {#snippet footer()}
    <Button disabled={running} onclick={onclose}>{prompt.cancel ?? msg('common.cancel')}</Button>
    <Button variant="danger" disabled={running} onclick={confirm}>
      {running ? (prompt.busy ?? prompt.confirm) : prompt.confirm}
    </Button>
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
</style>
