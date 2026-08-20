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
      <Artwork src={game.cover} alt={game.title} ratio="3 / 4" radius="var(--radius-lg)" />
      <span class="fade"></span>
    </button>
    {#if game.installed}
      <button class="play" aria-label="Играть" onclick={() => toast(`Запуск «${game.title}»...`)}>
        <Play size={15} strokeWidth={2} fill="currentColor" />
      </button>
    {/if}
    <button class="fav" class:on={favorite} aria-label="В избранное" onclick={() => (favorite = !favorite)}>
      <Star size={14} strokeWidth={2} fill={favorite ? 'currentColor' : 'none'} />
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
    gap: 10px;
    min-width: 0;
  }

  .cover-wrap {
    position: relative;
    border-radius: var(--radius-lg);
    transition: transform var(--dur) var(--ease);
  }

  .cover-wrap:hover {
    transform: translateY(-1px);
  }

  .cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-lg);
    overflow: hidden;
    border: 1px solid var(--border);
    transition: border-color var(--dur) var(--ease);
  }

  .cover :global(img) {
    transition: transform 300ms var(--ease);
  }

  .cover-wrap:hover .cover {
    border-color: var(--border-strong);
  }

  .cover-wrap:hover .cover :global(img) {
    transform: scale(1.015);
  }

  .fade {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, transparent 55%, rgba(5, 8, 12, 0.55));
    opacity: 0;
    transition: opacity var(--dur) var(--ease);
    pointer-events: none;
  }

  .cover-wrap:hover .fade {
    opacity: 1;
  }

  .play {
    position: absolute;
    left: 10px;
    bottom: 10px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    background: var(--accent);
    color: #fff;
    opacity: 0;
    transform: translateY(4px);
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
    top: 8px;
    right: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 8px;
    background: rgba(6, 9, 13, 0.65);
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
    color: #e8c35a;
  }

  .info {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    min-width: 0;
    text-align: left;
  }

  .title {
    width: 100%;
    font-size: 14px;
    font-weight: 550;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .meta {
    font-size: 12.5px;
    color: var(--text-3);
  }
</style>
