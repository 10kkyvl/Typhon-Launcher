<script module lang="ts">
  /** How long a slide stays before the next one comes, while nothing holds the carousel. */
  export const ADVANCE_MS = 7000;

  export interface HeroArt {
    /** The game's cover, a poster (a hero that is a poster is the cover again). */
    cover?: string;
    /** The game's own cover, tried when `cover` fails to load. */
    fallback?: string;
  }

  export interface HeroSlide {
    id: string;
    title: string;
    /** The game has wide art on the stage: the block draws no picture of its own. Default: off, the block shows the cover. */
    staged?: boolean;
    art: HeroArt;
    /** Seeds the generated art when there is no cover: the game's canonical id. Default: `id`. */
    artSeed?: string;
    /** The line under the title (last session, play time): the page's words. */
    line?: string;
    running?: boolean;
    /** A launch of this game waits on Go. */
    launching?: boolean;
  }
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { on } from 'svelte/events';
  import { Play, Square } from '@lucide/svelte';
  import { msg } from '../i18n';
  import { modalOpen } from '../layers';
  import { motionOn } from '../motion/motion';
  import { restAmbient } from '../stores/ambient';
  import { gameScope } from '../stores/gameTone';
  import { claimStage } from '../stores/stage';
  import { nextIndex } from '../utils/roving';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import TextLink from './TextLink.svelte';
  import { runSlide, watchInView } from './slideTimer';

  /**
   * The games the page puts forward, one slide at a time: the title (which opens the game), the page's
   * line under it and Play or Stop, on the stage's text plate (WAI-ARIA carousel with a slide picker).
   * The carousel draws no picture band: a slide whose game has wide art stands on the stage, which
   * the window draws behind it; one whose game has none shows the game's cover in the block. With two
   * slides or more it moves on every ADVANCE_MS; the timer holds while the pointer is over it, focus
   * is inside it, it is scrolled out of view, the window is hidden or a dialog is open, and it does
   * not run with motion off.
   * While the pointer or focus holds it, the slides keep their order on screen, each still showing
   * its newest state. A slide the player picks is announced; one the timer moves to is not. Each slide is
   * painted in its game's colour (stores/gameTone.ts), read as the carousel mounts: Play or Stop takes it.
   *
   * `stage`: the game on screen claims the stage (stores/stage.ts) by its id, whether or not it has
   * wide art there. `ambient`: it is the page's resting picture for the window backdrop
   * (restAmbient): it tints the window unless a screen claims a game or a card is rested on. Both
   * are the caller's choice, off by default. The prototype's logo has no source in the launcher.
   */
  let {
    slides: incoming,
    onopen,
    onplay,
    onstop,
    ambient = false,
    stage = false,
    level = 2,
    shown = $bindable(),
  }: {
    slides: HeroSlide[];
    onopen: (id: string) => void;
    onplay: (id: string) => void;
    onstop: (id: string) => void;
    /** The game on screen is the page's resting picture for the window backdrop. Default: off. */
    ambient?: boolean;
    /** The game on screen claims the stage. Default: off. */
    stage?: boolean;
    /** The heading level of a slide's title, under the page's own heading. */
    level?: 2 | 3 | 4;
    /** Read only: the id of the slide on screen, for what the page does with that game (its menu). */
    shown?: string;
  } = $props();

  const uid = $props.id();

  let hovering = $state(false);
  let focused = $state(false);
  let pageHidden = $state(typeof document !== 'undefined' && document.hidden);
  let inView = $state(true);
  let activeId = $state<string | null>(null);
  let progress = $state(0);

  // Held by the pointer or focus, or out of view, the order on screen stays as it was; each slide
  // still shows its newest state (a launch in flight), and one that left the list keeps its last.
  let frozen = $state<HeroSlide[] | null>(null);
  $effect(() => {
    const hold = hovering || focused || !inView;
    const latest = incoming;
    untrack(() => {
      if (hold && !frozen) frozen = latest;
      else if (!hold && frozen) frozen = null;
    });
  });
  const slides = $derived(frozen ? frozen.map((old) => incoming.find((slide) => slide.id === old.id) ?? old) : incoming);
  const count = $derived(slides.length);

  // The slide on screen is remembered by id, so a list that gains or loses a slide does not move it;
  // when its game leaves the list, the slide at the same place takes over.
  let lastIndex = 0;
  const active = $derived.by(() => {
    const found = slides.findIndex((slide) => slide.id === activeId);
    const index = found >= 0 ? found : Math.min(lastIndex, Math.max(0, count - 1));
    lastIndex = index;
    return index;
  });
  const current = $derived(slides[active]);
  $effect(() => {
    const id = current?.id ?? null;
    untrack(() => {
      if (activeId !== id) activeId = id;
      if (shown !== (id ?? undefined)) shown = id ?? undefined;
    });
  });

  const auto = $derived($motionOn && count > 1);
  const running = $derived(auto && inView && !hovering && !focused && !pageHidden && !$modalOpen);

  function go(index: number) {
    if (count === 0) return;
    activeId = slides[((index % count) + count) % count].id;
  }

  // The timer counts the time a slide has been shown while nothing held it: held, it stops; let go,
  // it runs out what is left. A new slide starts from nothing.
  let shownId: string | null = null;
  let elapsed = 0;
  $effect(() => {
    const id = current?.id ?? null;
    if (id !== shownId) {
      shownId = id;
      elapsed = 0;
      progress = 0;
    }
    if (!running) return;
    const before = elapsed;
    const stop = runSlide(
      ADVANCE_MS - before,
      () => go(active + 1),
      (ran) => (progress = Math.min(1, (before + ran) / ADVANCE_MS)),
    );
    return () => {
      elapsed = before + stop();
    };
  });

  $effect(() => on(document, 'visibilitychange', () => (pageHidden = document.hidden)));

  // The claim follows the id of the game on screen, not the slide object: an update of the page that
  // keeps the game (a new line, a launch in flight) claims nothing again.
  const claimed = $derived(ambient ? (current?.id ?? null) : null);
  $effect(() => restAmbient(claimed));
  const onStage = $derived(stage ? (current?.id ?? null) : null);
  $effect(() => claimStage(onStage));

  // Focus holds the carousel while the focused element is inside it. On focusout that is read once the
  // event is over: a window that loses focus sends focusout with no relatedTarget though focus stays
  // put, and a control removed while focused leaves focus on <body>.
  const holds: Attachment<HTMLElement> = (node) => {
    let alive = true;
    // A new watcher starts from "in view" and reports only changes.
    inView = true;
    const unwatch = watchInView(node, (next) => (inView = next));
    const recheck = () =>
      queueMicrotask(() => {
        if (alive) focused = node.contains(document.activeElement);
      });
    const offs = [
      on(node, 'pointerenter', () => (hovering = true)),
      on(node, 'pointerleave', () => (hovering = false)),
      on(node, 'focusin', () => (focused = true)),
      on(node, 'focusout', recheck),
    ];
    return () => {
      alive = false;
      unwatch();
      offs.forEach((off) => off());
    };
  };

  let picker = $state<HTMLElement>();

  function onkeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const all = [...(picker?.querySelectorAll<HTMLButtonElement>('.pick') ?? [])];
    const next = nextIndex(all.indexOf(document.activeElement as HTMLButtonElement), count, event.key, { orientation: 'horizontal' });
    if (next === null) return;
    event.preventDefault();
    go(next);
    all[next]?.focus();
  }
