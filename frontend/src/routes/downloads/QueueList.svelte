<script lang="ts">
  import { Gamepad2, Menu, X } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { gamePageId } from '../../lib/services/downloads';
  import { downloadNow, forceStart, moveDown, moveUp, queue, remove } from '../../lib/stores/downloads';
  import { gameArt } from '../../lib/stores/metadata';
  import { playThrottle } from '../../lib/stores/playThrottle';
  import { navigate } from '../../lib/stores/router';
  import { sources } from '../../lib/stores/sources';
  import { msg } from '../../lib/i18n';
  import DownloadsSection from './DownloadsSection.svelte';
  import { coverOf, sourceTag, typeTag } from './downloadRow';

  const scheduleHeld = $derived($playThrottle.schedule?.blocked === true);
</script>

{#if $queue.length > 0}
<DownloadsSection title={msg('transfers.downloadsQueueHeading')} count={$queue.length}>
  <div class="rows">
    {#each $queue as q, i (q.id)}
      <div class="row">
        <div class="thumb" aria-hidden="true">
          <Artwork src={coverOf(q, $gameArt)} alt="" label={q.name} ratio="3 / 4" radius="var(--radius-sm)" />
        </div>
        <div class="info inline">
          <span class="title wraps">{q.name}</span>
          {#if typeTag(q) || sourceTag(q, $sources) || q.ignoreSchedule}
            <div class="tags">
              {#if typeTag(q)}<StatusBadge kind="neutral" label={typeTag(q)} dot={false} />{/if}
              {#if q.origin.platform}<ConsoleTag platform={q.origin.platform} />{/if}
              {#if sourceTag(q, $sources)}<StatusBadge kind="neutral" label={sourceTag(q, $sources)} dot={false} />{/if}
              {#if q.ignoreSchedule}<StatusBadge kind="neutral" label={msg('transfers.outsideSchedule')} dot={false} />{/if}
            </div>
          {/if}
        </div>
        <div class="status">
          <span class="status-main">{msg('transfers.downloadsStatusWaiting')}</span>
        </div>
        <div class="row-actions">
          {#if scheduleHeld && !q.ignoreSchedule}
            <Button size="sm" onclick={() => downloadNow(q.id)}>{msg('transfers.downloadNow')}</Button>
          {/if}
          {#if gamePageId(q)}
            <IconButton label={msg('transfers.gamePage')} size="sm" onclick={() => navigate('game', { id: gamePageId(q) })}>
              <Gamepad2 size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          {/if}
          <DropdownMenu
            items={[
              ...(i > 0 ? [{ id: 'up', label: msg('transfers.downloadsMoveUp') }] : []),
              ...(i < $queue.length - 1 ? [{ id: 'down', label: msg('transfers.downloadsMoveDown') }] : []),
              { id: 'start', label: msg('transfers.downloadsStartNow'), separator: i > 0 || i < $queue.length - 1 },
            ]}
            onselect={(id) => {
              if (id === 'up') moveUp(q.id);
              else if (id === 'down') moveDown(q.id);
              else forceStart(q.id);
            }}
          >
            {#snippet trigger({ props })}
              <IconButton {...props} label={msg('transfers.downloadsQueueActionsFor', { name: q.name })} size="sm">
                <Menu size="1.6rem" strokeWidth={1.8} />
              </IconButton>
            {/snippet}
          </DropdownMenu>
          <IconButton label={msg('transfers.downloadsCancelLabel')} size="sm" onclick={() => remove(q.id)}>
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

  .info.inline {
    flex-direction: row;
    align-items: center;
    gap: var(--space-3);
  }

  .tags {
    flex-shrink: 0;
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

  .tags {
    display: flex;
    gap: 0.6rem;
  }

  .status {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    flex-shrink: 0;
    width: 17rem;
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
  }

  .status-main {
    color: var(--text-2);
  }

  .row-actions {
    display: flex;
    gap: 0.4rem;
    flex-shrink: 0;
  }

  @media (max-width: 1300px) {
    .status {
      display: none;
    }
  }
</style>
