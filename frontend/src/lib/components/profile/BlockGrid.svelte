<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { ArrowDown, ArrowUp, GripVertical, Settings2, Trash2 } from '@lucide/svelte';
  import Card from '../Card.svelte';
  import IconButton from '../IconButton.svelte';
  import SegmentedControl from '../SegmentedControl.svelte';
  import { msg } from '../../i18n';
  import type { BlockWidth } from '../../services/account';
  import type { GridBlock } from '../../profile/layoutView';
  import AboutBlock from './AboutBlock.svelte';
  import CollectionBlock from './CollectionBlock.svelte';
  import FingerprintBlock from './FingerprintBlock.svelte';
  import GenresBlock from './GenresBlock.svelte';
  import PinnedBlock from './PinnedBlock.svelte';
  import TextBlock from './TextBlock.svelte';
  import { blockLabel } from './blockLabels';
  import { pickTarget, type Box } from './reorder';

  let {
    blocks,
    external,
    flag,
    accent,
    editing = false,
    selectedId = '',
    onselect,
    onmove,
    onwidth,
    onremove,
  }: {
    blocks: GridBlock[];
    external?: Snippet<[GridBlock]>;
    flag?: Snippet;
    accent?: string;
    editing?: boolean;
    selectedId?: string;
    onselect?: (id: string) => void;
    onmove?: (from: number, to: number) => void;
    onwidth?: (id: string, width: BlockWidth) => void;
    onremove?: (id: string) => void;
  } = $props();

  const WIDTHS = [
    { id: 'full', label: msg('profile.widthFull') },
    { id: 'half', label: msg('profile.widthHalf') },
    { id: 'third', label: msg('profile.widthThird') },
  ];
  const WIDTH_SHORT: Record<string, string> = { full: '1', half: '1/2', third: '1/3' };

  let grid = $state<HTMLElement>();
  let dragFrom = $state<number | null>(null);
  let dragOver = $state<number | null>(null);

  function showable(block: GridBlock): boolean {
    const body = block.body;
    switch (body.kind) {
      case 'unknown':
        return false;
      case 'external':
        return !body.empty;
      case 'pinned':
        return body.game !== null;
      case 'collection':
        return body.games.length > 0;
      case 'text':
        return body.body.trim() !== '';
      default:
        return true;
    }
  }

  const shown = $derived(blocks.filter((block) => block.body.kind !== 'unknown' && (editing || showable(block))));

  function boxes(): Box[] {
    if (!grid) return [];
    return [...grid.querySelectorAll<HTMLElement>('[data-index]')].map((el) => {
      const rect = el.getBoundingClientRect();
      return { index: Number(el.dataset.index), left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom };
    });
  }

  async function commit(from: number, to: number, id: string) {
    onmove?.(from, to);
    await tick();
    grid?.querySelector<HTMLElement>(`[data-handle="${id}"]`)?.focus();
  }

  function grab(event: PointerEvent, block: GridBlock) {
    if (event.button !== 0) return;
    event.preventDefault();
    dragFrom = block.index;
    dragOver = block.index;
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  }

  function track(event: PointerEvent) {
    if (dragFrom === null) return;
    dragOver = pickTarget(event.clientX, event.clientY, boxes());
  }

  function drop(block: GridBlock) {
    const from = dragFrom;
    const to = dragOver;
    dragFrom = null;
    dragOver = null;
    if (from !== null && to !== null && from !== to) void commit(from, to, block.id);
  }

  function release() {
    dragFrom = null;
    dragOver = null;
  }

  function shift(block: GridBlock, delta: number) {
    const position = shown.findIndex((item) => item.id === block.id);
    const other = shown[position + delta];
    if (other) void commit(block.index, other.index, block.id);
  }

  function key(event: KeyboardEvent, block: GridBlock) {
    if (event.key === 'Escape') {
      release();
      return;
    }
    const delta = event.key === 'ArrowUp' || event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowDown' || event.key === 'ArrowRight' ? 1 : 0;
    if (delta === 0) return;
    event.preventDefault();
    shift(block, delta);
  }
</script>

