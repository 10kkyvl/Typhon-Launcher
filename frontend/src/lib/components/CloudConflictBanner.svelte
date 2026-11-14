<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import { msg } from '../i18n';
  import { cloudStatusLoading, openCloudConflict, showConflictOnPage, unsettledConflicts } from '../stores/cloudsaves';
  import { focusHandOff } from '../utils/focusHandOff';
  import { dateTime } from '../utils/format';

  /**
   * The game page's word on a save conflict of its game: one line with when each copy changed (and
   * where the cloud one did) and one action that opens the conflict dialog. Shown while the conflict is
   * unsettled; it goes as soon as a choice is applied. A conflict reported by an event comes without its
   * details and the status loads again for them: the banner waits for that load, so it comes once, with
   * its words. While it shows, the island and the sidebar's status line leave this game to it
   * (`showConflictOnPage`). When it goes with focus on it (or in the dialog it opened), focus moves to
   * `fallbackFocus`, the page's main heading; a dialog closed without a choice gives focus back to the
   * action.
   */
  let {
    gameId,
    fallbackFocus,
  }: {
    /** The library game; none for a page the library does not hold. */
    gameId?: string;
    fallbackFocus?: () => HTMLElement | null | undefined;
  } = $props();

  const listed = $derived(gameId ? $unsettledConflicts.has(gameId) : false);
  const details = $derived(gameId ? $unsettledConflicts.get(gameId) : undefined);
  const waiting = $derived(listed && !details && $cloudStatusLoading);

  // The game the banner is up for: once up it stays while the conflict is listed, whatever loads.
  let shownFor = $state('');
  let box = $state<HTMLElement>();

  $effect.pre(() => {
    const id = gameId ?? '';
    const up = listed && (untrack(() => shownFor) === id || !waiting);
    const next = up ? id : '';
    untrack(() => {
      if (next === shownFor) return;
      if (!next && shownFor) void focusHandOff(() => box)(() => fallbackFocus?.());
      shownFor = next;
    });
  });

  const shown = $derived(shownFor !== '' && shownFor === gameId);

  $effect(() => {
    if (shown && gameId) return showConflictOnPage(gameId);
  });

  function when(iso: string): string {
    const value = new Date(iso);
    return Number.isNaN(value.getTime()) ? iso : dateTime(value);
  }

  const line = $derived.by(() => {
    if (!details) return msg('cloudsaves.conflictTitle');
    const params = { local: when(details.localUpdatedAt), remote: when(details.remoteUpdatedAt) };
    return details.remoteDevice
      ? msg('cloudsaves.bannerWhenDevice', { ...params, device: details.remoteDevice })
      : msg('cloudsaves.bannerWhen', params);
  });

  function open() {
    if (gameId) openCloudConflict(gameId);
  }
</script>

{#if shown}
  <div class="banner" bind:this={box}>
    <div class="fold">
      <Notice tone="warning">
        {line}
        {#snippet action()}
          <Button size="sm" variant="primary" onclick={open}>{msg('cloudsaves.resolve')}</Button>
        {/snippet}
      </Notice>
    </div>
  </div>
{/if}

<style>
  /* It opens from no height, so what is under it moves down instead of jumping; reduced motion and the
     animations setting cut the transition (styles/global.css). */
  .banner {
    display: grid;
    grid-template-rows: 1fr;
    transition: grid-template-rows var(--dur-panel) var(--ease);
  }

  @starting-style {
    .banner {
      grid-template-rows: 0fr;
    }
  }

  .fold {
    min-height: 0;
    overflow: hidden;
  }
</style>
