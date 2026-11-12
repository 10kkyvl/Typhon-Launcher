<script lang="ts">
  import { ExternalLink } from '@lucide/svelte';
  import { t, type MessageKey } from '../i18n';
  import type { ReferenceKind } from '../services/metadata';
  import Chip from './Chip.svelte';

  let { kinds, onopen }: { kinds: ReferenceKind[]; onopen: (kind: ReferenceKind) => void } = $props();

  const LABELS: Partial<Record<ReferenceKind, MessageKey>> = {
    pcgamingwiki: 'games.detailReferencePcgw',
    howlongtobeat: 'games.detailReferenceHltb',
  };
</script>

{#snippet mark()}
  <ExternalLink />
{/snippet}

<!-- The pages have no address here: the launcher builds each one and opens it, so they are chip buttons. -->
{#if kinds.length > 0}
  <div class="links">
    <span class="title">{$t('games.detailReferenceHeading')}</span>
    <div class="list">
      {#each kinds as kind (kind)}
        {@const label = LABELS[kind]}
        {#if label}
          <Chip onclick={() => onopen(kind)} end={mark}>{$t(label)}</Chip>
        {/if}
      {/each}
    </div>
  </div>
{/if}

<style>
  .links {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }

  .title {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .list {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
