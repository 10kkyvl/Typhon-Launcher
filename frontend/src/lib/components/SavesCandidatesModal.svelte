<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { truncateMiddle } from '../utils/format';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    title,
    candidates,
    onpick,
    onbrowse,
  }: {
    open?: boolean;
    title: string;
    candidates: string[];
    onpick: (dir: string) => void;
    onbrowse: () => void;
  } = $props();
</script>

<Modal bind:open title={msg('ui.savesDirTitle')} width="52rem">
  <p class="hint">{msg('ui.savesMultipleCandidates', { title })}</p>
  <div class="candidates">
    {#each candidates as candidate (candidate)}
      <button class="candidate" onclick={() => onpick(candidate)} title={candidate}>
        {truncateMiddle(candidate, 70)}
      </button>
    {/each}
  </div>
  {#snippet footer()}
    <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" onclick={onbrowse}>
      {msg('ui.pickAnotherFolder')}
    </Button>
  {/snippet}
</Modal>

<style>
  .hint {
    margin-bottom: var(--space-4);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .candidates {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .candidate {
    padding: 0.9rem 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    font-size: var(--font-sm);
    color: var(--text-2);
    text-align: left;
    transition:
      background var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .candidate:hover {
    background: var(--hover-strong);
    border-color: var(--border-strong);
    color: var(--text);
  }
</style>
