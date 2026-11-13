<script lang="ts">
  import { ChevronRight } from '@lucide/svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { msg } from '../../lib/i18n';
  import { playtime, relativeDate } from '../../lib/utils/format';
  import type { Entry } from './entry';

  let {
    entries,
    running,
    onshowall,
    onplay,
  }: {
    entries: Entry[];
    running: ReadonlySet<string>;
    onshowall: () => void;
    onplay: (id: string) => void;
  } = $props();

  function recentMeta(entry: Entry) {
    return `${playtime(entry.playtimeSeconds)} · ${relativeDate(entry.lastPlayed)}`;
  }
</script>

<section class="section">
  <div class="section-head">
    <h2>{msg('games.recentLabel')}</h2>
    <span class="show-all">
      <TextLink tone="muted" onclick={onshowall}>
        {msg('games.libraryShowAllButton')}
        <ChevronRight size="1.4rem" strokeWidth={1.8} aria-hidden="true" />
      </TextLink>
    </span>
  </div>
  <div class="recent-row">
    {#each entries as entry (entry.id)}
      <div class="recent-item">
        <GameCard
          id={entry.id}
          title={entry.title}
          cover={entry.hero || entry.cover}
          variant="capsule"
          ambient
          installed={entry.installed}
          running={running.has(entry.id)}
          source={entry.source}
          platform={entry.platform}
          onplay={entry.playable ? () => onplay(entry.id) : undefined}
        >
          {#snippet footer()}
            <span class="recent-meta">
              <span class="dot"></span>
              {recentMeta(entry)}
            </span>
          {/snippet}
        </GameCard>
      </div>
    {/each}
  </div>
</section>

<style>
  .section {
    margin-bottom: var(--space-8);
  }

  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .section-head h2 {
    font-size: var(--font-xl);
  }

  .show-all {
    font-size: var(--font-sm);
  }

  .recent-row {
    display: flex;
    gap: var(--space-4);
    overflow-x: auto;
    margin: calc(-1 * var(--space-2)) calc(-1 * var(--space-2)) 0;
    padding: var(--space-2) var(--space-2) var(--space-3);
  }

  .recent-item {
    flex: 0 0 auto;
    width: 27rem;
  }

  .recent-meta {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
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
