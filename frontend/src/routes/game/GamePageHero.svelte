<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ChevronRight } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { completionLine, statusBadgeKind, statusLabel } from '../../lib/game/status';
  import { metaLine, orderPlatforms, type BusyState, type NotOnPcBadge } from '../../lib/game/view';
  import { msg } from '../../lib/i18n';
  import type { LibraryGame } from '../../lib/services/library';
  import type { CatalogGame } from '../../lib/services/sources';
  import { navigate } from '../../lib/stores/router';
  import { revealImage } from '../../lib/utils/revealImage';

  /**
   * The game page's hero: the art, the breadcrumb, the cover, the state badges, the title and the meta
   * line. The action row and its notes come from the page as `actions`.
   */
  let {
    info,
    title,
    coverSrc,
    heroSrc,
    showHero,
    heroFailed = $bindable(),
    titleHeading = $bindable(),
    localGame,
    installed,
    wasInstalled,
    running,
    notOnPcKind,
    updateAvailable,
    busy,
    popOnce,
    externalName,
    emulated,
    emulatedPlatform,
    imageMissing,
    actions,
  }: {
    info: CatalogGame | null;
    title: string;
    coverSrc: string;
    heroSrc: string;
    showHero: boolean;
    heroFailed: boolean;
    titleHeading: HTMLElement | undefined;
    localGame: LibraryGame | undefined;
    installed: boolean;
    wasInstalled: boolean;
    running: boolean;
    notOnPcKind: NotOnPcBadge;
    updateAvailable: boolean;
    busy: BusyState | null;
    popOnce: (node: HTMLElement) => void;
    externalName: string;
    emulated: boolean;
    emulatedPlatform: string;
    imageMissing: boolean;
    actions: Snippet;
  } = $props();

  const releaseYear = $derived.by(() => {
    if (!info) return '';
    if (info.releaseYear) return String(info.releaseYear);
    if (info.releaseDate) {
      const date = new Date(info.releaseDate);
      if (!Number.isNaN(date.getTime())) return String(date.getFullYear());
    }
    return '';
  });

  const platforms = $derived(orderPlatforms(info?.platforms));

  const metaParts = $derived(
    metaLine({
      year: releaseYear,
      developer: info?.developer,
      publisher: info?.publisher,
      genres: info?.genres,
      platforms,
    }),
  );
</script>

