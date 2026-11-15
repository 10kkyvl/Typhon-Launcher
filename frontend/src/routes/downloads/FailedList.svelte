<script lang="ts">
  import { CircleAlert, Gamepad2, RotateCcw, X } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { gamePageId } from '../../lib/services/downloads';
  import { failed, remove, resume } from '../../lib/stores/downloads';
  import { installErrorText } from '../../lib/install/installErrors';
  import { gameArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { msg } from '../../lib/i18n';
  import DownloadsSection from './DownloadsSection.svelte';
  import { coverOf } from './downloadRow';

  let { onopen }: { onopen: (id: string) => void } = $props();
</script>

{#if $failed.length > 0}
<DownloadsSection title={msg('transfers.downloadsFailedHeading')} count={$failed.length}>
  <div class="rows">
    {#each $failed as item (item.id)}
      <div class="row failed">
        <div class="thumb" aria-hidden="true">
          <Artwork src={coverOf(item, $gameArt)} alt="" label={item.name} ratio="3 / 4" radius="var(--radius-sm)" />
        </div>
        <div class="info">
          <span class="title wraps">
            <TextLink tone="muted" onclick={() => onopen(item.id)}>{item.name}</TextLink>
          </span>
          <span class="error-text">
            <CircleAlert size="1.4rem" strokeWidth={1.8} />
            <span>{installErrorText(item.error)}</span>
          </span>
        </div>
        <div class="row-actions">
          {#if gamePageId(item)}
            <IconButton label={msg('transfers.gamePage')} size="sm" onclick={() => navigate('game', { id: gamePageId(item) })}>
              <Gamepad2 size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          {/if}
          <Button size="sm" onclick={() => resume(item.id)}>
            <RotateCcw size="1.4rem" strokeWidth={1.8} />
            {msg('common.retry')}
          </Button>
          <IconButton label={msg('transfers.downloadsRemoveFromListLabel')} size="sm" onclick={() => remove(item.id)}>
            <X size="1.6rem" strokeWidth={1.8} />
          </IconButton>
        </div>
      </div>
    {/each}
  </div>
</DownloadsSection>
{/if}

<style>
  .rows {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    min-height: 6.4rem;
    padding: var(--space-2) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    transition: border-color var(--dur) var(--ease);
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .row:hover {
    border-color: var(--border-strong);
  }

  /* After :hover, which has the same specificity: a failed row keeps its border under the pointer. */
  .row.failed {
    border-color: color-mix(in srgb, var(--danger) 35%, var(--border));
  }

  .error-text {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    max-width: 100%;
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .error-text span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .error-text :global(svg) {
    flex-shrink: 0;
  }

  .thumb {
    width: 3.6rem;
    flex-shrink: 0;
  }

  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
  }

  .title {
    min-width: 0;
    max-width: 100%;
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* A name is shown whole, wrapping in its row: an ellipsis would hide all of a text link (a button,
     an atomic box), and a hint on the rest would reach a mouse only. */
  .title.wraps {
    white-space: normal;
    overflow: visible;
    overflow-wrap: anywhere;
  }

  .row-actions {
    display: flex;
    gap: 0.4rem;
    flex-shrink: 0;
  }
</style>
