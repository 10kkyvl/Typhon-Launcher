<script lang="ts">
  import { ChevronUp, Download, Pause, Play, Wrench } from '@lucide/svelte';
  import { activity, activitySummary, type ActivityItem } from '../stores/activity';
  import { pause, resume } from '../stores/downloads';
  import { navigate } from '../stores/router';
  import { clickOutside } from '../utils/clickOutside';
  import IconButton from './IconButton.svelte';
  import ProgressBar from './ProgressBar.svelte';

  const RING = 2 * Math.PI * 8;

  let hovered = $state(false);
  let focused = $state(false);
  let pinned = $state(false);
  let dismissed = $state(false);
  let root = $state<HTMLElement | undefined>(undefined);

  const items = $derived($activity);
  const summary = $derived($activitySummary);
  const expanded = $derived(items.length > 0 && (pinned || ((hovered || focused) && !dismissed)));
  const headline = $derived(summary.primary ? `${summary.primary.status} · ${summary.primary.name}` : '');

  function toneColor(tone: ActivityItem['tone']) {
    if (tone === 'warning') return 'var(--warning)';
    if (tone === 'danger') return 'var(--danger)';
    if (tone === 'muted') return 'var(--text-3)';
    return 'var(--accent)';
  }

  function pct(value: number) {
    return Math.floor(Math.min(1, Math.max(0, value)) * 100);
  }

  function toggle() {
    if (expanded) {
      pinned = false;
      dismissed = true;
      return;
    }
    pinned = true;
    dismissed = false;
  }

  function relax() {
    if (!hovered && !focused) dismissed = false;
  }

  function open() {
    pinned = false;
    dismissed = false;
    navigate('downloads');
  }

  function onFocusOut(event: FocusEvent) {
    const next = event.relatedTarget as Node | null;
    if (next && root?.contains(next)) return;
    focused = false;
    relax();
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key !== 'Escape') return;
    pinned = false;
    dismissed = hovered || focused;
  }}
/>

