<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { on } from 'svelte/events';
  import { Heart, Play, Square, type LucideIcon } from '@lucide/svelte';
  import { gameContextMenu } from '../stores/gameMenu';
  import type { MenuTriggerProps } from './menu';
  import { navigate } from '../stores/router';
  import { msg } from '../i18n';
  import Tooltip from './Tooltip.svelte';
  import Artwork from './Artwork.svelte';
  import type { CompatInfo } from '../services/sources';
  import { compatBadge } from '../game/compat';
  import type { GameSource } from '../library/source';
  import SourceBadge from './SourceBadge.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import ConsoleTag from './ConsoleTag.svelte';
  import Spinner from './Spinner.svelte';
  import { refuseWhile } from './refuse';
  import { ambientHover } from '../utils/ambientHover';

  interface GameCardActionBase {
    id: string;
    label: string;
    icon: LucideIcon;
    active?: boolean;
    filled?: boolean;
    pinned?: boolean;
    quiet?: boolean;
    /** Its own request runs: it says it is busy (aria-busy), ignores a press and keeps focus. */
    busy?: boolean;
    onclick?: (event: MouseEvent) => void;
    /** The action opens a menu: its button is that menu's button (`gameMenuButton`), instead of `onclick`. */
    menu?: MenuTriggerProps;
  }

  /**
   * `disabled`: it cannot act now. It says so (aria-disabled) and why (`reason`, its hint and
   * description), ignores a press and keeps focus and its place; a caller cannot pass the flag
   * without the reason. A request that runs is `busy`, not disabled.
   */
  export type GameCardAction = GameCardActionBase & ({ disabled?: false; reason?: undefined } | { disabled?: boolean; reason: string });

  let {
    id,
    title,
    cover = '',
    coverFallback = '',
    artSeed,
    installed = false,
    running = false,
    launching = false,
    favorite = false,
    meta,
    compat,
    source = null,
    platform = '',
    variant = 'poster',
    footer,
    actions,
    onplay,
    ambient = false,
  }: {
    id: string;
    title: string;
    cover?: string;
    /** The game's own cover, tried when `cover` (from metadata) fails to load. */
    coverFallback?: string;
    /** Seeds the generated art when there is no cover: the game's canonical id, so every screen draws the same picture. Default: `id`. */
    artSeed?: string;
    installed?: boolean;
    running?: boolean;
    /** A launch of this game waits on Go: Play shows busy and a second press sends nothing. */
    launching?: boolean;
    /** Shows the favourite mark. */
    favorite?: boolean;
    meta?: string;
    compat?: CompatInfo;
    source?: GameSource | null;
    platform?: string;
    variant?: 'poster' | 'capsule';
    footer?: Snippet;
    actions?: GameCardAction[];
    onplay?: () => void;
    /** The window backdrop follows this card's game while the pointer rests on it (library only). */
    ambient?: boolean;
  } = $props();

  const uid = $props.id();
  const playable = $derived(installed && Boolean(onplay));

  let popped = $state('');

  const ratio = $derived(variant === 'capsule' ? '16 / 9' : '3 / 4');

  // Бейджа нет, пока сервер не отдал строку: он отдаёт её только выше порога
  // наблюдений, и молчание тут честнее любой цифры.
  const badge = $derived(compatBadge(compat));
  const compatWorks = $derived(badge?.works === true);
  const compatLabel = $derived(
    badge
      ? msg(badge.works ? 'games.compatBadgeWorks' : 'games.compatBadgeBroken', {
          works: String(badge.works_count),
          total: String(badge.total),
        })
      : '',
  );

  const describedBy = $derived([compatLabel && `${uid}-compat`, favorite && `${uid}-fav`].filter(Boolean).join(' ') || undefined);

  // Ctrl+Enter (Cmd+Enter on a Mac) on any control of the card launches the game, as Play does; Enter
  // stays each control's own. Arrows across the grid are the page's.
  const launchKey: Attachment<HTMLElement> = (node) =>
    on(node, 'keydown', (event) => {
      if (event.key !== 'Enter' || !(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey || !playable) return;
      event.preventDefault();
      if (event.repeat || running || launching) return;
      onplay?.();
    });
</script>

<div class="card" role="presentation" {@attach gameContextMenu(id)} {@attach launchKey} use:ambientHover={ambient ? id : null}>
  <div class="cover-wrap" class:playable class:running={playable && running}>
    <!-- A pointer target only: the title below opens the same page and carries the game's name, so the
         cover is out of the Tab order and of the accessibility tree. -->
    <button class="cover" tabindex="-1" aria-hidden="true" onclick={() => navigate('game', { id })} aria-label={title}>
      <Artwork src={cover} fallback={coverFallback} alt="" label={title} seed={artSeed ?? id} {ratio} radius="var(--radius-md)" />
      <span class="fade"></span>
    </button>
    {#if platform}
      <span class="source"><ConsoleTag {platform} /></span>
    {:else if source}
      <span class="source"><SourceBadge {source} /></span>
    {/if}
    {#if installed && onplay}
      <button
        class="play"
        class:running
        aria-label={running ? msg('ui.stop') : msg('ui.play')}
        aria-describedby="{uid}-title"
        aria-busy={launching || undefined}
        onclick={onplay}
        {@attach refuseWhile(() => launching)}
      >
        {#if launching}
          <Spinner size="sm" tone="current" />
        {:else if running}
          <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
        {:else}
          <Play size="1.4rem" strokeWidth={2} fill="currentColor" />
        {/if}
      </button>
    {/if}
    <!-- After Play, so a rule can follow Play's keyboard focus to it. -->
    {#if compatLabel}
      <span class="compat" id="{uid}-compat"><StatusBadge kind={compatWorks ? 'success' : 'danger'} label={compatLabel} dot={false} /></span>
    {/if}
    {#if favorite}
      <span class="fav" id="{uid}-fav" role="img" aria-label={msg('library.row.favorite')}><Heart size="1.4rem" strokeWidth={2} fill="currentColor" /></span>
    {/if}
    {#if actions && actions.length > 0}
      <div class="quick">
        {#each actions as action (action.id)}
          {@const unavailable = action.disabled === true}
          <Tooltip text={unavailable ? (action.reason ?? '') : action.label} describe={unavailable}>
            <button
              {...action.menu}
              class="quick-btn"
              class:active={action.active}
              class:pinned={action.pinned}
              class:quiet={action.pinned && action.quiet}
              class:pop={action.active && popped === action.id}
              aria-label={action.label}
              aria-describedby="{uid}-title"
              aria-disabled={unavailable ? 'true' : undefined}
              aria-busy={action.busy || undefined}
              {@attach refuseWhile(() => action.disabled === true || action.busy === true)}
              onclick={(event) => {
                popped = action.active ? '' : action.id;
                (action.menu?.onclick ?? action.onclick)?.(event);
              }}
            >
              {#if action.busy}
                <Spinner size="sm" tone="current" />
              {:else}
                <action.icon size="1.5rem" strokeWidth={1.8} fill={action.filled ? 'currentColor' : 'none'} />
              {/if}
            </button>
          </Tooltip>
        {/each}
      </div>
    {/if}
  </div>
  <button class="info" aria-describedby={describedBy} onclick={() => navigate('game', { id })}>
    <span class="title" id="{uid}-title">{title}</span>
    {#if meta}
      <span class="meta">{meta}</span>
    {/if}
  </button>
  {#if footer}
    <div class="footer">{@render footer()}</div>
  {/if}
</div>

<style>
  .card {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    min-width: 0;
  }

  .compat {
    position: absolute;
    left: 0.6rem;
    bottom: 0.6rem;
    display: flex;
    max-width: calc(100% - 1.2rem);
    overflow: hidden;
    pointer-events: none;
    transition: opacity var(--dur) var(--ease);
  }

  /* The play button shows over the same corner: the verdict steps aside meanwhile, and only then. */
  .cover-wrap.playable:hover .compat,
  .cover-wrap.running .compat {
    opacity: 0;
  }

  .play:focus-visible ~ .compat,
  .play[aria-busy='true'] ~ .compat {
    opacity: 0;
  }

  .source {
    position: absolute;
    left: 0.6rem;
    top: 0.6rem;
    display: flex;
    max-width: calc(100% - 1.2rem);
    overflow: hidden;
    pointer-events: none;
  }

  .cover-wrap {
    position: relative;
    border-radius: var(--radius-md);
    transition: transform var(--dur-panel) var(--ease);
  }

  .cover-wrap::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    top: 100%;
    height: 0.4rem;
  }

  .cover-wrap:hover,
  .cover-wrap:focus-within {
    transform: translateY(-0.4rem);
  }

  .cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--surface-3);
    transition: box-shadow var(--dur-panel) var(--ease);
  }

  .cover :global(img) {
    transition: transform var(--dur-slow) var(--ease);
  }

  .cover-wrap:hover .cover {
    box-shadow: var(--shadow-lift);
  }

  .cover-wrap:hover .cover :global(img) {
    transform: scale(1.04);
  }

  .fade {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, transparent 55%, var(--art-scrim));
    opacity: 0;
    transition: opacity var(--dur) var(--ease);
    pointer-events: none;
  }

  .cover-wrap:hover .fade {
    opacity: 1;
  }

  .play {
    position: absolute;
    left: 1rem;
    bottom: 1rem;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.2rem;
    height: 3.2rem;
    border-radius: var(--radius-md);
    background: var(--accent);
    color: var(--accent-on, var(--accent-on-default));
    opacity: 0;
    transform: translateY(0.4rem) scale(0.9);
    transition:
      opacity var(--dur) var(--ease),
      transform var(--dur-panel) var(--ease-spring),
      background var(--dur) var(--ease);
  }

  .play:hover {
    background: var(--accent-hover);
  }

  .cover-wrap:hover .play,
  .play:focus-visible,
  .play.running,
  .play[aria-busy='true'] {
    opacity: 1;
    transform: translateY(0) scale(1);
  }

  .play:active:not([aria-busy='true']) {
    transform: scale(0.92);
  }

  .play[aria-busy='true'] {
    cursor: progress;
  }

  .quick {
    position: absolute;
    top: 0.8rem;
    right: 0.8rem;
    display: flex;
    gap: 0.4rem;
  }

  .quick-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3rem;
    height: 3rem;
    border-radius: var(--radius-sm);
    background: var(--art-control);
    color: var(--art-text);
    opacity: 0;
    transform: scale(0.8);
    transform-origin: top right;
    pointer-events: none;
    transition:
      opacity var(--dur) var(--ease),
      transform var(--dur-panel) var(--ease-spring),
      background var(--dur) var(--ease);
  }

  .cover-wrap:hover .quick-btn,
  .cover-wrap:focus-within .quick-btn,
  .quick-btn[aria-expanded='true'],
  .quick-btn.pinned {
    opacity: 1;
    transform: scale(1);
    pointer-events: auto;
  }

  .quick-btn:hover:not([aria-disabled='true']) {
    background: var(--art-control-hover);
  }

  .quick-btn.active {
    background: var(--accent);
    color: var(--accent-on, var(--accent-on-default));
  }

  .quick-btn.active:hover:not([aria-disabled='true']) {
    background: var(--accent-hover);
  }

  .cover-wrap:not(:hover):not(:focus-within) .quick-btn.quiet {
    background: var(--art-control-quiet);
    color: var(--art-text);
    transform: scale(0.8);
  }

  .cover-wrap .quick-btn:active:not([aria-disabled='true']) {
    transform: scale(0.9);
  }

  .quick-btn[aria-disabled='true'] {
    cursor: default;
  }

  .quick-btn[aria-busy='true'] {
    cursor: progress;
  }

  .quick-btn[aria-disabled='true'] :global(svg) {
    opacity: 0.45;
  }

  .quick-btn.pop :global(svg) {
    animation: pop-in var(--dur-slow) var(--ease-spring);
  }

  .fav {
    position: absolute;
    right: 0.6rem;
    bottom: 0.6rem;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.4rem;
    height: 2.4rem;
    border-radius: var(--radius-round);
    background: var(--art-control);
    color: var(--art-text);
    pointer-events: none;
  }

  .info {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.2rem;
    min-width: 0;
    text-align: left;
  }

  .title {
    width: 100%;
    font-size: var(--font-md);
    font-weight: 600;
    line-height: 1.3;
    letter-spacing: var(--tracking-heading);
    color: var(--text);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .meta {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }
</style>
