<script lang="ts">
  import { ArrowLeft, ArrowRight, ChevronDown, ChevronUp, Eye, EyeOff, GripVertical, Plus, X } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { msg } from '../../lib/i18n';
  import { SEPARATOR, type ThemeLayout } from '../../lib/theme/layout';
  import {
    addSeparator,
    canAddSeparator,
    dropOn,
    fromState,
    isHidden,
    moveToColumn,
    removeSeparator,
    step,
    toggleHidden,
    toState,
    type EditState,
    type ListKey,
  } from './layoutEdit';

  let { layout = $bindable() }: { layout?: ThemeLayout } = $props();

  let st = $state<EditState>(toState(layout));
  let emitted: ThemeLayout | undefined = layout;
  let dragging = $state<{ key: ListKey; index: number } | null>(null);
  let over = $state<string | null>(null);

  $effect(() => {
    if (layout === emitted) return;
    emitted = layout;
    st = toState(layout);
  });

  function apply(next: EditState) {
    st = next;
    emitted = fromState(next);
    layout = emitted;
  }

  function resetLayout() {
    apply(toState(undefined));
  }

  const sidebarLabels: Record<string, string> = {
    library: 'ui.library',
    catalog: 'ui.allGames',
    installed: 'ui.installed',
    diskusage: 'diskusage.nav',
    downloads: 'ui.downloadsNav',
    sources: 'ui.sourcesNav',
    friends: 'ui.friends',
    activity: 'ui.activity',
    stats: 'stats.nav',
    history: 'ui.history',
    lan: 'ui.localNetwork',
  };
  const topbarLabels: Record<string, string> = {
    bigpicture: 'bp.enter',
    notifications: 'ui.notifications',
    friends: 'ui.friends',
  };

  function label(key: ListKey, id: string): string {
    if (id === SEPARATOR) return msg('settings.layoutSeparator');
    if (key === 'sidebar') return msg(sidebarLabels[id] as never);
    if (key === 'topbar') return msg(topbarLabels[id] as never);
    return msg(`settings.layoutSection.${id}` as never);
  }

  const scopeOf = (key: ListKey) => (key === 'sidebar' ? 'sidebar' : key === 'topbar' ? 'topbar' : 'game');

  const sideOptions = $derived([
    { id: 'left', label: msg('settings.layoutSidebarLeft') },
    { id: 'right', label: msg('settings.layoutSidebarRight') },
  ]);
  const searchOptions = $derived([
    { id: 'left', label: msg('settings.layoutSearchLeft') },
    { id: 'center', label: msg('settings.layoutSearchCenter') },
  ]);

  function onDragStart(e: DragEvent, key: ListKey, index: number) {
    dragging = { key, index };
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', `${key}:${index}`);
    }
  }

  function onDragOver(e: DragEvent, key: ListKey, index?: number) {
    if (!dragging) return;
    const game = (k: ListKey) => k === 'main' || k === 'side';
    if (dragging.key !== key && !(game(dragging.key) && game(key))) return;
    e.preventDefault();
    over = `${key}:${index ?? 'end'}`;
  }

  function onDrop(e: DragEvent, key: ListKey, index?: number) {
    e.preventDefault();
    e.stopPropagation();
    if (dragging) apply(dropOn(st, dragging, { key, index }));
    dragging = null;
    over = null;
  }

  function onDragEnd() {
    dragging = null;
    over = null;
  }
</script>

