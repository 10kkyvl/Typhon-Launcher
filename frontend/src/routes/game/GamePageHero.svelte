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

  /**
   * The game page's header: the breadcrumb, the state badges, the title and the meta line; the action
   * row and its notes come from the page as `actions`. It draws no picture of its own. `look` says what
   * it stands on: `stage`, the game's wide art on the window's stage (ShellBackdrop), where the words
   * bring their own plate and the header ends at the stage's foot; `cover`, an empty stage, with the
   * game's cover beside the words; `pending`, not known yet, with the cover's place held by its skeleton.
   */
  let {
    info,
    title,
    coverSrc,
    look,
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
    look: 'stage' | 'cover' | 'pending';
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

  const staged = $derived(look === 'stage');
</script>

<section class="hero" class:staged data-part="game-hero">
  <div class="words" class:stage-copy={staged}>
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
      {#if !staged}
        <div class="cover">
          {#if title && look === 'cover'}
            <Artwork src={coverSrc} alt={title} ratio="3 / 4" radius="var(--radius-md)" />
          {:else}
            <div class="skeleton cover-skeleton"></div>
          {/if}
        </div>
      {/if}

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
  </div>
</section>

<style>
  .hero {
    margin-bottom: var(--space-8);
    padding: var(--space-4) var(--page-inset) var(--space-5);
    min-height: clamp(24rem, 28vh, 30rem);
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    gap: var(--space-8);
  }

  /* On the stage the header is its words alone and ends at the stage's foot, where the art gives way. */
  .hero.staged {
    min-height: max(24rem, calc(var(--stage-foot) - var(--titlebar-h) - var(--topbar-h)));
    justify-content: flex-end;
  }

  /* Off the stage the breadcrumb and the cover with the words are the header's own rows. */
  .words {
    display: contents;
  }

  /* On the stage they are one block of words as wide as they need, so the plate eases out to the right
     and the rest of the art stays bare. The plate reaches back to the scroller's edge and fades in up to
     the words, which on a wide window start well inside it. */
  .words.stage-copy {
    --copy-lead: var(--page-inset);
    display: block;
    align-self: flex-start;
    max-width: 64rem;
  }

  /* One text colour on the stage: the quiet greys would need a heavier plate. */
  .stage-copy :is(.breadcrumb, .crumb, .sep, .completed-on) {
    color: var(--text);
  }

  .stage-copy .breadcrumb {
    margin-bottom: var(--space-4);
  }

  .breadcrumb {
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