{#if items.length > 0}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="dock"
    bind:this={root}
    use:clickOutside={() => {
      pinned = false;
      dismissed = false;
    }}
    onmouseenter={() => (hovered = true)}
    onmouseleave={() => {
      hovered = false;
      relax();
    }}
    onfocusin={() => (focused = true)}
    onfocusout={onFocusOut}
  >
    {#if expanded}
      <div class="panel">
        <div class="panel-head">
          <span class="panel-title">Активность</span>
          <button class="panel-link" onclick={open}>Все загрузки</button>
        </div>
        <div class="rows">
          {#each items as item (item.key)}
            <div
              class="row"
              role="button"
              tabindex="0"
              onclick={open}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  open();
                }
              }}
            >
              <span class="row-icon" class:attention={item.attention}>
                {#if item.kind === 'install'}
                  <Wrench size="1.6rem" strokeWidth={1.8} />
                {:else}
                  <Download size="1.6rem" strokeWidth={1.8} />
                {/if}
              </span>
              <span class="row-body">
                <span class="row-head">
                  <span class="row-name">{item.name}</span>
                  <span class="row-pct">{pct(item.progress)}%</span>
                </span>
                <ProgressBar value={pct(item.progress)} color={toneColor(item.tone)} height={3} />
                <span class="row-foot">
                  <span class="row-status">{item.status}</span>
                  {#if item.detail}
                    <span class="row-detail">{item.detail}</span>
                  {/if}
                </span>
              </span>
              <span class="row-controls">
                {#if item.pausable}
                  <IconButton
                    label="Пауза"
                    size="sm"
                    onclick={(e) => {
                      e.stopPropagation();
                      pause(item.downloadId);
                    }}
                  >
                    <Pause size="1.6rem" strokeWidth={1.8} />
                  </IconButton>
                {:else if item.resumable}
                  <IconButton
                    label="Продолжить"
                    size="sm"
                    onclick={(e) => {
                      e.stopPropagation();
                      resume(item.downloadId);
                    }}
                  >
                    <Play size="1.6rem" strokeWidth={1.8} />
                  </IconButton>
                {/if}
              </span>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <button
      class="pill"
      class:open={expanded}
      class:attention={summary.attention}
      onclick={toggle}
      title={headline}
    >
      <span class="ring">
        <svg viewBox="0 0 20 20">
          <circle class="ring-track" cx="10" cy="10" r="8" />
          <circle
            class="ring-fill"
            cx="10"
            cy="10"
            r="8"
            stroke-dasharray={RING}
            stroke-dashoffset={RING * (1 - Math.min(1, Math.max(0, summary.progress)))}
          />
        </svg>
      </span>
      <span class="pill-text">{headline}</span>
      {#if items.length > 1}
        <span class="pill-more">+{items.length - 1}</span>
      {/if}
      <span class="pill-pct">{pct(summary.progress)}%</span>
      <span class="pill-chevron" class:down={expanded}>
        <ChevronUp size="1.6rem" strokeWidth={1.8} />
      </span>
    </button>
  </div>
{/if}

<style>
  .dock {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.6rem;
  }

  .pill {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    max-width: 34rem;
    height: 3.2rem;
    padding: 0 0.8rem 0 0.6rem;
    background: var(--surface-4);
    border: 1px solid var(--border-strong);
    border-radius: 99rem;
    box-shadow: var(--shadow-pop);
    color: var(--text-2);
    font-size: 1.2rem;
    font-variant-numeric: tabular-nums;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
    animation: dock-in var(--dur-panel) var(--ease);
  }

  .pill:hover,
  .pill.open {
    background: var(--surface-3);
    color: var(--text);
  }

  .pill.attention {
    border-color: rgba(216, 164, 93, 0.45);
  }

  .ring {
    display: flex;
    width: 2rem;
    height: 2rem;
    flex-shrink: 0;
  }

  .ring svg {
    width: 100%;
    height: 100%;
    transform: rotate(-90deg);
  }

  .ring circle {
    fill: none;
    stroke-width: 2;
  }

  .ring-track {
    stroke: var(--hover-strong);
  }

  .ring-fill {
    stroke: var(--accent);
    stroke-linecap: round;
    transition: stroke-dashoffset 600ms linear;
  }

  .pill.attention .ring-fill {
    stroke: var(--warning);
  }

  .pill-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pill-more {
    flex-shrink: 0;
    padding: 0 0.5rem;
    border-radius: var(--radius-xs);
    background: var(--hover-strong);
    color: var(--text-3);
    font-size: 1.1rem;
    line-height: 1.6rem;
  }

  .pill-pct {
    flex-shrink: 0;
    color: var(--text-3);
  }

  .pill-chevron {
    display: flex;
    flex-shrink: 0;
    color: var(--text-3);
    transition: transform var(--dur) var(--ease);
  }

  .pill-chevron.down {
    transform: rotate(180deg);
  }

  .panel {
    width: 38rem;
    max-height: 42rem;
    overflow-y: auto;
    padding: 0.6rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: dock-panel var(--dur-panel) var(--ease);
  }

  .panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.6rem 0.8rem 0.5rem;
  }

  .panel-title {
    font-size: 1.1rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
  }

  .panel-link {
    font-size: var(--font-xs);
    color: var(--text-3);
    transition: color var(--dur-fast) var(--ease);
  }

  .panel-link:hover {
    color: var(--accent-text);
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: flex-start;
    gap: 0.8rem;
    padding: 0.8rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: background var(--dur-fast) var(--ease);
  }

  .row:hover {
    background: var(--hover-strong);
  }

  .row-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.4rem;
    height: 2.4rem;
    flex-shrink: 0;
    border-radius: var(--radius-xs);
    background: var(--surface-4);
    color: var(--text-3);
  }

  .row-icon.attention {
    color: var(--warning);
    background: var(--warning-subtle);
  }

  .row-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .row-head,
  .row-foot {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.8rem;
    min-width: 0;
  }

  .row-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--font-xs);
    font-weight: 500;
    color: var(--text);
  }

  .row-pct,
  .row-status,
  .row-detail {
    flex-shrink: 0;
    font-size: 1.1rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .row-status {
    color: var(--text-2);
  }

  .row-detail {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex-shrink: 1;
  }

  .row-controls {
    display: flex;
    justify-content: flex-end;
    min-width: 2.8rem;
    flex-shrink: 0;
    margin: -0.2rem -0.4rem 0 0;
  }

  @keyframes dock-in {
    from {
      opacity: 0;
      transform: translateY(0.6rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @keyframes dock-panel {
    from {
      opacity: 0;
      transform: translateY(0.8rem) scale(0.98);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }
</style>
