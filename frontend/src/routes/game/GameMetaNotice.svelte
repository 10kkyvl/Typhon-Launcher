<script lang="ts">
  import { tick } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import type { GamePageData } from './gamePageData.svelte';
  import { msg } from '../../lib/i18n';
  import { focusableIn, focusQuietly } from '../../lib/layers/focus';

  /** The metadata notices of the overview: a search running, or no match, or a search that failed. */
  let {
    data,
    title,
    titleHeading,
    openMatch,
  }: {
    data: GamePageData;
    title: string;
    titleHeading: HTMLElement | undefined;
    openMatch: (mode: 'find' | 'change') => void;
  } = $props();

  let chooseSlot = $state<HTMLElement>();

  async function retryMeta(): Promise<boolean> {
    const ok = await data.refreshMeta(true);
    if (!ok && !data.metaRetryable) {
      await tick();
      const active = document.activeElement;
      if (!active || active === document.body || !active.isConnected) {
        const choose = chooseSlot ? focusableIn(chooseSlot)[0] : undefined;
        if (choose) focusQuietly(choose);
      }
    }
    return ok;
  }
</script>

{#if data.metaState === 'searching'}
  <div class="meta-note">
    <Notice tone="info" title={msg('games.detailMetaSearchingTitle')}>
      {#snippet icon()}
        <Spinner size="sm" />
      {/snippet}
      {msg('games.detailMetaSearchingHint', { title: title || msg('games.detailFallbackGameWord') })}
    </Notice>
  </div>
{:else if data.metaState === 'unmatched' || data.metaState === 'failed'}
  <div class="meta-note">
    <Notice
      tone={data.metaState === 'failed' ? 'warning' : 'info'}
      announce={data.metaSearchRan}
      title={data.metaState === 'unmatched' ? msg('games.detailMetaUnmatchedTitle') : msg('games.detailMetaFailedTitle')}
      bind:retrying={data.metaRetrying}
      retry={data.metaState === 'failed' && data.metaRetryable
        ? {
            label: msg('common.retry'),
            pending: msg('games.detailMetaRetrying'),
            run: retryMeta,
            focus: () => titleHeading,
          }
        : undefined}
    >
      {data.metaState === 'unmatched' ? msg('games.detailMetaUnmatchedHint') : data.metaFailure || msg('games.detailMetaFailedHint')}
      {#snippet action()}
        <span class="choose" bind:this={chooseSlot}>
          <Button size="sm" variant="primary" onclick={() => openMatch('find')}>{msg('games.detailChooseManually')}</Button>
        </span>
        <Button size="sm" variant="ghost" busy={data.metaSkipping} onclick={data.skipMeta}>{msg('games.detailSkipSearch')}</Button>
      {/snippet}
    </Notice>
  </div>
{/if}

<style>
  /* The page's `.main > :first-child` rule, for a notice that opens the overview. */
  :global(.main) > :first-child {
    margin-top: 0;
  }

  .meta-note {
    max-width: var(--prose-max);
    margin-bottom: var(--space-6);
  }

  .choose {
    display: contents;
  }
</style>
