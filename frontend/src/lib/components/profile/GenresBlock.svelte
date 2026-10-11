<script lang="ts">
  import Card from '../Card.svelte';
  import { msg } from '../../i18n';
  import type { GenreBreakdown } from '../../services/social';

  let { breakdown }: { breakdown: GenreBreakdown } = $props();

  interface Segment {
    id: string;
    name: string;
    share: number;
    tone: number | 'other' | 'unknown';
  }

  const segments = $derived.by(() => {
    const items: Segment[] = breakdown.genres.map((genre, index) => ({
      id: `g${index}`,
      name: genre.name,
      share: genre.share,
      tone: index,
    }));
    items.push({ id: 'other', name: msg('profile.genresOther'), share: breakdown.other, tone: 'other' });
    items.push({ id: 'unknown', name: msg('profile.genresUnknown'), share: breakdown.unknown, tone: 'unknown' });
    return items.filter((item) => item.share > 0);
  });

  function percent(share: number): string {
    const value = share * 100;
    return value < 1 ? '<1%' : `${Math.round(value)}%`;
  }
</script>

<Card title={msg('profile.blockGenres')}>
  {#if segments.length === 0}
    <p class="empty">{msg('profile.genresEmpty')}</p>
  {:else}
    <div class="bar" role="img" aria-label={segments.map((item) => `${item.name} ${percent(item.share)}`).join(', ')}>
      {#each segments as item (item.id)}
        <span class="seg tone-{item.tone}" style:flex="{item.share} 1 0"></span>
      {/each}
    </div>
    <ul class="legend">
      {#each segments as item (item.id)}
        <li>
          <span class="swatch tone-{item.tone}"></span>
          <span class="name" class:muted={typeof item.tone === 'string'}>{item.name}</span>
          <span class="value">{percent(item.share)}</span>
        </li>
      {/each}
    </ul>
  {/if}
</Card>

<style>
  .empty {
    padding: var(--space-4) 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .bar {
    display: flex;
    gap: 2px;
    height: 1.6rem;
    overflow: hidden;
    border-radius: var(--radius-sm);
  }

  .seg {
    min-width: 2px;
  }

  .tone-0 { background: var(--accent); }
  .tone-1 { background: color-mix(in srgb, var(--accent) 80%, var(--surface-3)); }
  .tone-2 { background: color-mix(in srgb, var(--accent) 62%, var(--surface-3)); }
  .tone-3 { background: color-mix(in srgb, var(--accent) 46%, var(--surface-3)); }
  .tone-4 { background: color-mix(in srgb, var(--accent) 32%, var(--surface-3)); }
  .tone-other { background: var(--text-3); }
  .tone-unknown {
    background: repeating-linear-gradient(135deg, var(--surface-4) 0 0.4rem, var(--surface-3) 0.4rem 0.8rem);
  }

  .legend {
    list-style: none;
    margin: var(--space-4) 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .legend li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    font-size: var(--font-sm);
  }

  .swatch {
    flex-shrink: 0;
    width: 1rem;
    height: 1rem;
    border-radius: 50%;
  }

  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-2);
  }

  .name.muted {
    color: var(--text-3);
  }

  .value {
    flex-shrink: 0;
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }
</style>