{#snippet placeholder(block: GridBlock, hint: string)}
  <div class="placeholder">
    <span class="ph-title">{blockLabel(block.type)}</span>
    <span class="ph-hint">{hint}</span>
  </div>
{/snippet}

{#snippet content(block: GridBlock)}
  {@const body = block.body}
  {#if body.kind === 'error'}
    <Card title={blockLabel(block.type)}>
      <p class="failed">{msg('profile.blockLoadFailed')}</p>
    </Card>
  {:else if body.kind === 'pinned'}
    {#if body.game}
      <PinnedBlock game={body.game} caption={body.caption} />
    {:else}
      {@render placeholder(block, msg('profile.blockChooseGame'))}
    {/if}
  {:else if body.kind === 'collection'}
    <CollectionBlock title={body.title} games={body.games} dated={body.dated} hearts={body.hearts} />
  {:else if body.kind === 'genres'}
    <GenresBlock breakdown={body.breakdown} />
  {:else if body.kind === 'fingerprint'}
    <FingerprintBlock genres={body.breakdown.genres} hours={body.hours} games={body.games} completed={body.completed} {accent} />
  {:else if body.kind === 'text'}
    {#if body.body.trim() === ''}
      {@render placeholder(block, msg('profile.blockEmptyText'))}
    {:else}
      <TextBlock title={body.title} body={body.body} />
    {/if}
  {:else if body.kind === 'external'}
    {#if body.empty}
      {@render placeholder(block, msg('profile.blockNoData'))}
    {:else if external}
      {@render external(block)}
    {/if}
  {/if}
{/snippet}

<div class="grid" class:editing bind:this={grid}>
  {#each shown as block (block.id)}
    <div
      class="cell w-{block.width}"
      class:dragging={dragFrom === block.index}
      class:over={dragFrom !== null && dragOver === block.index && dragFrom !== block.index}
      data-index={block.index}
    >
      {#if editing}
        {@const name = blockLabel(block.type)}
        <div class="frame" class:selected={selectedId === block.id} class:issue={block.issue}>
          <div class="bar">
            <button
              class="handle"
              type="button"
              data-handle={block.id}
              aria-label={msg('profile.dragBlock', { name })}
              title={msg('profile.dragBlock', { name })}
              onpointerdown={(event) => grab(event, block)}
              onpointermove={track}
              onpointerup={() => drop(block)}
              onpointercancel={release}
              onkeydown={(event) => key(event, block)}
            >
              <GripVertical size="1.6rem" strokeWidth={1.8} />
            </button>
            <span class="name">{name}</span>
            {#if block.issue}<span class="issue-tag">{msg('profile.blockIncomplete')}</span>{/if}
            {#if block.hidden && flag}{@render flag()}{/if}
            <span class="spacer"></span>
            <SegmentedControl
              options={WIDTHS}
              bind:value={() => block.width, (value) => onwidth?.(block.id, value as BlockWidth)}
            >
              {#snippet item(option)}{WIDTH_SHORT[option.id]}{/snippet}
            </SegmentedControl>
            <IconButton size="sm" label={msg('social.moveUp', { title: name })} onclick={() => shift(block, -1)}>
              <ArrowUp size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton size="sm" label={msg('social.moveDown', { title: name })} onclick={() => shift(block, 1)}>
              <ArrowDown size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton size="sm" label={msg('profile.blockSettings', { name })} active={selectedId === block.id} onclick={() => onselect?.(block.id)}>
              <Settings2 size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton size="sm" label={msg('profile.removeBlock', { name })} onclick={() => onremove?.(block.id)}>
              <Trash2 size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          </div>
          <div class="body" role="presentation" onclick={() => onselect?.(block.id)}>
            {@render content(block)}
          </div>
        </div>
      {:else}
        {@render content(block)}
        {#if block.hidden && flag && block.body.kind !== 'external'}
          <span class="flag">{@render flag()}</span>
        {/if}
      {/if}
    </div>
  {/each}
</div>

<style>
  .grid {
    container-type: inline-size;
    display: grid;
    grid-template-columns: repeat(6, minmax(0, 1fr));
    gap: var(--space-6);
    align-items: stretch;
  }

  .grid.editing {
    gap: var(--space-4);
  }

  .cell {
    position: relative;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .cell > :global(*:not(.flag)) {
    flex: 1;
  }

  .w-full {
    grid-column: span 6;
  }

  .w-half {
    grid-column: span 3;
  }

  .w-third {
    grid-column: span 2;
  }

  @container (max-width: 640px) {
    .cell {
      grid-column: span 6;
    }
  }

  .flag {
    position: absolute;
    top: var(--space-3);
    right: var(--space-3);
  }

  .failed {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .placeholder {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 0.4rem;
    min-height: 9rem;
    padding: var(--space-5) var(--space-6);
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-lg);
  }

  .ph-title {
    font-size: var(--font-md);
    font-weight: 600;
  }

  .ph-hint {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .frame {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
    gap: var(--space-2);
    padding: var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--hover);
    transition: border-color var(--dur) var(--ease);
  }

  .frame.selected {
    border-color: var(--accent);
  }

  .frame.issue:not(.selected) {
    border-color: var(--warning);
  }

  .cell.dragging {
    opacity: 0.45;
  }

  .cell.over .frame {
    outline: 2px dashed var(--accent);
    outline-offset: 2px;
  }

  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1);
  }

  .handle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--control-sm);
    height: var(--control-sm);
    border-radius: var(--radius-sm);
    color: var(--text-3);
    cursor: grab;
    touch-action: none;
  }

  .handle:hover,
  .handle:focus-visible {
    background: var(--hover-strong);
    color: var(--text);
  }

  .handle:active {
    cursor: grabbing;
  }

  .name {
    font-size: var(--font-xs);
    font-weight: 600;
    color: var(--text-2);
  }

  .issue-tag {
    font-size: var(--font-xs);
    color: var(--warning);
  }

  .spacer {
    flex: 1;
  }

  .body {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    cursor: pointer;
  }

  .body > :global(*) {
    flex: 1;
  }

  .body :global(*) {
    pointer-events: none;
  }
</style>
