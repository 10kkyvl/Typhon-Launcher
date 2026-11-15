<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import { msg } from '../../lib/i18n';
  import type { CatalogPageData } from './catalogPageData.svelte';

  // What the page says about itself above the list: the catalog offline or failed, its coverage, a game
  // just put away with its undo, the picks that fell back or went stale, the profile with no history.
  let { data }: { data: CatalogPageData } = $props();
</script>

{#if data.pages.offline || data.pageFailed || data.pageRetrying}
  <div class="page-note">
    <Notice
      tone={data.pageFailed ? 'danger' : 'warning'}
      title={data.pageFailed ? msg('games.catalogUnavailableTitle') : undefined}
      level={data.pageFailed ? 2 : undefined}
      bind:retrying={data.pageRetrying}
      retry={{ label: msg('games.catalogRetry'), run: data.retryFirstPage, within: () => data.results }}
    >
      {#if !data.pageFailed}
        {msg('games.catalogOffline')}
      {:else if data.pages.backendOutdated}
        {msg('games.catalogBackendOutdated')}
      {:else}
        {msg('games.catalogUnavailableDescription')}
      {/if}
    </Notice>
  </div>
{:else if data.pages.incomplete}<details class="muted"><summary>{msg('games.catalogCoverage')}</summary><p>{msg('games.catalogIncomplete')}</p></details>{/if}

{#if data.lastDismissed}
  <div class="page-note">
    <Notice tone="success">
      {msg('games.recommendationDismissed', { title: data.lastDismissed.title })}
      {#snippet action()}<Button size="sm" onclick={data.undoDismissal} busy={data.undoing} disabled={data.preferenceBusy && !data.undoing}>{msg('games.recommendationUndo')}</Button>{/snippet}
    </Notice>
  </div>
{/if}
<span bind:this={data.dismissalEnd} hidden></span>
{#if data.personalizationFallback}<div class="page-note"><Notice tone="info" compact>{msg('games.recommendationFallback')}</Notice></div>{/if}
{#if data.discoveryStale}<div class="page-note"><Notice tone="warning" compact>{msg('games.recommendationRefreshFailed')}</Notice></div>{/if}
{#if data.profile.confidence < 1 && (data.sort === 'auto' || data.sort === 'for-you')}<p class="muted profile-hint">{msg('games.recommendationNoHistory')}</p>{/if}

<style>
  .page-note { margin: var(--space-4) 0; }
  .profile-hint { margin-bottom: var(--space-5); }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
