<script lang="ts">
  import { ExternalLink } from '@lucide/svelte';
  import { t, type MessageKey } from '../i18n';
  import { storeEntries, type StoreId } from '../services/metadata';

  let {
    links,
    onopen,
    focusPrefix,
  }: {
    links: Record<string, string> | null | undefined;
    onopen: (store: StoreId) => void;
    focusPrefix?: string;
  } = $props();

  const LABELS: Record<StoreId, MessageKey> = {
    steam: 'games.storeSteam',
    gog: 'games.storeGog',
    epic: 'games.storeEpic',
  };

  const stores = $derived(storeEntries(links));
</script>

{#if stores.length > 0}
  <div class="store-links">
    <span class="store-title">{$t('games.storeLinksTitle')}</span>
    <div class="store-list">
      {#each stores as store (store)}
        <button
          class="store-link"
          data-bp-focus={focusPrefix ? `${focusPrefix}${store}` : undefined}
          onclick={() => onopen(store)}
        >
          {$t(LABELS[store])}
          <ExternalLink size="1.4rem" strokeWidth={1.8} />
        </button>
      {/each}
    </div>
  </div>
{/if}

<style>
  .store-links {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .store-title {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .store-list {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .store-link {
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

  .store-link:hover {
    color: var(--text);
    border-color: var(--border-strong);
  }
</style>
