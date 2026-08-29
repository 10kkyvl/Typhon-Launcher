<script lang="ts">
  import { Play, Square } from '@lucide/svelte';
  import { openGameMenu } from '../stores/gameMenu';
  import { navigate } from '../stores/router';
  import Artwork from './Artwork.svelte';

  let {
    id,
    title,
    cover = '',
    installed = false,
    running = false,
    meta,
    onplay,
  }: {
    id: string;
    title: string;
    cover?: string;
    installed?: boolean;
    running?: boolean;
    meta?: string;
    onplay?: () => void;
  } = $props();
</script>

<div class="card" role="presentation" oncontextmenu={(event) => openGameMenu(event, id)}>
  <div class="cover-wrap">
    <button class="cover" onclick={() => navigate('game', { id })} aria-label={title}>
      <Artwork src={cover} alt={title} ratio="3 / 4" radius="var(--radius-md)" />
      <span class="fade"></span>
    </button>
    {#if installed && onplay}
      <button class="play" class:running aria-label={running ? 'Остановить' : 'Играть'} onclick={onplay}>
        {#if running}
          <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
        {:else}
          <Play size="1.4rem" strokeWidth={2} fill="currentColor" />
        {/if}
      </button>
    {/if}
  </div>
  <button class="info" onclick={() => navigate('game', { id })}>
    <span class="title">{title}</span>
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
  .play:focus-visible,
  .play.running {
    opacity: 1;
    transform: translateY(0);
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
