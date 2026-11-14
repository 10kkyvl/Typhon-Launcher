<script lang="ts">
  import GameCard, { type GameCardAction } from '../../lib/components/GameCard.svelte';
  import type { GameArt } from '../../lib/services/metadata';
  import { entryCover, type LibraryEntry } from '../../lib/library/entries';
  import { entryMeta, entrySubtitle } from './entryText';
  import { createRoving } from './roving';
  import { noBrowserMenu } from './itemMenu';

  let {
    entries,
    art,
    running,
    busy,
    actions,
    onplay,
    labelledby,
    current = $bindable(),
    switched = $bindable(false),
    element = $bindable(),
  }: {
    entries: readonly LibraryEntry[];
    art: Readonly<Record<string, GameArt>>;
    running: ReadonlySet<string>;
    /** A launch or a stop of the game waits on Go. */
    busy: (id: string) => boolean;
    actions: (entry: LibraryEntry) => GameCardAction[] | undefined;
    onplay: (id: string) => void;
    /** Id of the heading that names the list. */
    labelledby: string;
    /** The game that holds the list's Tab stop: the one the player was on last. */
    current?: string;
    /** The view has just changed to this one: it rises in once. */
    switched?: boolean;
    element?: HTMLElement;
  } = $props();

  const roving = createRoving({
    layout: 'grid',
    idAt: (index) => entries[index]?.id,
    current: () => current,
    onmove: (id) => (current = id),
  });

  // The Tab order changes with the list's games and the current one, not with an update of a game
  // (a tick, new art); controls a game gains or loses are caught by the roving list itself.
  const ids = $derived(entries.map((entry) => entry.id).join('\n'));
  $effect(() => {
    void ids;
    void current;
    roving.refresh();
  });

  function endViewSwitch(event: AnimationEvent) {
    if (event.target === event.currentTarget) switched = false;
  }
</script>

<ul
  class="grid"
  data-results
  aria-labelledby={labelledby}
  class:switched
  onanimationend={endViewSwitch}
  bind:this={element}
  {@attach roving.attach}
  {@attach noBrowserMenu}
>
  {#each entries as entry (entry.id)}
    <li class="cell">
      <GameCard
        id={entry.id}
        title={entry.title}
        cover={entryCover(entry, art)}
        coverFallback={entry.game?.cover}
        artSeed={entry.catalogId ?? entry.id}
        favorite={entry.favorite}
        ambient
        titleFirst
        installed={entry.installed}
        running={running.has(entry.id)}
        launching={busy(entry.id)}
        source={entry.source}
        platform={entry.platform}
        meta={entry.kind === 'watched' ? entrySubtitle(entry) : entryMeta(entry)}
        actions={actions(entry)}
        onplay={entry.playable ? () => onplay(entry.id) : undefined}
      />
    </li>
  {/each}
</ul>

<style>
  /* The narrowest a card gets: the columns and the space a card off screen keeps before it is drawn. */
  .grid {
    --card-min: 16rem;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(var(--card-min), 1fr));
    gap: var(--space-6) var(--space-5);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  /* A card far off screen is not laid out or painted (content-visibility); it keeps the height of a
     cover and two lines meanwhile. Containment clips a cell's paint at its padding box, so the cell
     reaches as far as the card draws on hover and focus: --shadow-lift (offset 1.2rem down, blur
     2.8rem, spread -0.8rem) reaches 2rem sideways, 3.2rem below and 0.8rem above, and the lift adds
     0.4rem above. The margins give that room back to the grid. */
  .cell {
    min-width: 0;
    padding: var(--space-3) var(--space-5) var(--space-8);
    margin: calc(-1 * var(--space-3)) calc(-1 * var(--space-5)) calc(-1 * var(--space-8));
    content-visibility: auto;
    contain-intrinsic-size: auto calc(var(--card-min) * 4 / 3 + 2 * var(--control-sm));
  }

  .grid.switched {
    animation: rise-in var(--dur-panel) var(--ease);
  }

  @media (min-width: 2200px) {
    .grid {
      --card-min: 18rem;
    }
  }

  @media (max-width: 1400px) {
    .grid {
      --card-min: 14.5rem;
    }
  }
</style>
