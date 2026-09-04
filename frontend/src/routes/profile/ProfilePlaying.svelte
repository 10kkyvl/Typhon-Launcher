<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import Card from '../../lib/components/Card.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import type { GameRef } from '../../lib/services/profile';
  import { navigate } from '../../lib/stores/router';
  import HiddenBadge from './HiddenBadge.svelte';

  let { running, hidden }: { running: GameRef[]; hidden: boolean } = $props();

  const game = $derived(running[0] ?? null);
</script>

{#if game}
  <Card title="Сейчас играет">
    {#snippet action()}
      {#if hidden}<HiddenBadge text="Скрыто от других. Вы видите это, остальные — нет." />{/if}
    {/snippet}
    <button class="playing" type="button" onclick={() => navigate('game', { id: game.id })}>
      <span class="cover">
        <Artwork src={game.cover} alt={game.title} ratio="16 / 9" radius="var(--radius-md)" />
      </span>
      <span class="title">{game.title}</span>
      <StatusBadge kind="success" label="Играет" plain />
    </button>
  </Card>
{/if}

<style>
  .playing {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.8rem;
    width: 100%;
    padding: 0;
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    transition: transform var(--dur) var(--ease);
  }

  .playing:hover .cover {
    transform: scale(1.01);
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    line-height: 1.3;
  }
</style>
