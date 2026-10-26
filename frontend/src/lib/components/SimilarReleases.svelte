<script lang="ts">
  import { untrack } from 'svelte';
  import { Link2 } from '@lucide/svelte';
  import { candidateLabel, relativeCandidate } from '../game/similar';
  import { confirmMatch, similarReleases, type SimilarRelease } from '../services/sources';
  import { errorMessage } from '../stores/sources';
  import { toast } from '../stores/toasts';
  import { metadataErrorText } from '../metadata/metadataErrors';
  import { bytesSize, relativeDate } from '../utils/format';
  import { msg } from '../i18n';
  import Button from './Button.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let {
    gameId,
    familyIds,
    onlinked,
  }: {
    gameId: string;
    familyIds: string[];
    onlinked?: () => void;
  } = $props();

  let items = $state<SimilarRelease[]>([]);
  let failed = $state(false);
  let linking = $state<string | null>(null);
  let token = 0;

  const familyKey = $derived(familyIds.join('|'));

  async function load(id: string, family: string[]) {
    const current = ++token;
    failed = false;
    try {
      const list = await similarReleases(id, family);
      if (current !== token) return;
      items = list;
    } catch {
      if (current !== token) return;
      items = [];
      failed = true;
    }
  }

  $effect(() => {
    const id = gameId;
    familyKey;
    untrack(() => load(id, familyIds));
  });

  async function link(item: SimilarRelease) {
    linking = item.release.id;
    try {
      await confirmMatch(item.release.id, gameId);
      toast(msg('games.similarLinked'), 'success');
      onlinked?.();
      await load(gameId, familyIds);
    } catch (err) {
      toast(metadataErrorText(err, errorMessage(err)), 'danger');
    } finally {
      linking = null;
    }
  }
</script>

{#if failed || items.length > 0}
  <section class="section">
    <h2 class="heading">{msg('games.similarHeading')}</h2>
    {#if failed}
      <p class="muted">{msg('games.similarFailed')}</p>
      <Button onclick={() => load(gameId, familyIds)}>{msg('games.catalogRetry')}</Button>
    {:else}
      <p class="muted">{msg('games.similarHint')}</p>
      <div class="list">
        {#each items as item (item.release.id)}
          {@const note = relativeCandidate(item, gameId)}
          <div class="row">
            <div class="main">
              <span class="title" title={item.release.rawTitle}>{item.release.title || item.release.rawTitle}</span>
              {#if note}
                <span class="note">{msg('games.similarCandidate', { title: candidateLabel(note) })}</span>
              {/if}
            </div>
            <span class="size">{bytesSize(item.release.size)}</span>
            <span class="source">{item.sourceName}</span>
            <span class="date">{relativeDate(item.release.uploadedAt)}</span>
            {#if item.release.matchStatus === 'review'}
              <StatusBadge kind="warning" label={msg('modals.sourceDetailsStatusReview')} plain />
            {/if}
            <Button size="sm" disabled={linking !== null} onclick={() => link(item)}>
              <Link2 size="1.4rem" strokeWidth={1.8} />
              {msg('games.similarLink')}
            </Button>
          </div>
        {/each}
      </div>
    {/if}
  </section>
{/if}

<style>
  .section {
    margin-top: var(--space-10);
  }

  .heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
    padding: 0.4rem 0;
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 1.1rem 0;
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    flex: 1.3;
  }

  .title {
    font-size: var(--font-sm);
    overflow-wrap: anywhere;
  }

  .note {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .size,
  .source,
  .date {
    font-size: var(--font-xs);
    color: var(--text-2);
    white-space: nowrap;
  }
</style>
