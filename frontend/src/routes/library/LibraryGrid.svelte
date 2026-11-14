<script lang="ts">
  import GameCard, { type GameCardAction } from '../../lib/components/GameCard.svelte';
  import type { GameArt } from '../../lib/services/metadata';
  import { entryCover, type LibraryEntry } from '../../lib/library/entries';
  import { entryMeta, entrySubtitle } from './entryText';
  import { noBrowserMenu } from './itemMenu';

  let {
    entries,
    art,
    running,
    busy,
    actions,
    onplay,
    labelledby,
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
    /** The view has just changed to this one: it rises in once. */
    switched?: boolean;
    element?: HTMLElement;
  } = $props();

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
  {@attach noBrowserMenu}
>
  {#each entries as entry (entry.id)}
    <li class="cell">
      <GameCard
        id={entry.id}
        title={entry.title}
        cover={entryCover(entry, art)}
        ambient
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
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--space-6) var(--space-5);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .cell {
    min-width: 0;
  }

  .grid.switched {
    animation: rise-in var(--dur-panel) var(--ease);
  }

  @media (min-width: 2200px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    }
  }

  @media (max-width: 1400px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(14.5rem, 1fr));
    }
  }
</style>
