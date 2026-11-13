<script lang="ts">
  import { untrack } from 'svelte';
  import { Download, FileCheck, FolderInput, TriangleAlert, Wrench } from '@lucide/svelte';
  import { installErrorText } from '../install/installErrors';
  import { cancelInstall } from '../services/install';
  import { activity } from '../stores/activity';
  import { downloads, pause, resume } from '../stores/downloads';
  import { installations } from '../stores/install';
  import { themeLayout } from '../stores/layout';
  import { libraryGames, runningGames, sessionStarts } from '../stores/library';
  import { moves } from '../stores/relocate';
  import { navigate, route } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { focusHandOff } from '../utils/focusHandOff';
  import { progressPercent } from '../utils/format';
  import { msg } from '../i18n';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import Popover from './Popover.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import TextLink from './TextLink.svelte';
  import { detailOf, leadOf, liveMessages, reading, shownItems, valueText, widgetItems, type Outcome, type WidgetItem } from './activityWidget';

  /**
   * What goes on for a while, in the sidebar's activity zone: downloads, installs, verifications, a
   * move, running games. The zone keeps its height whether it shows anything or not, so nothing in
   * the sidebar moves. One line names the leading item with its percent and "+N" for the rest; a press
   * opens a panel over the content with a row and the actions for each (a layer: the stack answers
   * Escape, presses outside and Tab). A live region says when an item appears, ends or needs the
   * user, never its progress.
   */

  let {
    away,
  }: {
    /** Where focus goes when the last row goes from under it and the widget empties: a control that stays. */
    away?: () => HTMLElement | null | undefined;
  } = $props();

  const all = $derived(widgetItems($activity, $runningGames, $sessionStarts, $libraryGames));
  const items = $derived(shownItems(all, $route));
  const lead = $derived(leadOf(items));
  const more = $derived(items.length - 1);

  let open = $state(false);
  let head = $state<HTMLButtonElement>();
  let body = $state<HTMLElement>();

  $effect(() => {
    if (!lead) open = false;
  });

  // The session timers tick only while a timer is shown.
  let now = $state(Date.now());
  const timed = $derived(items.some((item) => item.startedAt !== undefined));
  $effect(() => {
    if (!timed) return;
    now = Date.now();
    const timer = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(timer);
  });

  function outcome(item: WidgetItem): Outcome {
    const ended = (done: boolean, failed: boolean): Outcome => (done ? 'done' : failed ? 'failed' : null);
    if (item.kind === 'download') {
      const status = $downloads.find((d) => d.id === item.id)?.status;
      return ended(status === 'completed', status === 'failed');
    }
    if (item.kind === 'install') {
      const status = $installations.find((i) => `install:${i.id}` === item.key)?.status;
      return ended(status === 'completed', status === 'failed');
    }
    if (item.kind === 'move') {
      const stage = $moves.find((job) => job.id === item.id)?.stage;
      return ended(stage === 'done', stage === 'failed');
    }
    return null;
  }

  // Said from the whole list, whatever the page hides: a page change appears and ends nothing. What is
  // going on when the widget comes up is not news.
  let said: Map<string, WidgetItem> | null = null;
  let liveText = $state('');
  $effect(() => {
    const after = all;
    untrack(() => {
      if (said) {
        const lines = liveMessages(said, after, outcome);
        if (lines.length > 0) liveText = lines.join('. ');
      }
      said = new Map(after.map((item) => [item.key, item]));
    });
  });

  // A row that goes while focus is in it (a finished download under its focused Pause) hands focus to
  // the next row that stays, else the one before, else the head. The last row takes the head and the
  // panel with it: focus goes `away`. Focus elsewhere is left alone.
  let shownKeys: string[] = [];
  $effect.pre(() => {
    const keys = items.map((item) => item.key);
    const before = shownKeys;
    shownKeys = keys;
    const active = document.activeElement;
    const row = active instanceof HTMLElement && body?.contains(active) ? active.closest<HTMLElement>('[data-key]') : null;
    const key = row?.dataset.key;
    if (!key || keys.includes(key)) return;
    const at = before.indexOf(key);
    const next = before.slice(at + 1).find((k) => keys.includes(k)) ?? before.slice(0, Math.max(0, at)).reverse().find((k) => keys.includes(k));
    // The popover hands focus to its own panel when a focused control goes: that counts as fallen too.
    const panel = () => body?.parentElement;
    const handOff = focusHandOff(() => (document.activeElement === panel() ? panel() : null));
    const rowOf = (k: string) => [...(body?.querySelectorAll<HTMLElement>('[data-key]') ?? [])].find((li) => li.dataset.key === k);
    void handOff(() => (next ? rowOf(next)?.querySelector<HTMLElement>('.row') : keys.length > 0 ? head : away?.()));
  });

  let busy = $state<string[]>([]);

  async function act(item: WidgetItem, run: () => Promise<void>) {
    if (busy.includes(item.key)) return;
    busy = [...busy, item.key];
    try {
      await run();
    } finally {
      busy = busy.filter((key) => key !== item.key);
    }
  }

  // The store toasts a failed pause or resume itself.
  const toggle = (item: WidgetItem) => act(item, () => (item.pausable ? pause(item.id) : resume(item.id)));

  const cancelWaiting = (item: WidgetItem) =>
    act(item, async () => {
      try {
        await cancelInstall(item.key.slice('install:'.length));
      } catch (err) {
        toast(installErrorText(err), 'danger');
      }
    });

  function openRow(item: WidgetItem, close: () => void) {
    close();
    if (item.kind === 'game' || item.kind === 'verify') navigate('game', { id: item.id });
    // A move's progress lives in the library settings, not on the Downloads page.
    else if (item.kind === 'move') navigate('settings');
    else navigate('downloads');
  }

  function openDownloads(close: () => void) {
    close();
    navigate('downloads');
  }
