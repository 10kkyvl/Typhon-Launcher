<script lang="ts">
  import { ExternalLink } from '@lucide/svelte';
  import { t, type MessageKey } from '../i18n';
  import { storeEntries, type StoreId } from '../services/metadata';
  import Chip from './Chip.svelte';

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

  // A web page is a real link (what a link offers, its address on hover); the launcher still opens
  // it, in the system browser, after its own checks. Any other address is never put in an href.
  function webPage(store: StoreId): string | undefined {
    const address = links?.[store] ?? '';
    return /^https?:\/\//i.test(address) ? address : undefined;
  }

  function open(event: MouseEvent, store: StoreId) {
    event.preventDefault();
    onopen(store);
  }

  // A middle click would open the page in a webview window of its own, past the launcher's checks:
  // it opens through the launcher like any other press.
  function openAux(event: MouseEvent, store: StoreId) {
    if (event.button !== 1) return;
    open(event, store);
  }
</script>

{#snippet mark()}
  <ExternalLink />
{/snippet}

{#if stores.length > 0}
  <div class="store-links">
    <span class="store-title">{$t('games.storeLinksTitle')}</span>
    <div class="store-list">
      {#each stores as store (store)}
        {@const href = webPage(store)}
        {@const focus = focusPrefix ? `${focusPrefix}${store}` : undefined}
        {#if href}
          <Chip {href} data-bp-focus={focus} onclick={(event) => open(event, store)} onauxclick={(event) => openAux(event, store)} end={mark}>{$t(LABELS[store])}</Chip>
        {:else}
          <Chip data-bp-focus={focus} onclick={(event) => open(event, store)} end={mark}>{$t(LABELS[store])}</Chip>
        {/if}
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
</style>
