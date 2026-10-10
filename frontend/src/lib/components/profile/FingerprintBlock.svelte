<script lang="ts">
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
  }: { genres: GenreShare[]; hours: number; games: number; completed: number; accent?: string } = $props();
</script>

<Card title={msg('profile.blockFingerprint')}>
  <div class="emblem">
    <Fingerprint {genres} {hours} {games} {completed} {accent} />
  </div>
  <div class="facts">
    <div><span class="num">{formatCount(hours)}</span><span class="label">{msg('social.statHours')}</span></div>
    <div><span class="num">{formatCount(games)}</span><span class="label">{msg('social.statGames')}</span></div>
    <div><span class="num">{formatCount(completed)}</span><span class="label">{msg('social.statCompleted')}</span></div>
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
