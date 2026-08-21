<script lang="ts">
  import { Play, Star } from '@lucide/svelte';
  import type { Game } from '../mock/types';
  import { navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import Artwork from './Artwork.svelte';

  let {
    game,
    meta,
  }: {
    game: Game;
    meta?: string;
  } = $props();

  // svelte-ignore state_referenced_locally
  let favorite = $state(game.favorite);
</script>

<div class="card">
  <div class="cover-wrap">
    <button class="cover" onclick={() => navigate('game', { id: game.id })} aria-label={game.title}>
      <Artwork src={game.cover} alt={game.title} ratio="3 / 4" radius="var(--radius-md)" />
      <span class="fade"></span>
    </button>
    {#if game.installed}
      <button class="play" aria-label="Играть" onclick={() => toast(`Запуск «${game.title}»...`)}>
        <Play size="1.4rem" strokeWidth={2} fill="currentColor" />
      </button>
    {/if}
    <button class="fav" class:on={favorite} aria-label="В избранное" onclick={() => (favorite = !favorite)}>
      <Star size="1.4rem" strokeWidth={2} fill={favorite ? 'currentColor' : 'none'} />
    </button>
  </div>
  <button class="info" onclick={() => navigate('game', { id: game.id })}>
    <span class="title">{game.title}</span>
    {#if meta}
      <span class="meta">{meta}</span>
    {/if}
  </button>
</div>

<style>
  .card {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    min-width: 0;
  }

  .cover-wrap {
    position: relative;
    border-radius: var(--radius-md);
  }

  .cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--surface-3);
    transition: transform var(--dur) var(--ease);
  }

  .cover-wrap:hover .cover {
    transform: scale(1.01);
  }

  .fade {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, transparent 55%, rgba(5, 8, 12, 0.6));
    opacity: 0;
    transition: opacity var(--dur) var(--ease);
    pointer-events: none;
  }

  .cover-wrap:hover .fade {
    opacity: 1;
  }

  .play {
    position: absolute;
    left: 1rem;
    bottom: 1rem;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.2rem;
    height: 3.2rem;
    border-radius: var(--cut) var(--radius-md) var(--radius-md) var(--radius-md);
    background: var(--accent);
    color: #fff;
    opacity: 0;
    transform: translateY(0.3rem);
    transition:
      opacity var(--dur) var(--ease),
      transform var(--dur) var(--ease),
      background var(--dur) var(--ease);
  }

  .play:hover {
    background: var(--accent-hover);
  }

  .cover-wrap:hover .play,
  .play:focus-visible {
    opacity: 1;
    transform: translateY(0);
  }

  .fav {
    position: absolute;
    top: 0.8rem;
    right: 0.8rem;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.8rem;
    height: 2.8rem;
    border-radius: var(--radius-sm);
    background: rgba(6, 9, 13, 0.6);
    color: var(--text-2);
    opacity: 0;
    transition:
      opacity var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .cover-wrap:hover .fav,
  .fav:focus-visible,
  .fav.on {
    opacity: 1;
  }

  .fav.on {
    color: #e3c26b;
  }

  .info {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.2rem;
    min-width: 0;
    text-align: left;
  }

  .title {
    width: 100%;
    font-size: var(--font-md);
    font-weight: 600;
    line-height: 1.3;
    letter-spacing: var(--tracking-heading);
    color: var(--text);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .meta {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }
</style>