<section class="hero" class:plain={!showHero} data-part="game-hero">
  <div class="art">
    {#if showHero}
      {#key heroSrc}
        <img src={heroSrc} class="media-reveal" use:revealImage alt="" decoding="async" draggable="false" onerror={() => (heroFailed = true)} />
      {/key}
    {/if}
  </div>
  <div class="veil"></div>

  <nav class="breadcrumb">
    {#if installed}
      <span class="crumb"><TextLink tone="muted" onclick={() => navigate('installed')}>{msg('games.installedStatusWord')}</TextLink></span>
    {:else}
      <span class="crumb"><TextLink tone="muted" onclick={() => navigate('library')}>{msg('games.libraryWord')}</TextLink></span>
    {/if}
    <ChevronRight size="1.4rem" strokeWidth={1.8} />
    <span class="crumb current">{title || msg('games.detailFallbackTitle')}</span>
  </nav>

  <div class="foot">
    <div class="cover">
      {#if title}
        <Artwork src={coverSrc} alt={title} ratio="3 / 4" radius="var(--radius-md)" />
      {:else}
        <div class="skeleton cover-skeleton"></div>
      {/if}
    </div>

    <div class="ident">
      <div class="state">
        {#if running}
          <StatusBadge kind="accent" label={msg('games.runningLabel')} />
        {:else if installed}
          <StatusBadge kind="success" label={msg('games.installedStatusWord')} dot={false} />
        {:else if localGame && wasInstalled}
          <StatusBadge kind="neutral" label={msg('games.detailBadgeUninstalled')} dot={false} />
        {:else if title}
          <StatusBadge kind="neutral" label={msg('games.notInstalledStatusWord')} dot={false} />
        {/if}
        {#if notOnPcKind}
          <StatusBadge
            kind="neutral"
            label={notOnPcKind === "other-platform" ? msg('games.badgeNotPcVersion') : msg('games.badgeNeverOnPc')}
            dot={false}
          />
        {/if}
        {#if updateAvailable && !busy}
          <span class="pop" use:popOnce>
            <StatusBadge kind="accent" label={msg('games.detailBadgeUpdateAvailable')} />
          </span>
        {/if}
        {#if externalName}
          <StatusBadge kind="neutral" label={externalName} dot={false} />
        {/if}
        {#if emulated}
          <ConsoleTag platform={emulatedPlatform} />
          {#if imageMissing}<StatusBadge kind="danger" label={msg('emul.imageMissing')} dot={false} />{/if}
        {/if}
        {#if localGame?.status}
          <StatusBadge kind={statusBadgeKind(localGame.status)} label={statusLabel(localGame.status)} dot={false} />
          {@const finished = completionLine(localGame.status, localGame.completedAt)}
          {#if finished}<span class="completed-on">{finished}</span>{/if}
        {/if}
        {#each localGame?.tags ?? [] as tag (tag)}
          <StatusBadge kind="neutral" label={tag} dot={false} />
        {/each}
      </div>

      {#if title}
        <h1 class="title" tabindex="-1" bind:this={titleHeading}>{title}</h1>
      {:else}
        <div class="skeleton title-skeleton"></div>
      {/if}

      {#if metaParts.length > 0}
        <p class="metaline">
          {#each metaParts as part, index (part)}
            {#if index > 0}<span class="sep">·</span>{/if}
            <span>{part}</span>
          {/each}
        </p>
      {/if}

      {@render actions()}
    </div>
  </div>
</section>

<style>
  .hero {
    position: relative;
    margin-bottom: var(--space-8);
    padding: var(--space-4) var(--page-inset) var(--space-5);
    min-height: clamp(31rem, 42vh, 48rem);
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    gap: var(--space-8);
  }

  .art {
    position: absolute;
    inset: 0;
    overflow: hidden;
  }

  /* The art and its veil melt into the window backdrop below the hero instead of ending on the page colour. */
  .art,
  .veil {
    -webkit-mask-image: linear-gradient(180deg, #000 55%, transparent 100%);
    mask-image: linear-gradient(180deg, #000 55%, transparent 100%);
  }

  .art img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: 50% 28%;
  }

  .hero.plain {
    min-height: clamp(24rem, 28vh, 30rem);
  }

  .hero.plain .art {
    background:
      radial-gradient(100% 78% at 6% 0%, color-mix(in srgb, var(--accent) 20%, transparent), transparent 64%),
      radial-gradient(80% 70% at 88% 8%, var(--hover), transparent 62%),
      linear-gradient(180deg, var(--surface-3), var(--bg) 86%);
  }

  .veil {
    position: absolute;
    inset: 0;
    pointer-events: none;
    background:
      linear-gradient(
        180deg,
        color-mix(in srgb, var(--bg) 58%, transparent) 0%,
        color-mix(in srgb, var(--bg) 8%, transparent) 20%,
        color-mix(in srgb, var(--bg) 8%, transparent) 38%,
        color-mix(in srgb, var(--bg) 62%, transparent) 68%,
        color-mix(in srgb, var(--bg) 92%, transparent) 88%,
        var(--bg) 100%
      ),
      linear-gradient(
        90deg,
        color-mix(in srgb, var(--bg) 82%, transparent) 0%,
        color-mix(in srgb, var(--bg) 52%, transparent) 34%,
        color-mix(in srgb, var(--bg) 10%, transparent) 62%,
        transparent 80%
      );
  }

  .hero.plain .veil {
    background: linear-gradient(
      180deg,
      transparent 60%,
      color-mix(in srgb, var(--bg) 60%, transparent) 82%,
      var(--bg) 100%
    );
  }

  .breadcrumb {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.8rem;
    color: var(--text-3);
  }

  .crumb {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .crumb.current {
    color: var(--text-2);
    font-weight: 500;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .foot {
    position: relative;
    display: flex;
    align-items: flex-end;
    gap: var(--space-6);
  }

  .cover {
    width: clamp(13rem, 11vw, 18rem);
    flex-shrink: 0;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: rise-in var(--dur-slow) var(--ease) backwards;
  }

  .cover-skeleton {
    aspect-ratio: 3 / 4;
    border-radius: var(--radius-md);
  }

  .ident {
    flex: 1;
    min-width: 0;
  }

  .state {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    flex-wrap: wrap;
    margin-bottom: 1rem;
    min-height: 2.4rem;
  }

  .pop {
    display: inline-flex;
  }

  .completed-on {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .pop:global(.fresh) {
    animation: badge-pop var(--dur-slow) var(--ease-spring) backwards;
  }

  .title {
    font-size: clamp(2.8rem, 2.4vw + 1rem, 4.6rem);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.04;
    text-wrap: balance;
    max-width: 26ch;
    text-shadow: 0 0.2rem 2.4rem color-mix(in srgb, var(--bg) 70%, transparent);
  }

  .state,
  .title,
  .metaline {
    animation: rise-in var(--dur-slow) var(--ease) backwards;
    animation-delay: calc(var(--dur-fast) / 2);
  }

  @keyframes badge-pop {
    from {
      opacity: 0;
      transform: scale(0.7);
    }
  }

  .title-skeleton {
    width: min(48rem, 60%);
    height: 4.4rem;
    border-radius: var(--radius-md);
  }

  .metaline {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.7rem;
    margin-top: 1rem;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .sep {
    color: var(--text-3);
  }

  .skeleton {
    background: var(--surface-3);
    animation: loading-breathe 1.6s ease-in-out 3;
    border-radius: var(--radius-sm);
  }

  @media (max-width: 1200px) {
    .hero {
      min-height: 28rem;
      gap: var(--space-6);
    }

    .foot {
      gap: var(--space-4);
    }

    .cover {
      width: 10rem;
    }
  }
</style>
