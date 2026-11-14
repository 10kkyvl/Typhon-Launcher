<script lang="ts">
  import { X } from '@lucide/svelte';
  import HeroCarousel, { type HeroArt, type HeroSlide } from '../../lib/components/HeroCarousel.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import { msg } from '../../lib/i18n';
  import type { GameArt } from '../../lib/services/metadata';
  import type { LibraryEntry } from '../../lib/library/entries';
  import { openGameMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { contextMenuTrigger } from '../../lib/utils/contextmenu';
  import { playtime, relativeDate } from '../../lib/utils/format';

  /**
   * The library's carousel: the model's slides with their pictures and the line the old hero showed
   * (last session, play time), and the control that puts it away for the session. The backdrop claim
   * (`ambient`) stays off: it would outrank the cards' own hover backdrop.
   */
  let {
    entries,
    art,
    running,
    busy,
    onplay,
    onstop,
    onhide,
  }: {
    entries: readonly LibraryEntry[];
    art: Readonly<Record<string, GameArt>>;
    running: ReadonlySet<string>;
    /** A launch or a stop of the game waits on Go. */
    busy: (id: string) => boolean;
    onplay: (id: string) => void;
    onstop: (id: string) => void;
    onhide: () => void;
  } = $props();

  // A wide picture is the hero; a metadata cover stands in as a poster; the game's own cover is
  // tried when the first picture fails, then the generated art.
  function slideArt(entry: LibraryEntry): HeroArt {
    const own = entry.catalogId ? art[entry.catalogId] : undefined;
    const cover = entry.game?.cover || undefined;
    if (own?.hero) return { hero: own.hero, heroIsPoster: own.heroIsPoster, cover: own.cover || cover };
    if (own?.cover) return { hero: own.cover, heroIsPoster: true, cover };
    return { cover };
  }

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

  let hideSpot = $state<HTMLElement>();

  /** The control that puts the carousel away, for focus to land on when the carousel comes back. */
  export function hideControl(): HTMLElement | undefined {
    return hideSpot?.querySelector('button') ?? undefined;
  }

  const slides = $derived<HeroSlide[]>(
    entries.map((entry) => ({
      id: entry.id,
      title: entry.title,
      art: slideArt(entry),
      artSeed: entry.catalogId ?? entry.id,
      line: slideLine(entry),
      running: running.has(entry.id),
      launching: busy(entry.id),
    })),
  );
</script>

<div class="carousel" {@attach slideMenus}>
  <HeroCarousel {slides} bind:shown onopen={(id) => navigate('game', { id })} {onplay} {onstop} />
  <span class="hide" bind:this={hideSpot}>
    <IconButton variant="secondary" size="sm" label={msg('library.carousel.hide')} onclick={onhide}>
      <X size="1.6rem" strokeWidth={1.8} />
    </IconButton>
  </span>
</div>

<style>
  .carousel {
    position: relative;
  }

  .hide {
    position: absolute;
    top: var(--space-3);
    right: var(--page-inset, var(--space-8));
  }
</style>
