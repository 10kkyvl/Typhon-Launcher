<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { on } from 'svelte/events';
  import { Ellipsis, Heart, Play, Square } from '@lucide/svelte';
  import { msg } from '../i18n';
  import Artwork from './Artwork.svelte';
  import IconButton from './IconButton.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import TextLink from './TextLink.svelte';
  import type { GameCardAction } from './GameCard.svelte';
  import { ambientHover } from '../utils/ambientHover';

  /** One of the page's own actions on a row (Unwatch): busy while its request runs, as on a card. */
  export type GameListRowAction = Pick<GameCardAction, 'id' | 'label' | 'icon' | 'busy' | 'onclick'>;

  /**
   * One game in a list: an `<li>`, so the page puts it in a `<ul>` or `<ol>`. Its main action is the
   * title, which opens the game; a press anywhere else on the row that is not one of its controls
   * opens it too. Play or Stop (when the page gives `onplay` / `onstop`), busy while the launch is in
   * flight: focused and not disabled, a second press sends nothing. "More" asks the page for the
   * game's menu (`onmore`); the row owns no menu. The page passes its own secondary texts, the size
   * (shown once) and its marks (status, source). `actions`: the page's own controls, after Play.
   * `titleTone`: the title link's tone (TextLink); `ambient`: the window backdrop follows the game
   * while the pointer rests on the row (the library).
   */
  let {
    id,
    title,
    cover = '',
    coverFallback = '',
    artSeed,
    details = [],
    size = '',
    running = false,
    launching = false,
    favorite = false,
    marks,
    actions,
    titleTone = 'muted',
    ambient = false,
    lazy = false,
    onopen,
    onplay,
    onstop,
    onmore,
  }: {
    id: string;
    title: string;
    cover?: string;
    /** The game's own cover, tried when `cover` fails to load. */
    coverFallback?: string;
    /** Seeds the generated art when there is no cover: the game's canonical id. Default: `id`. */
    artSeed?: string;
    /** The page's secondary texts under the title (last played, play time). */
    details?: string[];
    /** The size on disk, shown once at the row's end. */
    size?: string;
    running?: boolean;
    /** A launch of this game waits on Go. */
    launching?: boolean;
    favorite?: boolean;
    /** Status and source marks, drawn before the size. */
    marks?: Snippet;
    actions?: GameListRowAction[];
    titleTone?: 'muted' | 'plain';
    ambient?: boolean;
    /** Far off screen the row is not laid out or painted (content-visibility): for a long list. */
    lazy?: boolean;
    onopen: (id: string) => void;
    onplay?: (id: string) => void;
    onstop?: (id: string) => void;
    /** Asks the page to open the game's menu at `anchor`. */
    onmore?: (id: string, anchor: HTMLElement) => void;
  } = $props();

  const uid = $props.id();
  const action = $derived(running ? onstop : onplay);

  // The row's own press opens the game; a press on one of its controls is that control's.
  const pressOpens: Attachment<HTMLElement> = (node) =>
    on(node, 'click', (event) => {
      if (event.target instanceof Element && event.target.closest('button, a, input, select, textarea, [role="button"]')) return;
      onopen(id);
    });
</script>

<li class="row" class:running class:lazy {@attach pressOpens} use:ambientHover={ambient ? id : null}>
  <span class="thumb"><Artwork src={cover} fallback={coverFallback} alt="" label={title} seed={artSeed ?? id} ratio="3 / 4" radius="var(--radius-xs)" /></span>
  <div class="main">
    <span class="title" id="{uid}-title"><TextLink tone={titleTone} truncate onclick={() => onopen(id)}>{title}</TextLink></span>
    {#if details.length > 0}
      <span class="details">
        {#each details as detail, index (index)}<span>{detail}</span>{/each}
      </span>
    {/if}
  </div>
  <div class="marks">
    {#if favorite}
      <span class="fav" role="img" aria-label={msg('library.row.favorite')}><Heart size="1.4rem" strokeWidth={2} fill="currentColor" /></span>
    {/if}
    {@render marks?.()}
    {#if running}<StatusBadge kind="success" label={msg('games.runningLabel')} />{/if}
  </div>
  {#if size}<span class="size">{size}</span>{/if}
  <div class="actions">
    {#if action}
      <IconButton
        label={running ? msg('ui.stop') : msg('ui.play')}
        variant={running ? 'secondary' : 'primary'}
        busy={launching}
        aria-describedby="{uid}-title"
        onclick={() => action(id)}
      >
        {#if running}<Square size="1.4rem" strokeWidth={2} fill="currentColor" />{:else}<Play size="1.6rem" strokeWidth={2} fill="currentColor" />{/if}
      </IconButton>
    {/if}
    {#each actions ?? [] as action (action.id)}
      <IconButton label={action.label} busy={action.busy} aria-describedby="{uid}-title" onclick={(event) => action.onclick?.(event)}>
        <action.icon size="1.6rem" strokeWidth={1.8} />
      </IconButton>
    {/each}
    {#if onmore}
      <IconButton label={msg('library.row.more')} aria-describedby="{uid}-title" onclick={(event) => onmore(id, event.currentTarget as HTMLElement)}>
        <Ellipsis size="1.8rem" />
      </IconButton>
    {/if}
  </div>
</li>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    cursor: pointer;
    transition: background var(--dur) var(--ease);
  }

  /* Off screen it keeps the height of its cover; what it draws (the hover fill, focus rings) stays
     inside its padding, where containment clips. */
  .row.lazy {
    content-visibility: auto;
    contain-intrinsic-size: auto calc(var(--row-thumb-w) * 4 / 3);
  }

  .row:hover,
  .row:focus-within {
    background: var(--hover);
  }

  .thumb {
    display: block;
    flex-shrink: 0;
    width: var(--row-thumb-w);
    border-radius: var(--radius-xs);
    overflow: hidden;
  }

  .main {
    display: flex;
    flex: 1 1 auto;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .title {
    display: flex;
    min-width: 0;
    font-size: var(--font-md);
    font-weight: 600;
  }

  .details {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-3);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .marks {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .fav {
    display: flex;
    color: var(--accent-text);
  }

  .size {
    font-size: var(--font-sm);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }
</style>
