<script lang="ts">
  import type { Snippet } from 'svelte';
  import Card from '../Card.svelte';
  import { msg } from '../../i18n';
  import type { GenreShare } from '../../services/social';
  import { formatCount } from '../../utils/format';
  import Fingerprint from './Fingerprint.svelte';

  let {
    genres,
    hours,
    games,
    completed,
    accent,
    masked,
  }: {
    genres: GenreShare[];
    hours: number | null;
    games: number | null;
    completed: number | null;
    accent?: string;
    masked?: Snippet;
  } = $props();

  const facts = $derived([
    { value: hours, label: msg('social.statHours') },
    { value: games, label: msg('social.statGames') },
    { value: completed, label: msg('social.statCompleted') },
  ]);
</script>

<Card title={msg('profile.blockFingerprint')}>
  <div class="emblem">
    <Fingerprint {genres} hours={hours ?? 0} games={games ?? 0} completed={completed ?? 0} {accent} />
  </div>
  <div class="facts">
    {#each facts as fact (fact.label)}
      <div>
        <span class="num">
          {#if fact.value !== null}{formatCount(fact.value)}{:else if masked}{@render masked()}{:else}—{/if}
        </span>
        <span class="label">{fact.label}</span>
      </div>
    {/each}
  </div>
</Card>

<style>
  .emblem {
    max-width: 18rem;
    margin: 0 auto var(--space-4);
  }

  .facts {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    margin: 0;
  }

  .facts div {
    display: flex;
    flex-direction: column;
    align-items: center;
    min-width: 0;
    gap: 0.2rem;
  }

  .num {
    font-size: var(--font-lg);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
