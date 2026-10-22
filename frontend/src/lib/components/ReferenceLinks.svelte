<script lang="ts">
  import { ExternalLink } from '@lucide/svelte';
  import { t, type MessageKey } from '../i18n';
  import type { ReferenceKind } from '../services/metadata';

  let { kinds, onopen }: { kinds: ReferenceKind[]; onopen: (kind: ReferenceKind) => void } = $props();

  const LABELS: Partial<Record<ReferenceKind, MessageKey>> = {
    pcgamingwiki: 'games.detailReferencePcgw',
    howlongtobeat: 'games.detailReferenceHltb',
  };
</script>

{#if kinds.length > 0}
  <div class="links">
    <span class="title">{$t('games.detailReferenceHeading')}</span>
    <div class="list">
      {#each kinds as kind (kind)}
        {@const label = LABELS[kind]}
        {#if label}
          <button class="link" onclick={() => onopen(kind)}>
            {$t(label)}
            <ExternalLink size="1.4rem" strokeWidth={1.8} />
          </button>
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

  .link {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: 2.8rem;
    padding: 0 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-2);
    font-size: var(--font-xs);
    font-weight: 500;
    transition:
      color var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .link:hover {
    color: var(--text);
    border-color: var(--border-strong);
  }
</style>
