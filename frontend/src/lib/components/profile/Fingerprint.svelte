<script lang="ts">
  import type { GenreShare } from '../../services/social';
  import { msg } from '../../i18n';
  import { CENTER, emblemOf } from './fingerprint';

  let {
    genres,
    hours,
    games,
    completed,
    accent = 'var(--accent)',
  }: {
    genres: GenreShare[];
    hours: number;
    games: number;
    completed: number;
    accent?: string;
  } = $props();

  const emblem = $derived(emblemOf({ genres, hours, games, completed }));
</script>

<svg class="emblem" viewBox="0 0 100 100" role="img" aria-label={msg('profile.fingerprintLabel')} style:color={accent}>
  <g transform={`rotate(${emblem.rotation} ${CENTER} ${CENTER})`}>
    {#each emblem.rings as ring, index (index)}
      <circle class="track" cx={CENTER} cy={CENTER} r={ring.r} stroke-width={ring.width} />
      {#if ring.arc}
        <path class="arc" d={ring.arc} stroke-width={ring.width} opacity={ring.opacity} />
      {/if}
    {/each}
    {#each emblem.satellites as dot, index (index)}
      <circle class="dot" cx={dot.x} cy={dot.y} r="1.5" />
    {/each}
  </g>
  <circle class="core" cx={CENTER} cy={CENTER} r={emblem.core} />
</svg>

<style>
  .emblem {
    display: block;
    width: 100%;
    height: auto;
    aspect-ratio: 1;
  }

  .track {
    fill: none;
    stroke: currentColor;
    opacity: 0.1;
  }

  .arc {
    fill: none;
    stroke: currentColor;
    stroke-linecap: round;
  }

  .dot,
  .core {
    fill: currentColor;
  }

  .core {
    opacity: 0.85;
  }
</style>
