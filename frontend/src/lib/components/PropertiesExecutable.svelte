<script lang="ts">
  import { FolderOpen } from '@lucide/svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import Tooltip from './Tooltip.svelte';
  import { msg } from '../i18n';
  import type { Candidate } from '../services/install';
  import type { LibraryGame } from '../services/library';
  import { truncateMiddle } from '../utils/format';

  // The executable section of GamePropertiesModal, which holds its state and does its writes.
  let {
    game,
    emulated,
    candidates,
    candidatesLoading,
    candidatesError,
    exeBusy,
    exeError,
    onpick,
    onbrowse,
  }: {
    game: LibraryGame;
    emulated: boolean;
    candidates: Candidate[];
    candidatesLoading: boolean;
    candidatesError: string;
    exeBusy: '' | 'pick' | 'browse';
    exeError: string;
    onpick: (path: string) => void;
    onbrowse: () => void;
  } = $props();
</script>

<section class="section">
  <h4>{msg('modals.propertiesExeTitle')}</h4>
  <p class="path">
    {game.executable ? msg('modals.propertiesExeCurrent', { path: game.executable }) : msg('modals.propertiesExeNone')}
  </p>
  {#if emulated}
    <p class="hint">{msg('emul.imageHint')}</p>
  {:else if candidatesLoading}
    <Spinner size="sm" label={msg('modals.propertiesExeLoading')} />
  {:else if candidatesError}
    <Notice tone="danger" compact>{candidatesError}</Notice>
  {:else if candidates.length === 0}
    <EmptyState size="inline" title={msg('modals.propertiesExeNoCandidates')} />
  {:else}
    <span class="label">{msg('modals.propertiesExeCandidates')}</span>
    <div class="candidates">
      {#each candidates as candidate (candidate.path)}
        <Tooltip text={candidate.path}>
          <button
            type="button"
            class="candidate"
            aria-current={candidate.path === game.executable || undefined}
            aria-disabled={exeBusy !== '' || undefined}
            onclick={() => onpick(candidate.path)}
          >
            {truncateMiddle(candidate.path, 70)}
          </button>
        </Tooltip>
      {/each}
    </div>
  {/if}
  {#if exeError}
    <Notice tone="danger" compact announce={false}>{exeError}</Notice>
  {/if}
  <div>
    <Button size="sm" disabled={exeBusy === 'pick'} busy={exeBusy === 'browse'} onclick={onbrowse}>
      <FolderOpen size="1.5rem" strokeWidth={1.8} />
      {msg('modals.propertiesExeBrowse')}
    </Button>
  </div>
</section>

<style>
  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h4 {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text);
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .path {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .candidates {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    max-height: 18rem;
    overflow-y: auto;
  }

  .candidate {
    text-align: left;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--text-2);
    font-size: var(--font-sm);
    cursor: pointer;
  }

  .candidate:hover:not([aria-disabled='true']) {
    border-color: var(--border-strong);
    color: var(--text);
  }

  .candidate[aria-current='true'] {
    border-color: var(--accent-bright);
    color: var(--text);
  }

  .candidate[aria-disabled='true'] {
    opacity: 0.6;
    cursor: default;
  }

  /* Inside the scrolling list: the shared ring is drawn inward, where the list does not clip it. */
  .candidate:focus-visible {
    outline-offset: -2px;
  }
</style>
