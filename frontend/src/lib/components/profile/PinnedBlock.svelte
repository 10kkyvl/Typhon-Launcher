<script lang="ts">
  import Artwork from '../Artwork.svelte';
  import { statusLabel } from '../../game/status';
  import { msg } from '../../i18n';
  import type { BlockGame } from '../../profile/layoutView';
  import { shortDate } from '../../profile/view';
  import { playtime } from '../../utils/format';

  let { game, caption }: { game: BlockGame; caption: string } = $props();

  const meta = $derived.by(() => {
    const parts: string[] = [];
    if (game.seconds && game.seconds > 0) parts.push(playtime(game.seconds));
    if (game.completedAt) parts.push(msg('social.completedOn', { date: shortDate(game.completedAt) }));
    else if (game.status) parts.push(statusLabel(game.status));
    return parts.join(' · ');
  });
</script>

<div class="pinned">
  <button class="hero" type="button" onclick={game.open}>
    <Artwork src={game.hero || game.cover} alt="" label={game.title} ratio="16 / 9" />
    <span class="shade"></span>
    <span class="info">
      <span class="eyebrow">{msg('profile.pinnedEyebrow')}</span>
      <span class="title">{game.title}</span>
      {#if meta}<span class="meta">{meta}</span>{/if}
    </span>
  </button>
  {#if caption}
    <p class="caption">{caption}</p>
  {/if}
</div>

<style>
  .pinned {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
  }

  .hero {
    position: relative;
    display: block;
    width: 100%;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .hero :global(img) {
    transition: transform var(--dur-slow) var(--ease);
  }

  .hero:hover :global(img),
  .hero:focus-visible :global(img) {
    transform: scale(1.03);
  }

  .shade {
    position: absolute;
    inset: 0;
    background: linear-gradient(transparent 35%, rgba(5, 8, 12, 0.88));
    pointer-events: none;
  }

  .info {
    position: absolute;
    left: var(--space-5);
    right: var(--space-5);
    bottom: var(--space-4);
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    color: #f1f4f8;
  }

  .eyebrow {
    font-size: var(--font-xs);
    font-weight: 500;
    color: rgba(241, 244, 248, 0.7);
  }

  .title {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    line-height: 1.2;
    overflow-wrap: anywhere;
  }

  .meta {
    font-size: var(--font-xs);
    color: rgba(241, 244, 248, 0.78);
    font-variant-numeric: tabular-nums;
  }

  .caption {
    margin: 0;
    padding: var(--space-4) var(--space-5);
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
