<script lang="ts">
  import { ArrowUpToLine, Ellipsis, GripVertical, Play, X } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import type { MenuItem } from '../../lib/components/menu';
  import type { DownloadAction } from '../../lib/downloads/actions';
  import { gamePageId, type Download } from '../../lib/services/downloads';
  import { gameScope } from '../../lib/stores/gameTone';
  import { gameArt } from '../../lib/stores/metadata';
  import { sources } from '../../lib/stores/sources';
  import { bytesSize, progressPercent } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import { coverOf, sourceTag, typeTag } from './downloadRow';
  import { queueState } from './queueRow';

  /**
   * One queued download: the grip that reorders it (keys and pointer, wired by QueueList), its cover
   * and title to the game's page, its place and state in words, the part already downloaded, and its
   * actions. Every control is offered only when Go accepts its action (`offers`), and every name that a
   * list would repeat carries the download's title.
   */
  let {
    download,
    index,
    total,
    offers,
    waiting,
    startNow,
    details,
    helpId,
    lifted = false,
    transform,
    busyNow = false,
    busyCancel = false,
    ongripkey,
    ongrab,
    ontrack,
    ondrop,
    onletgo,
    ontop,
    onnow,
    oncancel,
    onpick,
  }: {
    download: Download;
    /** The row's place in the queue as shown, 0 first: the `position` Go's MoveTo takes. */
    index: number;
    total: number;
    offers: readonly DownloadAction[];
    /** The schedule holds this download back now. */
    waiting: boolean;
    /** "Start now" would start something (queueRow.ts::startNowHelps). */
    startNow: boolean;
    /** The screen opens the download's details: the menu offers them. */
    details: boolean;
    /** The id of the text that explains the grip's keys. */
    helpId: string;
    lifted?: boolean;
    /** The row's shift while a drag is under way. */
    transform?: string;
    busyNow?: boolean;
    busyCancel?: boolean;
    ongripkey: (event: KeyboardEvent) => void;
    ongrab: (event: PointerEvent) => void;
    ontrack: (event: PointerEvent) => void;
    ondrop: (event: PointerEvent) => void;
    onletgo: (event: PointerEvent) => void;
    ontop: () => void;
    onnow: () => void;
    oncancel: () => void;
    onpick: (id: 'details' | 'start' | 'up' | 'down') => void;
  } = $props();

  const title = $derived(download.name);
  const gameId = $derived(gamePageId(download));
  const movable = $derived(offers.includes('moveTo'));
  const tag = $derived(typeTag(download));
  const source = $derived(sourceTag(download, $sources));

  const items = $derived.by((): MenuItem[] => {
    const head: MenuItem[] = [
      ...(details ? [{ id: 'details', label: msg('downloads.queue.details') }] : []),
      ...(startNow && offers.includes('forceStart') ? [{ id: 'start', label: msg('transfers.downloadsStartNow') }] : []),
    ];
    const moves: MenuItem[] = [
      ...(movable && index > 0 ? [{ id: 'up', label: msg('transfers.downloadsMoveUp') }] : []),
      ...(movable && index < total - 1 ? [{ id: 'down', label: msg('transfers.downloadsMoveDown') }] : []),
    ];
    if (head.length > 0 && moves.length > 0) moves[0] = { ...moves[0], separator: true };
    return [...head, ...moves];
  });
</script>

