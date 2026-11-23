<script lang="ts">
  import { CircleAlert, CircleCheck, EllipsisVertical, FileText, Globe, ShieldAlert, TriangleAlert } from '@lucide/svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { msg } from '../../lib/i18n';
  import { sourceLocation, type Source, type SourceHealth, type SourceStatus } from '../../lib/services/sources';
  import { isIndeterminate, progressPercent, type SourceProgress } from '../../lib/sources/progress';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';
  import { sourceWarningText } from '../../lib/sources/sourceWarnings';
  import { clockTime, formatCount, numericDate, relativeDate, truncateMiddle } from '../../lib/utils/format';

  /** One source on the Sources screen: what it is, its state, its counts and its menu. */
  let {
    source,
    live,
    onopen,
    onaction,
  }: {
    source: Source;
    /** The refresh stage Go reports while the source updates. */
    live: SourceProgress | undefined;
    /** Opens the source's details. */
    onopen: (id: string) => void;
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

  function exactTime(iso: string | null) {
    if (!iso) return undefined;
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? undefined : `${numericDate(date)}, ${clockTime(date)}`;
  }

  function sourceMenu(source: Source) {
    return [
      {
        id: 'refresh',
        label: source.status === 'updating' ? msg('transfers.sourcesUpdatingEllipsis') : msg('transfers.sourcesRefreshNow'),
        disabled: source.status === 'updating',
      },
      { id: 'toggle', label: source.enabled ? msg('transfers.sourcesDisableAction') : msg('transfers.sourcesEnableAction') },
      { id: 'details', label: msg('transfers.sourcesDetailsAction') },
      { id: 'remove', label: msg('transfers.sourcesRemoveAction'), danger: true, separator: true },
    ];
  }

  const badge = $derived(statusBadgeFor(source.status));
  const health = $derived(healthMetaFor(source.health));
</script>

<div class="row" class:disabled={!source.enabled}>
  {#snippet details()}
    <button class="source" onclick={() => onopen(source.id)}>
      <span class="type-icon">
        {#if source.type === 'file'}
          <FileText size="1.7rem" strokeWidth={1.8} />
        {:else}
          <Globe size="1.7rem" strokeWidth={1.8} />
        {/if}
      </span>
      <span class="source-text">
        <span class="source-name">
          {source.name}
          {#if source.insecure}
            <span class="insecure"><ShieldAlert size="1.4rem" strokeWidth={1.8} /></span>
          {/if}
        </span>
        <span class="source-url" title={sourceLocation(source)}>{sourceLocation(source)}</span>
        {#if source.lastError}
          <span class="source-error" title={sourceErrorText(source.lastError)}>{sourceErrorText(source.lastError)}</span>
        {/if}
      </span>
    </button>
  {/snippet}
  {#if source.insecure}
    <Tooltip text={msg('transfers.sourcesInsecureTooltip')}>{@render details()}</Tooltip>
  {:else}
    {@render details()}
  {/if}
  <span class="cell status">
    {#if live}
      {@const percent = Math.round(progressPercent(live))}
      <span class="live" title={stageLabel(live)}>
        <span class="live-label">{stageLabel(live)}</span>
        <span class="live-bar">
          <ProgressBar value={percent} indeterminate={isIndeterminate(live)} height={4} label={source.name} valueText={`${stageLabel(live)}, ${percent}%`} />
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
      <IconButton size="sm" label={msg('common.error')} hint={truncateMiddle(sourceErrorText(source.lastError), 90)} onclick={() => onopen(source.id)}>
        <span class="mark danger"><CircleAlert size="1.5rem" strokeWidth={1.8} /></span>
      </IconButton>
    {:else if source.health !== 'healthy'}
      {@const warnings = source.warnings?.length ? source.warnings.map(sourceWarningText).join('; ') : ''}
      <IconButton size="sm" label={health.label} hint={warnings ? `${health.label}: ${warnings}` : undefined} onclick={() => onopen(source.id)}>
        <span class="mark {health.kind}"><health.icon size="1.5rem" strokeWidth={1.8} /></span>
      </IconButton>
    {/if}
  </span>
  <span class="cell" title={exactTime(source.lastUpdatedAt)}>{relativeDate(source.lastUpdatedAt)}</span>
  <span class="cell nums">{formatCount(source.entries)}</span>
  <span class="cell nums counts">
    <Tooltip
      text={msg('transfers.sourcesMatchTooltip', {
        matched: formatCount(source.matched),
        review: formatCount(source.review),
        unmatched: formatCount(source.unmatched),
      })}
    >
      <TextLink
        aria-label={source.review > 0
          ? msg('transfers.sourcesMatchReviewName', { matched: formatCount(source.matched), review: formatCount(source.review) })
          : msg('transfers.sourcesMatchName', { matched: formatCount(source.matched) })}
        onclick={() => onopen(source.id)}
      >
        <!-- Inline, with a no-break space between: the link's underline draws on inline boxes only. -->
        <span class="count">{formatCount(source.matched)}</span>{#if source.review > 0}&nbsp;<span class="count review">+{formatCount(source.review)}</span>{/if}
      </TextLink>
    </Tooltip>
  </span>
  <span class="cell menu-cell">
    <DropdownMenu items={sourceMenu(source)} onselect={onaction}>
      {#snippet trigger({ props })}
        <IconButton {...props} label={msg('transfers.sourcesMenuFor', { name: source.name })} size="sm">
          <EllipsisVertical size="1.6rem" strokeWidth={1.8} />
        </IconButton>
      {/snippet}
    </DropdownMenu>
  </span>
</div>

<style>
  .row:last-child {
    border-radius: 0 0 var(--radius-lg) var(--radius-lg);
  }

  .source-error {
    font-size: var(--font-xs);
    color: var(--danger);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .live {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-width: 0;
    width: 100%;
  }

  .live-label {
    font-size: var(--font-xs);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
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

  /* The columns are the table's (Sources.svelte), shared with its header row; every row draws the
     line above it, the header's included. */
  .row {
    display: grid;
    grid-template-columns: var(--source-columns);
    align-items: center;
    gap: var(--space-5);
    padding: 1.2rem 1.2rem;
    border-top: 1px solid var(--border);
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .row.disabled .source-name,
  .row.disabled .cell {
    color: var(--text-3);
  }

  .source {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
    text-align: left;
  }

  .type-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.6rem;
    height: 3.6rem;
    flex-shrink: 0;
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .source-text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .insecure {
    display: inline-flex;
    align-items: center;
    color: var(--warning);
  }

  .source-name {
    font-size: var(--font-md);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: color var(--dur) var(--ease);
  }

  .source:hover .source-name {
    color: var(--accent-text);
  }

  .source-url {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell.status,
  .cell.counts {
    overflow: visible;
  }

  /* The menu button's focus ring is drawn outside it. */
  .cell.menu-cell {
    overflow: visible;
  }

  .cell.nums {
    justify-content: flex-end;
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

  .count {
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .count.review {
    font-size: var(--font-xs);
    color: var(--warning);
  }

  .menu-cell {
    justify-self: end;
  }

  @media (max-width: 1300px) {
    .cell.nums:not(.counts) {
      display: none;
    }
  }
</style>
