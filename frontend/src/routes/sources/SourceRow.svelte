<script lang="ts">
  import { CircleAlert, CircleCheck, EllipsisVertical, FileText, Globe, ShieldAlert, TriangleAlert } from '@lucide/svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { msg } from '../../lib/i18n';
  import { sourceLocation, type Source, type SourceHealth, type SourceStatus } from '../../lib/services/sources';
  import { isIndeterminate, progressPercent, type SourceProgress } from '../../lib/sources/progress';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';
  import { sourceWarningText } from '../../lib/sources/sourceWarnings';
  import { clockTime, formatCount, longDate, relativeDate, truncateMiddle } from '../../lib/utils/format';

  /**
   * One source on the Sources screen, an item of its list: what it is and where it is read from, why
   * its last refresh failed, its counts, its state and when it was refreshed, and its menu. The name is
   * the row's lead control and opens the details; every number says in the row what it counts.
   */
  let {
    source,
    live,
    enabled,
    onopen,
    ontoggle,
    onaction,
  }: {
    source: Source;
    /** The refresh stage Go reports while the source updates. */
    live: SourceProgress | undefined;
    /** What the switch shows: the state the page asked Go for while Go answers, else the source's own. */
    enabled: boolean;
    /** Opens the source's details. */
    onopen: (id: string) => void;
    /** The switch was pressed: asks for the source on (true) or off. */
    ontoggle: (enabled: boolean) => void;
    /** An item of the row menu was picked. */
    onaction: (action: string) => void;
  } = $props();

  function stageLabel(p: SourceProgress): string {
    const counts = { done: formatCount(p.done), total: formatCount(p.total) };
    switch (p.stage) {
      case 'fetch':
        return msg('transfers.sourcesStageFetch');
      case 'parse':
        return msg('transfers.sourcesStageParse', counts);
      case 'match':
        return msg('transfers.sourcesStageMatch', counts);
      case 'apply':
        return msg('transfers.sourcesStageApply');
      case 'save':
        return msg('transfers.sourcesStageSave');
    }
  }

  function statusBadgeFor(status: SourceStatus): { kind: 'success' | 'neutral' | 'danger'; label: string } {
    switch (status) {
      case 'active':
        return { kind: 'success', label: msg('transfers.sourcesStatusActive') };
      case 'disabled':
        return { kind: 'neutral', label: msg('transfers.sourcesStatusDisabled') };
      case 'error':
        return { kind: 'danger', label: msg('common.error') };
      case 'updating':
        return { kind: 'neutral', label: msg('transfers.sourcesStatusUpdating') };
    }
  }

  function healthMetaFor(health: SourceHealth): { icon: typeof CircleCheck; kind: 'success' | 'warning' | 'danger'; label: string } {
    switch (health) {
      case 'healthy':
        return { icon: CircleCheck, kind: 'success', label: msg('transfers.sourcesHealthOk') };
      case 'warning':
        return { icon: TriangleAlert, kind: 'warning', label: msg('transfers.sourcesHealthWarning') };
      case 'error':
        return { icon: CircleAlert, kind: 'danger', label: msg('transfers.sourcesHealthError') };
    }
  }

  /** When the source was last refreshed: in words, and exactly. Null when it never was. */
  function refreshedAt(iso: string | null): { words: string; exact: string } | null {
    if (!iso) return null;
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return null;
    return { words: relativeDate(iso), exact: msg('transfers.sourcesExactTime', { date: longDate(date), time: clockTime(date) }) };
  }

  function sourceMenu(source: Source) {
    return [
      {
        id: 'refresh',
        label: source.status === 'updating' ? msg('transfers.sourcesUpdatingEllipsis') : msg('transfers.sourcesRefreshNow'),
        disabled: source.status === 'updating',
      },
      { id: 'details', label: msg('transfers.sourcesDetailsAction') },
      { id: 'remove', label: msg('transfers.sourcesRemoveAction'), danger: true, separator: true },
    ];
  }

  const uid = $props.id();
  const nameId = `${uid}-name`;
  const badge = $derived(statusBadgeFor(source.status));
  const health = $derived(healthMetaFor(source.health));
  const when = $derived(refreshedAt(source.lastUpdatedAt));
  const matched = $derived(formatCount(source.matched));
  const review = $derived(formatCount(source.review));
</script>