</script>

{#if current}
  <section class="hero" aria-roledescription={msg('library.carousel.kind')} aria-label={msg('library.carousel.label')} {@attach holds}>
    <div class="slides" id="{uid}-slides" aria-live={running ? 'off' : 'polite'}>
      {#each slides as slide, index (slide.id)}
        {@const shownHere = index === active}
        <div
          class="slide"
          class:on={shownHere}
          role="group"
          aria-roledescription={msg('library.carousel.slide')}
          aria-label={msg('library.carousel.position', { index: String(index + 1), total: String(count) })}
          aria-hidden={!shownHere || undefined}
          inert={!shownHere}
          data-picture={slide.staged ? 'stage' : 'cover'}
          {@attach gameScope(slide.id)}
        >
          <div class="copy">
            {#if !slide.staged}
              <div class="cover">
                <Artwork src={slide.art.cover ?? ''} fallback={slide.art.fallback} label={slide.title} seed={slide.artSeed ?? slide.id} ratio="3 / 4" radius="var(--radius-md)" />
              </div>
            {/if}
            <div class="stand" class:stage-copy={slide.staged}>
              {#snippet titleLink()}<TextLink tone="plain" onclick={() => onopen(slide.id)}>{slide.title}</TextLink>{/snippet}
              {#if level === 4}
                <h4 class="title">{@render titleLink()}</h4>
              {:else if level === 3}
                <h3 class="title">{@render titleLink()}</h3>
              {:else}
                <h2 class="title">{@render titleLink()}</h2>
              {/if}
              {#if slide.line}<p class="line">{slide.line}</p>{/if}
              <div class="cta">
                <Button variant="play" size="lg" busy={slide.launching} onclick={() => (slide.running ? onstop : onplay)(slide.id)}>
                  {#if slide.running}<Square size="1.6rem" strokeWidth={2} fill="currentColor" />{msg('ui.stop')}{:else}<Play size="1.8rem" strokeWidth={2} fill="currentColor" />{msg('ui.play')}{/if}
                </Button>
              </div>
            </div>
          </div>
        </div>
      {/each}
    </div>

    {#if count > 1}
      <div class="picker" role="group" aria-label={msg('library.carousel.picker')} bind:this={picker}>
        {#each slides as slide, index (slide.id)}
          <button
            type="button"
            class="pick"
            aria-label={msg('library.carousel.pick', { index: String(index + 1), total: String(count), title: slide.title })}
            aria-current={index === active ? 'true' : undefined}
            aria-controls="{uid}-slides"
            tabindex={index === active ? 0 : -1}
            onclick={() => go(index)}
            {onkeydown}
          >
            <span class="chip" aria-hidden="true">
              <Artwork src={slide.art.cover ?? ''} fallback={slide.art.fallback} seed={slide.artSeed ?? slide.id} ratio="3 / 4" radius="var(--radius-sm)" />
            </span>
            <span class="track" aria-hidden="true">
              {#if index === active && auto}<span class="bar" style:transform="scaleX({progress})"></span>{/if}
            </span>
          </button>
        {/each}
      </div>
    {/if}
  </section>
{/if}

<style>
  /* No ground of its own: the stage shows through. The block ends at the stage's foot and clips nothing:
     the words' plate eases out below it. */
  .hero {
    position: relative;
    height: calc(var(--stage-foot) - var(--titlebar-h) - var(--topbar-h));
    min-height: var(--hero-min-h);
    isolation: isolate;
  }

  .slides,
  .slide {
    position: absolute;
    inset: 0;
  }

  .slide {
    opacity: 0;
    transition: opacity var(--dur-slow) var(--ease);
  }

  .slide.on {
    opacity: 1;
  }

  .copy {
    display: flex;
    align-items: flex-end;
    gap: var(--space-6);
    height: 100%;
    padding: var(--space-8) var(--page-inset, var(--space-8)) var(--space-12);
  }

  .cover {
    flex: none;
    width: var(--hero-cover-w);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-lift);
  }

  .stand {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
    min-width: 0;
  }

  .title {
    max-width: var(--prose-max);
    margin: 0;
    font-size: var(--font-hero);
    font-weight: 700;
    line-height: 1.15;
    letter-spacing: var(--tracking-title);
    color: var(--text);
    text-wrap: balance;
    overflow-wrap: anywhere;
  }

  .line {
    margin: 0;
    font-size: var(--font-md);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .cta {
    margin-top: var(--space-2);
  }

  /* The slide controls are the games themselves: a short row of their covers, the one on screen raised.
     They stand on the art beside the words, so they bring the words' plate colour: the ring and the
     timer's bar hold 3:1 on it over any picture (theme-contrast.test.ts). The padding holds the raised
     cover's ring. */
  .picker {
    position: absolute;
    right: var(--page-inset, var(--space-8));
    bottom: var(--space-8);
    display: flex;
    align-items: flex-end;
    gap: var(--space-2);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--scrim-copy);
  }

  .pick {
    display: grid;
    gap: var(--space-1);
    width: var(--hero-chip-w);
    padding: 0;
    border: 0;
    background: none;
    border-radius: var(--radius-sm);
    cursor: pointer;
    opacity: 0.62;
    transition:
      opacity var(--dur) var(--ease),
      transform var(--dur-slow) var(--ease);
  }

  .pick:hover,
  .pick[aria-current='true'] {
    opacity: 1;
  }

  .pick[aria-current='true'] {
    transform: translateY(calc(-1 * var(--space-1)));
  }

  .chip {
    display: block;
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-lift);
  }

  .pick[aria-current='true'] .chip {
    outline: 2px solid var(--text);
    outline-offset: 2px;
  }

  .track {
    display: block;
    height: var(--hero-pick-track);
    border-radius: var(--radius-bar);
    overflow: hidden;
  }

  .pick[aria-current='true'] .track {
    background: color-mix(in srgb, var(--text) 24%, transparent);
  }

  .bar {
    display: block;
    height: 100%;
    background: var(--text);
    transform-origin: left;
  }
</style>
