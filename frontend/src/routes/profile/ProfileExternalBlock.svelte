<script lang="ts">
  import Card from '../../lib/components/Card.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import AboutBlock from '../../lib/components/profile/AboutBlock.svelte';
  import { msg } from '../../lib/i18n';
  import { coverOf } from '../../lib/profile/view';
  import type { LibraryGame } from '../../lib/services/library';
  import type { GameArt } from '../../lib/services/metadata';
  import type { GameRef } from '../../lib/services/profile';
  import { profileSnapshot } from '../../lib/stores/profile';
  import { navigate } from '../../lib/stores/router';
  import { playtime, relativeDate } from '../../lib/utils/format';
  import ProfileActivity from './ProfileActivity.svelte';
  import ProfilePlaying from './ProfilePlaying.svelte';
  import ProfileStats from './ProfileStats.svelte';

  // The body of a block BlockGrid leaves to the page. The snapshot is read from its store, not taken as a prop:
  // a list from a prop compiles to immutable each items, so a snapshot set again with the same objects would no
  // longer re-run the attachments of the recent row's cards.
  interface Props {
    type: string;
    playingNow: GameRef | null;
    hiddenTypes: Set<string>;
    art: Record<string, GameArt>;
    library: LibraryGame[];
    bio: string;
  }

  const { type, playingNow, hiddenTypes, art, library, bio }: Props = $props();

  function lastPlayedOf(id: string): string | null {
    return library.find((game) => game.id === id)?.lastPlayed ?? null;
  }
</script>

{#if type === 'playing' && playingNow}
  <ProfilePlaying
    title={playingNow.title}
    art={coverOf(playingNow, art)}
    hidden={hiddenTypes.has('playing')}
    disabled={playingNow.archived}
    onopen={() => navigate('game', { id: playingNow.id })}
  />
{:else if type === 'recent'}
  <Card title={msg('social.recentlyPlayedTitle')}>
    <div class="recent-row">
      {#each $profileSnapshot.playing as entry (entry.game.id)}
        <div class="recent-item">
          <GameCard id={entry.game.id} title={entry.game.title} cover={coverOf(entry.game, art)} variant="capsule">
            {#snippet footer()}
              <span class="recent-meta">
                <span class="dot"></span>
                {playtime(entry.game.playtimeSeconds)} · {relativeDate(lastPlayedOf(entry.game.id))}
              </span>
            {/snippet}
          </GameCard>
        </div>
      {/each}
    </div>
  </Card>
{:else if type === 'activity'}
  <ProfileActivity days={$profileSnapshot.activity} hidden={hiddenTypes.has('activity')} />
{:else if type === 'stats'}
  <Card title={msg('profile.blockStats')}>
    <ProfileStats stats={$profileSnapshot.stats} hidden={hiddenTypes.has('stats')} />
  </Card>
{:else if type === 'about'}
  <AboutBlock {bio} />
{/if}

<style>
  .recent-row {
    display: flex;
    gap: var(--space-4);
    overflow-x: auto;
    margin: calc(-1 * var(--space-2)) calc(-1 * var(--space-2)) 0;
    padding: var(--space-2) var(--space-2) var(--space-3);
  }

  .recent-item {
    flex: 0 0 auto;
    width: 22rem;
  }

  .recent-meta {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .recent-meta .dot {
    width: 0.7rem;
    height: 0.7rem;
    border-radius: 50%;
    background: var(--success);
    flex-shrink: 0;
  }
</style>
