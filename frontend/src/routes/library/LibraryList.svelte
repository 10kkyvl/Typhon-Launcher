<script lang="ts">
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import GameListRow, { type GameListRowAction } from '../../lib/components/GameListRow.svelte';
  import SourceBadge from '../../lib/components/SourceBadge.svelte';
  import type { GameArt } from '../../lib/services/metadata';
  import { openGameMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { bytesSize } from '../../lib/utils/format';
  import { entryCover, type LibraryEntry } from '../../lib/library/entries';
  import { rowDetails } from './entryText';
  import { createRoving } from './roving';
  import { noBrowserMenu, rowMenus } from './itemMenu';

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
    /** The page's own actions on a row (Unwatch on a watched game). */
    actions: (entry: LibraryEntry) => GameListRowAction[] | undefined;
    /** Plays the game, or stops it while it runs. */
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
    layout: 'list',
    idAt: (index) => entries[index]?.id,
    current: () => current,
    onmove: (id) => (current = id),
    // As the card's own key: not while a launch or a stop waits, and a running game is not launched again.
    onlaunch: (id) => {
      const entry = entries.find((one) => one.id === id);
      if (entry?.playable && !running.has(id) && !busy(id)) onplay(id);
    },
  });

  $effect(() => {
    void entries;
    void current;
    roving.refresh();
  });

  function endViewSwitch(event: AnimationEvent) {
    if (event.target === event.currentTarget) switched = false;
  }

  const open = (id: string) => navigate('game', { id });

  function more(id: string, anchor: HTMLElement) {
    openGameMenu(id, { at: { anchor }, focus: 'first', trigger: null, source: anchor.closest('li') });
  }
</script>

<ul
  class="list"
  data-results
  aria-labelledby={labelledby}
  class:switched
  onanimationend={endViewSwitch}
  bind:this={element}
  {@attach roving.attach}
  {@attach rowMenus((index) => entries[index]?.id)}
  {@attach noBrowserMenu}
>
  {#each entries as entry (entry.id)}
    <GameListRow
      id={entry.id}
      title={entry.title}
      cover={entryCover(entry, art)}
      coverFallback={entry.game?.cover}
      artSeed={entry.catalogId ?? entry.id}
      details={rowDetails(entry)}
      size={entry.sizeBytes > 0 ? bytesSize(entry.sizeBytes) : ''}
      running={running.has(entry.id)}
      launching={busy(entry.id)}
      favorite={entry.favorite}
      actions={actions(entry)}
      titleTone="plain"
      ambient
      onopen={open}
      onplay={entry.playable ? onplay : undefined}
      onstop={entry.playable ? onplay : undefined}
      onmore={entry.kind === 'game' ? more : undefined}
    >
      {#snippet marks()}
        {#if entry.platform}
          <ConsoleTag platform={entry.platform} />
        {:else if entry.source}
          <SourceBadge source={entry.source} />
        {/if}
      {/snippet}
    </GameListRow>
  {/each}
</ul>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .list.switched {
    animation: rise-in var(--dur-panel) var(--ease);
  }
</style>
