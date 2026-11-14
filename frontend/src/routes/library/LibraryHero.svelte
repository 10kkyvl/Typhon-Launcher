<script lang="ts">
  import { Play, Square, X } from '@lucide/svelte';
  import { tick } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import { msg } from '../../lib/i18n';
  import { focusableIn, focusQuietly } from '../../lib/layers/focus';
  import { gameContextMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { playtime, relativeDate } from '../../lib/utils/format';
  import { revealImage } from '../../lib/utils/revealImage';
  import type { LibraryEntry } from '../../lib/library/entries';

  let {
    hero,
    art,
    running,
    onplay,
    onhide,
  }: { hero: LibraryEntry; art: string; running: boolean; onplay: () => void; onhide: () => void } = $props();

  let heroArtFailed = $state('');
  const heroArt = $derived(art);
  const heroArtShown = $derived(heroArt !== '' && heroArtFailed !== heroArt);

  let heroSection = $state<HTMLElement>();

  // The hide button goes with the block: focus moves on to what Tab would reach next.
  async function hideHero() {
    const all = focusableIn(document.body);
    const mine = all.filter((el) => heroSection?.contains(el));
    const next = all[all.indexOf(mine[mine.length - 1]) + 1] ?? null;
    onhide();
    await tick();
    const active = document.activeElement;
    if (next?.isConnected && (!active || active === document.body || !active.isConnected)) focusQuietly(next);
  }
</script>

{#key hero.id}
  <section
    bind:this={heroSection}
    class="hero"
    class:plain={!heroArtShown}
    aria-label={msg('games.libraryContinuePlayingTitle')}
    {@attach gameContextMenu(hero.id)}
  >
    {#if heroArtShown}
      <div class="hero-art">
        {#key heroArt}
          <img
            src={heroArt}
            alt=""
            class="media-reveal"
            use:revealImage
            decoding="async"
            draggable="false"
            onerror={() => (heroArtFailed = heroArt)}
          />
        {/key}
      </div>
      <div class="hero-scrim"></div>
    {/if}
    <div class="hero-body">
      <span class="hero-eyebrow">{msg('games.libraryContinuePlayingTitle')}</span>
      <h2 class="hero-title">{hero.title}</h2>
      <p class="hero-meta">
        <span>{msg('games.libraryLastSessionLabel', { date: relativeDate(hero.lastPlayed) })}</span>
        <span>{msg('games.libraryPlaytimeLabel', { time: playtime(hero.playtimeSeconds) })}</span>
      </p>
      <div class="hero-actions">
        <Button variant="primary" size="lg" onclick={onplay}>
          {#if running}
            <Square size="1.4rem" strokeWidth={2} fill="currentColor" />
            {msg('games.stop')}
          {:else}
            <Play size="1.5rem" strokeWidth={2} fill="currentColor" />
            {msg('common.continue')}
          {/if}
        </Button>
        <Button size="lg" onclick={() => navigate('game', { id: hero.id })}>{msg('games.libraryMoreDetailsButton')}</Button>
      </div>
    </div>
    <span class="hero-close">
      <IconButton variant="secondary" size="sm" label={msg('games.libraryHideLabel')} onclick={hideHero}>
        <X size="1.6rem" strokeWidth={1.8} />
      </IconButton>
    </span>
  </section>
{/key}

<style>
  .hero {
    position: relative;
    display: flex;
    align-items: flex-end;
    min-height: clamp(20rem, 17vw, 28rem);
    margin-bottom: var(--space-8);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
    overflow: hidden;
    isolation: isolate;
  }

  .hero.plain {
    background:
      linear-gradient(115deg, var(--accent-subtle), transparent 62%),
      var(--surface-2);
  }

  .hero-art,
  .hero-scrim {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }

  .hero-art img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center 28%;
    transition: transform var(--dur-slow) var(--ease);
  }

  .hero-art img:global([data-ready]) {
    animation: media-in var(--dur-slow) var(--ease);
  }

  .hero:hover .hero-art img {
    transform: scale(1.03);
  }

  .hero-scrim {
    background:
      linear-gradient(
        90deg,
        color-mix(in srgb, var(--surface-2) 96%, transparent) 0%,
        color-mix(in srgb, var(--surface-2) 86%, transparent) 40%,
        color-mix(in srgb, var(--surface-2) 30%, transparent) 72%,
        color-mix(in srgb, var(--surface-2) 6%, transparent) 100%
      ),
      linear-gradient(0deg, color-mix(in srgb, var(--surface-2) 55%, transparent) 0%, transparent 62%);
  }

  .hero-body {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
    min-width: 0;
    max-width: min(64rem, 100%);
    padding: var(--space-6) var(--space-8);
    color: var(--text);
  }

  .hero-body > * {
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .hero-body > .hero-title {
    animation-duration: var(--dur-slow);
  }

  .hero-body > .hero-meta {
    animation-duration: var(--dur-slow);
    animation-delay: calc(var(--dur-fast) / 2);
  }

  .hero-eyebrow {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
  }

  .hero-title {
    max-width: 100%;
    font-size: var(--font-title);
    font-weight: 600;
    line-height: 1.1;
    letter-spacing: var(--tracking-title);
    overflow-wrap: anywhere;
  }

  .hero-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.2rem var(--space-5);
    font-size: var(--font-sm);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .hero-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .hero-close {
    position: absolute;
    top: var(--space-3);
    right: var(--space-3);
  }
</style>
