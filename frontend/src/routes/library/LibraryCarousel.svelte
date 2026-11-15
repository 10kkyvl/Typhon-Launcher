<script lang="ts">
  import { derived } from 'svelte/store';
  import HeroCarousel, { type HeroArt, type HeroSlide } from '../../lib/components/HeroCarousel.svelte';
  import { msg } from '../../lib/i18n';
  import type { GameArt } from '../../lib/services/metadata';
  import type { LibraryEntry } from '../../lib/library/entries';
  import { openGameMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { stageWide } from '../../lib/stores/stage';
  import { contextMenuTrigger } from '../../lib/utils/contextmenu';
  import { playtime, relativeDate } from '../../lib/utils/format';

  /**
   * The library's carousel: the model's slides with the line the old hero showed (last session, play
   * time). It stays while there are slides: nothing puts it away (decision В11). The slide on screen
   * stands on the stage (`stage`) and is the page's resting picture for the window backdrop
   * (`ambient`); a rested card overrides the backdrop, not the stage.
   */
  let {
    entries,
    art,
    running,
    busy,
    onplay,
    onstop,
  }: {
    entries: readonly LibraryEntry[];
    art: Readonly<Record<string, GameArt>>;
    running: ReadonlySet<string>;
    /** A launch or a stop of the game waits on Go. */
    busy: (id: string) => boolean;
    onplay: (id: string) => void;
    onstop: (id: string) => void;
  } = $props();

  // The cover a slide shows when its game has no wide art: the metadata cover, else a hero that is a
  // poster, else the game's own cover, which is also tried when the first fails; then the generated art.
  function slideArt(entry: LibraryEntry): HeroArt {
    const own = entry.catalogId ? art[entry.catalogId] : undefined;
    const fallback = entry.game?.cover || undefined;
    return { cover: own?.cover || (own?.heroIsPoster ? own.hero : '') || fallback, fallback };
  }

  // Whether each slide's game stands on the stage with wide art. "Not known yet" counts as none: the
  // art store has no entry for a game it has not answered for, which for a game with no art at all
  // lasts, so the slide shows its cover and gives way to the stage once the wide art is known. The
  // block's height does not depend on it, so nothing below moves.
  // Keyed on the ids as a string: an update that keeps the games subscribes to nothing again.
  const slideIds = $derived(entries.map((entry) => entry.id).join('\n'));
  const wide = $derived.by(() => {
    const ids = slideIds ? slideIds.split('\n') : [];
    return derived(ids.map(stageWide), (answers) => new Set(ids.filter((_, index) => answers[index].state === 'yes')));
  });

  function slideLine(entry: LibraryEntry): string {
    return [
      msg('games.libraryLastSessionLabel', { date: relativeDate(entry.lastPlayed) }),
      msg('games.libraryPlaytimeLabel', { time: playtime(entry.playtimeSeconds) }),
    ].join(' · ');
  }

  // The game of the slide on screen, as the carousel names it.
  let shown = $state<string>();

  // A right click or Shift+F10 on the carousel opens the menu of the game it shows, as on a card.
  const slideMenus = contextMenuTrigger(({ at, focus }, source) => {
    const id = shown;
    return Boolean(id && openGameMenu(id, { at, focus, trigger: null, source }));
  });

  const slides = $derived<HeroSlide[]>(
    entries.map((entry) => ({
      id: entry.id,
      title: entry.title,
      staged: $wide.has(entry.id),
      art: slideArt(entry),
      artSeed: entry.catalogId ?? entry.id,
      line: slideLine(entry),
      running: running.has(entry.id),
      launching: busy(entry.id),
    })),
  );
</script>

<div class="carousel" {@attach slideMenus}>
  <HeroCarousel {slides} bind:shown ambient stage onopen={(id) => navigate('game', { id })} {onplay} {onstop} />
</div>
