<script lang="ts">
  import { Play, Square } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import type { LibraryGame } from '../../lib/services/library';
  import { runningGames } from '../../lib/stores/library';
  import { emulationPlatform, isEmulated } from '../../lib/library/emulation';
  import { missingImages } from '../../lib/stores/missingImages';
  import { isLaunchable } from '../../lib/library/external';
  import { gameArt } from '../../lib/stores/metadata';
  import { gameContextMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { updatesByGame } from '../../lib/stores/updates';
  import { msg } from '../../lib/i18n';
  import { coverFor, play, sizeLabel, stop, updateLabel } from './installedGame';

  // The grid view: one cover per game with Play or Stop over it. `uid` is the page's id prefix: a
  // cover's button is described by its card's title through an id made from it.
  let { games, uid }: { games: LibraryGame[]; uid: string } = $props();
</script>

<div class="grid">
  {#each games as game (game.id)}
    {@const running = $runningGames.has(game.id)}
    {@const update = $updatesByGame.get(game.id)?.availability}
    {@const updateText = updateLabel(update)}
    <div class="card" role="presentation" data-game-row={game.id} {@attach gameContextMenu(game.id)}>
      <div class="card-media">
        <button
          class="card-cover"
          onclick={() => navigate('game', { id: game.id })}
          aria-label={updateText ? `${game.title} — ${updateText}` : game.title}
        >
          <Artwork src={coverFor(game, $gameArt)} alt="" label={game.title} ratio="3 / 4" radius="var(--radius-md)" />
          {#if updateText}
            <span class="card-badge">
              <StatusBadge kind={update?.kind === 'update' ? 'warning' : 'neutral'} label={updateText} />
            </span>
          {/if}
        </button>
        {#if running}
          <span class="card-play running">
            <IconButton variant="primary" size="sm" label={msg('games.installedStopButton')} aria-describedby="{uid}-{game.id}-title" onclick={() => stop(game)}>
              <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
            </IconButton>
          </span>
        {:else if isLaunchable(game) && !$missingImages.has(game.id)}
          <span class="card-play">
            <IconButton variant="primary" size="sm" label={msg('games.play')} aria-describedby="{uid}-{game.id}-title" onclick={() => play(game)}>
              <Play size="1.4rem" strokeWidth={2} fill="currentColor" />
            </IconButton>
          </span>
        {/if}
      </div>
      <div class="card-info">
        <span class="card-title" id="{uid}-{game.id}-title">{game.title}</span>
        <span class="card-meta">
          {#if isEmulated(game)}<ConsoleTag platform={emulationPlatform(game)} />{/if}
          {sizeLabel(game)}
          {#if $missingImages.has(game.id)}<StatusBadge kind="danger" label={msg('emul.imageMissing')} plain />{/if}
        </span>
      </div>
    </div>
  {/each}
</div>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--space-6) var(--space-5);
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    min-width: 0;
  }

  .card-media {
    position: relative;
    border-radius: var(--radius-md);
    transition: transform var(--dur-panel) var(--ease);
  }

  .card-media::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    top: 100%;
    height: 0.4rem;
  }

  .card-media:hover,
  .card-media:focus-within {
    transform: translateY(-0.4rem);
  }

  .card-cover {
    position: relative;
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    transition: box-shadow var(--dur-panel) var(--ease);
  }

  .card-cover :global(img) {
    transition: transform var(--dur-slow) var(--ease);
  }

  .card-media:hover .card-cover {
    box-shadow: var(--shadow-lift);
  }

  .card-media:hover .card-cover :global(img) {
    transform: scale(1.04);
  }

  .card-badge {
    position: absolute;
    top: 0.6rem;
    left: 0.6rem;
    display: flex;
    max-width: calc(100% - 1.2rem);
    overflow: hidden;
    transform-origin: left top;
    animation: badge-pop var(--dur-slow) var(--ease-spring) backwards;
  }

  @keyframes badge-pop {
    from {
      opacity: 0;
      transform: scale(0.6);
    }
  }

  .card-play {
    position: absolute;
    left: 1rem;
    bottom: 1rem;
    display: flex;
    opacity: 0;
    transform: translateY(0.4rem) scale(0.9);
    transition:
      opacity var(--dur) var(--ease),
      transform var(--dur-panel) var(--ease-spring);
  }

  .card-media:hover .card-play,
  .card-play:focus-within,
  .card-play.running {
    opacity: 1;
    transform: translateY(0) scale(1);
  }

  .card-info {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .card-title {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    line-height: 1.3;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .card-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  @media (min-width: 2200px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    }
  }

  @media (max-width: 1400px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(14.5rem, 1fr));
    }
  }
</style>