{#snippet rows(key: ListKey, title: string)}
  {@const list = st[key]}
  {@const scope = scopeOf(key)}
  <div class="list-block">
    {#if title}
      <span class="list-title">{title}</span>
    {/if}
    <ul
      class="list"
      class:drop-end={over === `${key}:end`}
      aria-label={title || undefined}
      ondragover={(e) => onDragOver(e, key)}
      ondrop={(e) => onDrop(e, key)}
    >
      {#each list as id, index (`${id}-${index}`)}
        {@const separator = id === SEPARATOR}
        {@const hidden = !separator && isHidden(st, scope, id)}
        <li
          class="item"
          class:hidden
          class:separator
          class:dragging={dragging?.key === key && dragging.index === index}
          class:drop-target={over === `${key}:${index}`}
          draggable="true"
          ondragstart={(e) => onDragStart(e, key, index)}
          ondragover={(e) => onDragOver(e, key, index)}
          ondrop={(e) => onDrop(e, key, index)}
          ondragend={onDragEnd}
        >
          <span class="grip" aria-hidden="true"><GripVertical size={14} /></span>
          <span class="item-name">{label(key, id)}</span>
          <div class="item-actions">
            <button
              type="button"
              class="mini"
              aria-label={msg('settings.layoutMoveUp')}
              title={msg('settings.layoutMoveUp')}
              disabled={index === 0}
              onclick={() => apply(step(st, key, index, -1))}
            >
              <ChevronUp size={14} />
            </button>
            <button
              type="button"
              class="mini"
              aria-label={msg('settings.layoutMoveDown')}
              title={msg('settings.layoutMoveDown')}
              disabled={index === list.length - 1}
              onclick={() => apply(step(st, key, index, 1))}
            >
              <ChevronDown size={14} />
            </button>
            {#if key === 'main' || key === 'side'}
              {@const other = key === 'main' ? 'side' : 'main'}
              {@const moveLabel = key === 'main' ? msg('settings.layoutToSide') : msg('settings.layoutToMain')}
              <button
                type="button"
                class="mini"
                aria-label={moveLabel}
                title={moveLabel}
                onclick={() => apply(moveToColumn(st, key, index, other))}
              >
                {#if key === 'main'}<ArrowRight size={14} />{:else}<ArrowLeft size={14} />{/if}
              </button>
            {/if}
            {#if separator}
              <button
                type="button"
                class="mini"
                aria-label={msg('settings.layoutRemoveSeparator')}
                title={msg('settings.layoutRemoveSeparator')}
                onclick={() => apply(removeSeparator(st, index))}
              >
                <X size={14} />
              </button>
            {:else}
              {@const eyeLabel = hidden ? msg('settings.layoutShow') : msg('settings.layoutHide')}
              <button
                type="button"
                class="mini"
                aria-label={eyeLabel}
                aria-pressed={hidden}
                title={eyeLabel}
                onclick={() => apply(toggleHidden(st, scope, id))}
              >
                {#if hidden}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
              </button>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  </div>
{/snippet}

<div class="layout-editor">
  <div class="head">
    <span class="row-label">{msg('settings.layoutTitle')}</span>
    <Button size="sm" variant="secondary" disabled={layout === undefined} onclick={resetLayout}>
      {msg('settings.layoutReset')}
    </Button>
  </div>
  <p class="hint">{msg('settings.layoutIntro')}</p>

  <details class="group" open>
    <summary>{msg('settings.layoutSidebarTitle')}</summary>
    <div class="group-body">
      <SegmentedControl
        options={sideOptions}
        bind:value={() => st.sidebarSide, (v) => apply({ ...st, sidebarSide: v === 'right' ? 'right' : 'left' })}
      />
      {@render rows('sidebar', '')}
      <div>
        <Button size="sm" variant="secondary" disabled={!canAddSeparator(st)} onclick={() => apply(addSeparator(st))}>
          <Plus size={14} />{msg('settings.layoutAddSeparator')}
        </Button>
      </div>
      <div class="support-row">
        <span class="item-name">{msg('tickets.nav')}</span>
        <button
          type="button"
          class="mini"
          aria-label={isHidden(st, 'sidebar', 'support') ? msg('settings.layoutShow') : msg('settings.layoutHide')}
          title={isHidden(st, 'sidebar', 'support') ? msg('settings.layoutShow') : msg('settings.layoutHide')}
          aria-pressed={isHidden(st, 'sidebar', 'support')}
          onclick={() => apply(toggleHidden(st, 'sidebar', 'support'))}
        >
          {#if isHidden(st, 'sidebar', 'support')}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
        </button>
      </div>
    </div>
  </details>

  <details class="group" open>
    <summary>{msg('settings.layoutTopbarTitle')}</summary>
    <div class="group-body">
      <SegmentedControl
        options={searchOptions}
        bind:value={() => st.search, (v) => apply({ ...st, search: v === 'center' ? 'center' : 'left' })}
      />
      {@render rows('topbar', '')}
    </div>
  </details>

  <details class="group" open>
    <summary>{msg('settings.layoutGameTitle')}</summary>
    <div class="group-body game-cols">
      {@render rows('main', msg('settings.layoutMainColumn'))}
      {@render rows('side', msg('settings.layoutSideColumn'))}
    </div>
  </details>
</div>

<style>
  .layout-editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .row-label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .group {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .group > summary {
    padding: var(--space-3) var(--space-4);
    font-size: var(--font-sm);
    font-weight: 600;
    cursor: pointer;
  }

  .group > summary:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .group-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: 0 var(--space-4) var(--space-4);
  }

  .game-cols {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
    align-items: start;
  }

  .list-block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }

  .list-title {
    font-size: var(--font-xs);
    font-weight: 600;
    color: var(--text-2);
  }

  .list {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-height: 2.4rem;
    padding: 0;
    border-radius: var(--radius-md);
  }

  .list.drop-end {
    outline: 2px dashed var(--accent);
    outline-offset: 2px;
  }

  .item,
  .support-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
  }

  .item.dragging {
    opacity: 0.4;
  }

  .item.drop-target {
    border-color: var(--accent);
  }

  .item.hidden .item-name {
    color: var(--text-3);
    text-decoration: line-through;
  }

  .item.separator .item-name {
    color: var(--text-3);
    font-style: italic;
  }

  .grip {
    display: inline-flex;
    color: var(--text-3);
    cursor: grab;
  }

  .item-name {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--font-sm);
  }

  .item-actions {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
  }

  .mini {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--control-sm);
    height: var(--control-sm);
    border-radius: var(--radius-sm);
    color: var(--text-2);
  }

  .mini:hover:not(:disabled) {
    background: var(--surface-3);
    color: var(--text);
  }

  .mini:disabled {
    opacity: 0.35;
  }

  .mini:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }
</style>
