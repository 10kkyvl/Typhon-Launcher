<script module lang="ts">
  /** How long a slide stays before the next one comes, while nothing holds the carousel. */
  export const ADVANCE_MS = 7000;

  export interface HeroArt {
    /** A wide picture, or a poster when `heroIsPoster`. */
    hero?: string;
    /** The game's cover: a poster, used when the hero is missing or fails. */
    cover?: string;
    heroIsPoster?: boolean;
  }

  export interface HeroSlide {
    id: string;
    title: string;
    art: HeroArt;
    /** The line under the title (last session, play time): the page's words. */
    line?: string;
    running?: boolean;
    /** A launch of this game waits on Go. */
    launching?: boolean;
  }

  export type HeroPicture = { kind: 'wide' | 'poster'; url: string } | { kind: 'generated' };

  /** What a slide shows: the hero, then the cover as a poster, then the generated art; a failed URL is skipped. */
  export function heroPicture(art: HeroArt, failed: Record<string, true>): HeroPicture {
    const candidates: { kind: 'wide' | 'poster'; url: string }[] = [];
    if (art.hero) candidates.push({ kind: art.heroIsPoster ? 'poster' : 'wide', url: art.hero });
    if (art.cover && art.cover !== art.hero) candidates.push({ kind: 'poster', url: art.cover });
    return candidates.find((candidate) => !(candidate.url in failed)) ?? { kind: 'generated' };
  }
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { on } from 'svelte/events';
  import { Play, Square } from '@lucide/svelte';
  import GeneratedArt from '../art/GeneratedArt.svelte';
  import { msg } from '../i18n';
  import { modalOpen } from '../layers';
  import { motionOn } from '../motion/motion';
  import { claimAmbient } from '../stores/ambient';
  import { nextIndex } from '../utils/roving';
  import Button from './Button.svelte';
  import { runSlide } from './slideTimer';

  /**
   * The games the page puts forward, one slide at a time: a picture, the title (which opens the game),
   * the page's line under it and Play or Stop (WAI-ARIA carousel with a slide picker). With two
   * slides or more it moves on every ADVANCE_MS; the timer holds while the pointer is over it, focus
   * is inside it, the window is hidden or a dialog is open, and it does not run with motion off.
   * While the pointer or focus holds it, the slides keep their order on screen, each still showing
   * its newest state. A slide the player picks is announced; one the timer moves to is not.
   *
   * `ambient`: the window backdrop shows the game on screen (claimAmbient), for the page that wants
   * it. The prototype's logo and blurred underlay have no source in the launcher and are not drawn.
   */
  let {
    slides: incoming,
    onopen,
    onplay,
    onstop,
    ambient = false,
    level = 2,
  }: {
    slides: HeroSlide[];
    onopen: (id: string) => void;
    onplay: (id: string) => void;
    onstop: (id: string) => void;
    ambient?: boolean;
    /** The heading level of a slide's title, under the page's own heading. */
    level?: 2 | 3 | 4;
  } = $props();

  const uid = $props.id();

  let hovering = $state(false);
  let focused = $state(false);
  let pageHidden = $state(typeof document !== 'undefined' && document.hidden);
  let activeId = $state<string | null>(null);
  let failed = $state<Record<string, true>>({});
  let progress = $state(0);

  // Held by the pointer or focus, the order on screen stays as it was; each slide still shows its
  // newest state (a launch in flight), and one that left the list keeps its last.
  let frozen = $state<HeroSlide[] | null>(null);
  $effect(() => {
    const hold = hovering || focused;
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
    });
  });

  const auto = $derived($motionOn && count > 1);
  const running = $derived(auto && !hovering && !focused && !pageHidden && !$modalOpen);

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

  $effect(() => (ambient ? claimAmbient(current?.id ?? null) : undefined));

  const holds: Attachment<HTMLElement> = (node) => {
    const offs = [
      on(node, 'pointerenter', () => (hovering = true)),
      on(node, 'pointerleave', () => (hovering = false)),
      on(node, 'focusin', () => (focused = true)),
      on(node, 'focusout', (event) => {
        const next = event.relatedTarget;
        if (!(next instanceof Node) || !node.contains(next)) focused = false;
      }),
    ];
    return () => offs.forEach((off) => off());
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

  const fail = (url: string) => (failed = { ...failed, [url]: true });
</script>

{#if current}
  <section class="hero" aria-roledescription={msg('library.carousel.kind')} aria-label={msg('library.carousel.label')} {@attach holds}>
    <div class="slides" id="{uid}-slides" aria-live={running ? 'off' : 'polite'}>
      {#each slides as slide, index (slide.id)}
        {@const shownHere = index === active}
        {@const picture = heroPicture(slide.art, failed)}
        <div
          class="slide"
          class:on={shownHere}
          role="group"
          aria-roledescription={msg('library.carousel.slide')}
          aria-label={msg('library.carousel.position', { index: String(index + 1), total: String(count) })}
          aria-hidden={!shownHere || undefined}
          inert={!shownHere}
          data-picture={picture.kind}
        >
          <div class="art">
            {#if picture.kind === 'wide'}
              {#key picture.url}
                <img class="wide" src={picture.url} alt="" draggable="false" onerror={() => fail(picture.url)} />
              {/key}
            {:else}
              <span class="ground"><GeneratedArt seed={slide.id} shape="wide" /></span>
              {#if picture.kind === 'poster'}
                {#key picture.url}
                  <img class="poster" src={picture.url} alt="" draggable="false" onerror={() => fail(picture.url)} />
                {/key}
              {/if}
            {/if}
          </div>
          <div class="copy">
            {#snippet titleLink()}<button type="button" class="title-link" onclick={() => onopen(slide.id)}>{slide.title}</button>{/snippet}
            {#if level === 4}
              <h4 class="title">{@render titleLink()}</h4>
            {:else if level === 3}
              <h3 class="title">{@render titleLink()}</h3>
            {:else}
              <h2 class="title">{@render titleLink()}</h2>
            {/if}
            {#if slide.line}<p class="line">{slide.line}</p>{/if}
            <div class="cta">
              <Button variant="primary" size="lg" busy={slide.launching} onclick={() => (slide.running ? onstop : onplay)(slide.id)}>
                {#if slide.running}<Square size="1.6rem" strokeWidth={2} fill="currentColor" />{msg('ui.stop')}{:else}<Play size="1.8rem" strokeWidth={2} fill="currentColor" />{msg('ui.play')}{/if}
              </Button>
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
  .hero {
    position: relative;
    height: var(--hero-h);
    overflow: hidden;
    isolation: isolate;
    background: var(--surface-2);
  }

  .slides,
  .slide,
  .art,
  .ground {
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

  .wide {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  /* A poster keeps its own shape at the block's end; the game's generated art fills the rest. */
  .poster {
    position: absolute;
    top: var(--space-6);
    bottom: var(--space-6);
    right: var(--page-inset, var(--space-8));
    height: calc(100% - 2 * var(--space-6));
    aspect-ratio: 3 / 4;
    object-fit: cover;
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-lift);
  }

  /* The text stands on the scrim's full strength; the scrim fades out only in the padding above it. */
  .copy {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-12) var(--page-inset, var(--space-8)) var(--space-8);
    background: linear-gradient(to top, var(--hero-scrim) calc(100% - var(--space-12)), transparent);
  }

  .title {
    max-width: var(--prose-max);
    margin: 0;
    font-size: var(--font-hero);
    font-weight: 700;
    line-height: 1.15;
    letter-spacing: var(--tracking-title);
    color: var(--art-text);
    text-shadow: var(--art-text-shadow);
    text-wrap: balance;
    overflow-wrap: anywhere;
  }

  .title-link {
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    letter-spacing: inherit;
    text-align: left;
    text-shadow: inherit;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  .line {
    margin: 0;
    font-size: var(--font-md);
    color: var(--art-text-2);
    font-variant-numeric: tabular-nums;
  }

  .cta {
    margin-top: var(--space-2);
  }

  .picker {
    position: absolute;
    right: var(--page-inset, var(--space-8));
    bottom: var(--space-8);
    display: flex;
    gap: var(--space-1);
  }

  .pick {
    display: flex;
    align-items: center;
    width: var(--hero-pick-w);
    height: var(--control-sm);
    padding: 0;
    border: 0;
    background: none;
    border-radius: var(--radius-xs);
    cursor: pointer;
  }

  .track {
    display: block;
    width: 100%;
    height: var(--hero-pick-track);
    border-radius: var(--radius-bar);
    background: color-mix(in srgb, var(--art-text) 30%, transparent);
    overflow: hidden;
    transition: background var(--dur) var(--ease);
  }

  .pick:hover .track,
  .pick[aria-current='true'] .track {
    background: color-mix(in srgb, var(--art-text) 55%, transparent);
  }

  .bar {
    display: block;
    height: 100%;
    background: var(--art-text);
    transform-origin: left;
  }
</style>