{#snippet open()}
  <TextLink tone="plain" data-row-control="open" onclick={() => onopen(source.id)}>{source.name}</TextLink>
{/snippet}

<li class="row" class:off={!source.enabled} data-source-row={source.id}>
  <span class="type-icon" aria-hidden="true">
    {#if source.type === 'file'}
      <FileText size="1.7rem" strokeWidth={1.8} />
    {:else}
      <Globe size="1.7rem" strokeWidth={1.8} />
    {/if}
  </span>

  <div class="main">
    <p class="name" id={nameId}>
      {#if source.insecure}
        <Tooltip text={msg('transfers.sourcesInsecureTooltip')}>{@render open()}</Tooltip>
        <span class="insecure" aria-hidden="true"><ShieldAlert size="1.4rem" strokeWidth={1.8} /></span>
      {:else}
        {@render open()}
      {/if}
    </p>
    <p class="source-url">{sourceLocation(source)}</p>
    {#if source.lastError}
      <p class="source-error">{sourceErrorText(source.lastError)}</p>
    {/if}
    <p class="facts">
      <span>{msg('transfers.sourcesEntriesCount', { count: source.entries })}</span>
      <Tooltip text={msg('transfers.sourcesMatchTooltip', { matched, review, unmatched: formatCount(source.unmatched) })}>
        <TextLink
          data-row-control="counts"
          aria-label={source.review > 0 ? msg('transfers.sourcesMatchReviewName', { matched, review }) : msg('transfers.sourcesMatchName', { matched })}
          onclick={() => onopen(source.id)}
        >
          <!-- Inline, with a no-break space between: the link's underline draws on inline boxes only. -->
          <span class="count">{msg('transfers.sourcesMatchName', { matched })}</span>{#if source.review > 0}&nbsp;<span class="count review">{msg('transfers.sourcesReviewCount', { review })}</span>{/if}
        </TextLink>
      </Tooltip>
    </p>
  </div>

  <div class="state">
    <div class="status">
      {#if live}
        {@const percent = Math.round(progressPercent(live))}
        <span class="live">
          <span class="live-label">{stageLabel(live)}</span>
          <span class="live-bar">
            <ProgressBar value={percent} indeterminate={isIndeterminate(live)} height={4} label={source.name} valueText={msg('transfers.sourcesStageProgress', { stage: stageLabel(live), percent })} />
            {#if !isIndeterminate(live)}<span class="live-percent">{percent}%</span>{/if}
          </span>
        </span>
      {:else}
        <StatusBadge kind={badge.kind} label={badge.label} plain />
        {#if source.status === 'updating'}
          <Spinner size="sm" />
        {/if}
      {/if}
      {#if source.lastError}
        <IconButton size="sm" data-row-control="mark" label={msg('common.error')} hint={truncateMiddle(sourceErrorText(source.lastError), 90)} onclick={() => onopen(source.id)}>
          <span class="mark danger"><CircleAlert size="1.5rem" strokeWidth={1.8} /></span>
        </IconButton>
      {:else if source.health !== 'healthy'}
        {@const warnings = source.warnings?.length ? source.warnings.map(sourceWarningText).join('; ') : ''}
        <IconButton
          size="sm"
          data-row-control="mark"
          label={health.label}
          hint={warnings ? msg('transfers.sourcesHealthHint', { label: health.label, warnings }) : undefined}
          onclick={() => onopen(source.id)}
        >
          <span class="mark {health.kind}"><health.icon size="1.5rem" strokeWidth={1.8} /></span>
        </IconButton>
      {/if}
    </div>
    <p class="when">
      {#if when}
        <!-- The exact time goes to assistive tech as text and to the pointer as a hint; the time element
             takes focus only from the pointer, so the row adds no Tab stop for it. -->
        <Tooltip text={when.exact} describe={false}>
          <time datetime={source.lastUpdatedAt} tabindex="-1"
            ><span aria-hidden="true">{msg('transfers.sourcesUpdatedAt', { when: when.words })}</span><span class="sr-only"
              >{msg('transfers.sourcesUpdatedAt', { when: when.exact })}</span
            ></time
          >
        </Tooltip>
      {:else}
        {msg('transfers.sourcesNeverUpdated')}
      {/if}
    </p>
  </div>

  <!-- Named by the source's name the row shows: the switch says whether that source is in use. -->
  <Toggle checked={enabled} labelledby={nameId} data-row-control="switch" onchange={ontoggle} />

  <div class="actions">
    <DropdownMenu items={sourceMenu(source)} onselect={onaction}>
      {#snippet trigger({ props })}
        <IconButton {...props} data-row-control="menu" label={msg('transfers.sourcesMenuFor', { name: source.name })} size="sm">
          <EllipsisVertical size="1.6rem" strokeWidth={1.8} />
        </IconButton>
      {/snippet}
    </DropdownMenu>
  </div>
</li>

<style>
  .row {
    display: grid;
    grid-template-columns: var(--row-thumb-w) minmax(0, 1fr) minmax(16rem, 22rem) auto auto;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    transition: background var(--dur) var(--ease);
  }

  .row:not(:first-child) {
    border-top: 1px solid var(--border);
  }

  .row:hover {
    background: var(--hover);
  }

  .type-icon {
    display: grid;
    place-items: center;
    width: var(--row-thumb-w);
    height: var(--row-thumb-w);
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .main,
  .state {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .main p,
  .state p {
    margin: 0;
  }

  /* Shown whole, wrapping in the row: an ellipsis would hide words that no keyboard could reach. */
  .name {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--font-md);
    font-weight: 500;
    color: var(--text);
    overflow-wrap: anywhere;
  }

  .row.off .name,
  .row.off .facts {
    color: var(--text-3);
  }

  .insecure {
    display: inline-flex;
    color: var(--warning);
  }

  .source-url {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow-wrap: anywhere;
  }

  .source-error {
    font-size: var(--font-xs);
    color: var(--danger);
    overflow-wrap: anywhere;
  }

  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-1) var(--space-3);
    font-size: var(--font-xs);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .count {
    white-space: nowrap;
  }

  .count.review {
    color: var(--warning);
  }

  .status {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: var(--space-8);
  }

  .live {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
    width: 100%;
  }

  .live-label {
    font-size: var(--font-xs);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .live-bar {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .live-percent {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .when {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .mark {
    display: inline-flex;
  }

  .mark.danger {
    color: var(--danger);
  }

  .mark.warning {
    color: var(--warning);
  }

  .actions {
    display: flex;
    align-items: center;
  }
</style>