</script>

{#snippet icon(item: WidgetItem)}
  <span class="icon" class:attention={item.attention} aria-hidden="true">
    {#if item.attention}
      <TriangleAlert size="1.8rem" strokeWidth={1.8} />
    {:else if item.kind === 'game'}
      <Artwork src={item.cover ?? ''} alt="" label={item.name} ratio="1 / 1" radius="var(--radius-xs)" />
    {:else if item.kind === 'install'}
      <Wrench size="1.8rem" strokeWidth={1.8} />
    {:else if item.kind === 'verify'}
      <FileCheck size="1.8rem" strokeWidth={1.8} />
    {:else if item.kind === 'move'}
      <FolderInput size="1.8rem" strokeWidth={1.8} />
    {:else}
      <Download size="1.8rem" strokeWidth={1.8} />
    {/if}
  </span>
{/snippet}

<div class="sr-only" role="status">{liveText}</div>

{#if lead}
  <div class="widget" role="group" aria-label={msg('ui.activityNow')}>
    <Popover bind:open label={msg('ui.activityNow')} placement={$themeLayout.side === 'right' ? 'top-end' : 'top-start'} width="var(--activity-panel-w)" partId="activity">
      {#snippet trigger({ props })}
        <button type="button" class="head" class:open bind:this={head} {...props}>
          {@render icon(lead)}
          <span class="name">{lead.name}</span>
          {#if reading(lead, now)}<span class="reading">{reading(lead, now)}</span>{/if}
          {#if more > 0}
            <span class="more" aria-hidden="true">{msg('ui.activityMore', { count: more })}</span>
            <span class="sr-only">{msg('ui.activityMoreLabel', { count: more })}</span>
          {/if}
        </button>
      {/snippet}
      {#snippet children({ close })}
        <div class="panel" bind:this={body}>
          <ul class="rows">
            {#each items as item (item.key)}
              <li class="item" data-key={item.key}>
                <button type="button" class="row" onclick={() => openRow(item, close)}>
                  {@render icon(item)}
                  <span class="body">
                    <span class="title">
                      <span class="name">{item.name}</span>
                      {#if item.kind !== 'game' && !item.indeterminate && item.progress !== null}
                        <span class="reading">{progressPercent(item.progress)}%</span>
                      {/if}
                    </span>
                    {#if item.progress !== null}
                      <ProgressBar value={progressPercent(item.progress)} indeterminate={item.indeterminate} tone={item.tone} height={3} decorative />
                    {/if}
                    <span class="detail">{detailOf(item, now)}</span>
                  </span>
                </button>
                {#if item.pausable || item.resumable}
                  <Button size="sm" variant="secondary" busy={busy.includes(item.key)} onclick={() => void toggle(item)}>
                    {item.pausable ? msg('ui.pause') : msg('common.continue')}
                  </Button>
                {:else if item.kind === 'install' && item.attention}
                  <Button size="sm" variant="secondary" busy={busy.includes(item.key)} onclick={() => void cancelWaiting(item)}>
                    {msg('modals.installCancelInstall')}
                  </Button>
                {/if}
              </li>
            {/each}
          </ul>
          {#if $route.name !== 'downloads'}
            <div class="foot">
              <TextLink tone="muted" onclick={() => openDownloads(close)}>{msg('ui.allDownloads')}</TextLink>
            </div>
          {/if}
        </div>
      {/snippet}
    </Popover>
    {#if lead.progress !== null}
      <span class="bar">
        <ProgressBar
          value={progressPercent(lead.progress)}
          indeterminate={lead.indeterminate}
          tone={lead.tone}
          height={3}
          label={lead.name}
          valueText={valueText(lead)}
        />
      </span>
    {/if}
  </div>
{/if}

<style>
  .widget {
    position: relative;
    display: flex;
    flex-direction: column;
    padding-top: var(--space-2);
    border-top: 1px solid var(--border);
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    height: var(--activity-head-h);
    padding: 0 var(--space-2) var(--space-2);
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    color: var(--text-2);
    text-align: start;
    font-variant-numeric: tabular-nums;
    transition:
      background var(--motion-fast) var(--ease-out),
      color var(--motion-fast) var(--ease-out);
  }

  .head:hover,
  .head.open {
    background: var(--hover);
    color: var(--text);
  }

  .head:focus-visible {
    outline-offset: -2px;
  }

  /* Under the line, inside the head's box: a press on it opens the panel as the line does. */
  .bar {
    position: absolute;
    left: var(--space-2);
    right: var(--space-2);
    bottom: var(--space-2);
    pointer-events: none;
  }

  .icon {
    display: grid;
    flex: none;
    place-items: center;
    width: 2.4rem;
    height: 2.4rem;
    overflow: hidden;
    border-radius: var(--radius-xs);
    color: var(--text-3);
  }

  .icon.attention {
    color: var(--warning);
  }

  .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
    color: var(--text);
  }

  .reading {
    flex: none;
    margin-left: auto;
    white-space: nowrap;
  }

  .more {
    flex: none;
    color: var(--text-3);
  }

  .reading + .more {
    margin-left: 0;
  }

  .name + .more {
    margin-left: auto;
  }

  .panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .rows {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .row {
    display: flex;
    flex: 1;
    align-items: flex-start;
    gap: var(--space-2);
    min-width: 0;
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    text-align: start;
    color: var(--text-2);
    transition: background var(--motion-fast) var(--ease-out);
  }

  .row:hover {
    background: var(--hover-strong);
  }

  .row:focus-visible {
    outline-offset: -2px;
  }

  .body {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .title {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
    font-size: var(--font-sm);
  }

  .title .reading {
    font-weight: 600;
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }

  .detail {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .foot {
    display: flex;
    justify-content: flex-end;
    padding: var(--space-1) var(--space-2);
    font-size: var(--font-xs);
  }

  /* The rail: the icon and the count stay; the name and its reading leave sight only, and stay the
     head's name (hidden as the shared .sr-only hides text). */
  @media (max-width: 1140px) {
    .head {
      position: relative;
      justify-content: center;
      padding: 0 0 var(--space-2);
    }

    .head .name,
    .head .reading {
      position: absolute;
      width: 1px;
      height: 1px;
      margin: -1px;
      padding: 0;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
      border: 0;
    }

    .head .more {
      position: absolute;
      top: 0;
      right: 0;
      margin: 0;
      font-size: var(--font-xs);
      color: var(--text-2);
    }

    .bar {
      left: var(--space-1);
      right: var(--space-1);
    }
  }
</style>