<li class="row" class:first={index === 0} class:lifted class:room={transform !== undefined && !lifted} data-id={download.id} style:transform>
  {#if movable}
    <Tooltip text={msg('downloads.queue.gripHint')} describe={false}>
      <button
        type="button"
        class="grip"
        data-grip={download.id}
        aria-label={msg('downloads.queue.grip', { title, position: index + 1, total })}
        aria-describedby={helpId}
        onkeydown={ongripkey}
        onpointerdown={ongrab}
        onpointermove={ontrack}
        onpointerup={ondrop}
        onpointercancel={onletgo}
        onlostpointercapture={onletgo}
      >
        <GripVertical size="1.8rem" strokeWidth={1.8} aria-hidden="true" />
      </button>
    </Tooltip>
  {:else}
    <span class="grip-space" aria-hidden="true"></span>
  {/if}

  <div class="thumb">
    <GameLink id={gameId} name={title} repeat>
      <Artwork src={coverOf(download, $gameArt)} alt="" label={title} ratio="3 / 4" radius="var(--radius-sm)" />
    </GameLink>
  </div>

  <div class="info">
    <div class="name">
      <span class="title wraps"><GameLink id={gameId} name={title} /></span>
      {#if tag || source || download.origin.platform || download.ignoreSchedule}
        <span class="tags">
          {#if tag}<StatusBadge kind="neutral" label={tag} dot={false} />{/if}
          {#if download.origin.platform}<ConsoleTag platform={download.origin.platform} />{/if}
          {#if source}<StatusBadge kind="neutral" label={source} dot={false} />{/if}
          {#if download.ignoreSchedule}<StatusBadge kind="neutral" label={msg('transfers.outsideSchedule')} dot={false} />{/if}
        </span>
      {/if}
    </div>
    <p class="meta">
      {#if download.total > 0}
        <span>{bytesSize(download.total)}</span>
        <span aria-hidden="true">·</span>
      {/if}
      <span>{msg('downloads.queue.position', { n: index + 1 })}</span>
      <span aria-hidden="true">·</span>
      <span>{queueState(index, waiting)}</span>
      {#if download.downloaded > 0}
        <!-- The bar below says the amount; the percent here is its look, so it is said once. -->
        <span aria-hidden="true">·</span>
        <span aria-hidden="true">{msg('downloads.queue.downloaded', { percent: progressPercent(download.progress) })}</span>
      {/if}
    </p>
    {#if download.downloaded > 0}
      <div class="bar" {@attach gameScope(gameId)}>
        <ProgressBar value={download.progress} max={1} height={3} tone="game" label={msg('downloads.queue.downloadedFor', { title })} />
      </div>
    {/if}
  </div>

  <div class="actions">
    {#if offers.includes('downloadNow')}
      <Button size="sm" busy={busyNow} aria-label={msg('downloads.queue.downloadNowFor', { title })} onclick={onnow}>
        <Play size="1.4rem" strokeWidth={1.8} aria-hidden="true" />
        {msg('transfers.downloadNow')}
      </Button>
    {/if}
    {#if movable && index > 0}
      <Button size="sm" aria-label={msg('downloads.queue.toTopFor', { title })} onclick={ontop}>
        <ArrowUpToLine size="1.4rem" strokeWidth={1.8} aria-hidden="true" />
        {msg('downloads.queue.toTop')}
      </Button>
    {/if}
    {#if items.length > 0}
      <DropdownMenu {items} onselect={(id) => onpick(id as 'details' | 'start' | 'up' | 'down')}>
        {#snippet trigger({ props })}
          <IconButton {...props} label={msg('transfers.downloadsQueueActionsFor', { name: title })} size="sm" data-menu={download.id}>
            <Ellipsis size="1.6rem" strokeWidth={1.8} />
          </IconButton>
        {/snippet}
      </DropdownMenu>
    {/if}
    {#if offers.includes('cancel')}
      <IconButton label={msg('downloads.queue.cancelFor', { title })} size="sm" busy={busyCancel} data-cancel={download.id} onclick={oncancel}>
        <X size="1.6rem" strokeWidth={1.8} />
      </IconButton>
    {/if}
  </div>
</li>

<style>
  .row {
    display: grid;
    grid-template-columns: auto 3.6rem minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-2);
    border-radius: var(--radius-lg);
  }

  .row:not(.first) {
    border-top: 1px solid var(--border);
  }

  /* The others make room as the held row passes. Only during a drag: a drop lands at once, and with
     motion off or reduced the global rule makes the room instant too. */
  .row.room {
    transition: transform var(--motion-fast) var(--ease-out);
  }

  /* The held row paints over the rows it passes; local to the list, which it isolates. */
  .row.lifted {
    position: relative;
    z-index: 1;
    isolation: isolate;
    background: var(--surface-3);
    box-shadow: var(--shadow-lift);
  }

  .grip,
  .grip-space {
    width: 2.8rem;
    height: var(--control-md);
  }

  .grip {
    display: grid;
    place-items: center;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--text-2);
    cursor: grab;
    touch-action: none;
  }

  .grip:hover {
    background: var(--hover);
    color: var(--text);
  }

  .lifted .grip {
    cursor: grabbing;
  }

  .thumb {
    overflow: hidden;
    border-radius: var(--radius-sm);
  }

  .info {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .name {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    min-width: 0;
  }

  .title {
    min-width: 0;
    max-width: 100%;
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
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
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-2);
    margin: 0;
    color: var(--text-2);
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
  }

  .bar {
    max-width: 32rem;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    align-items: center;
    gap: var(--space-2);
  }

  @media (max-width: 760px) {
    .actions {
      grid-column: 3 / -1;
      justify-content: flex-start;
    }
  }

  @media (pointer: coarse) {
    .grip,
    .grip-space {
      width: var(--control-lg);
      height: var(--control-lg);
    }
  }
</style>
