<script module lang="ts">
  import { createReportCache } from '../game/sysreqCache';

  const reports = createReportCache();
</script>

<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { Check, ChevronDown, CircleQuestionMark, TriangleAlert, X } from '@lucide/svelte';
  import { msg } from '../i18n';
  import { metadataErrorText } from '../metadata/metadataErrors';
  import { checkSystemRequirements, type SysreqReport } from '../services/sysreq';
  import {
    cacheKey,
    haveText,
    kindLabelKey,
    sortChecks,
    statusLabelKey,
    statusTone,
    verdictLabelKey,
    verdictTone,
  } from '../game/sysreq';
  import { focusableIn, focusQuietly } from '../layers/focus';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let {
    gameId,
    installPath,
    installed = false,
    revision = '',
    heading = true,
  }: { gameId: string; installPath: string; installed?: boolean; revision?: string; heading?: boolean } = $props();

  const uid = $props.id();

  let report = $state<SysreqReport | null>(null);
  let failure = $state('');
  let loading = $state(false);
  let expanded = $state(false);
  let token = 0;
  const SETTLE_MS = 300;

  function fetchReport(id: string, path: string, rev: string, fresh: boolean): Promise<SysreqReport> {
    const onDisk = installed;
    return reports.fetch(cacheKey(id, onDisk ? '' : path, rev), () => checkSystemRequirements(id, path, onDisk), fresh);
  }

  // A retry keeps the failure on screen until its answer: the notice and its focused button stay.
  async function load(id: string, path: string, rev: string, fresh: boolean, retry = false) {
    const current = ++token;
    report = null;
    if (!retry) failure = '';
    if (!id) {
      loading = false;
      failure = '';
      return;
    }
    loading = !retry;
    try {
      const result = await fetchReport(id, path, rev, fresh);
      if (current !== token) return;
      report = result;
      failure = '';
    } catch (err) {
      if (current === token) failure = metadataErrorText(err, msg('sysreq.failed'));
    } finally {
      if (current === token) loading = false;
    }
  }

  let retrying = $state(false);
  let root = $state<HTMLElement>();

  async function retry() {
    if (retrying) return;
    const button = document.activeElement;
    retrying = true;
    try {
      await load(gameId, installPath, revision, true, true);
    } finally {
      retrying = false;
    }
    // The notice went with the focused Retry: focus moves into what loaded instead of the page.
    await tick();
    const active = document.activeElement;
    if (!button || button.isConnected || (active && active !== document.body && active.isConnected)) return;
    if (root?.isConnected) focusQuietly(focusableIn(root)[0] ?? root);
  }

  $effect(() => {
    const id = gameId;
    const path = installPath;
    const rev = `${revision}|${installed}`;
    expanded = false;
    report = null;
    failure = '';
    loading = Boolean(id);
    const timer = setTimeout(() => untrack(() => void load(id, path, rev, false)), SETTLE_MS);
    return () => clearTimeout(timer);
  });

  const tone = $derived(report ? verdictTone(report.verdict) : 'neutral');
  const checks = $derived(report ? sortChecks(report.checks) : []);
</script>

{#if gameId}
  <!-- Focusable by script only: focus lands here when a retry brings a verdict without a control. -->
  <div class="sysreq" tabindex="-1" bind:this={root}>
    {#if heading}<h3 class="title">{msg('sysreq.heading')}</h3>{/if}
    {#if loading}
      <Spinner size="sm" label={msg('sysreq.loading')} />
    {:else if failure}
      <Notice tone="danger" compact>
        {failure}
        {#snippet action()}
          <Button size="sm" variant="ghost" busy={retrying} onclick={retry}>{msg('sysreq.retry')}</Button>
        {/snippet}
      </Notice>
    {:else if report}
      <div class="line">
        <StatusBadge kind={tone} label={msg(verdictLabelKey(report))} dot={false} />
        {#if checks.length > 0}
          <Button variant="ghost" size="sm" aria-expanded={expanded} aria-controls="{uid}-checks" onclick={() => (expanded = !expanded)}>
            {expanded ? msg('sysreq.hide') : msg('sysreq.show')}
            <span class="chevron" class:open={expanded}><ChevronDown size="1.4rem" strokeWidth={1.8} /></span>
          </Button>
        {/if}
      </div>
      {#if expanded}
        <ul class="checks" id="{uid}-checks">
          {#each checks as check (check.kind)}
            {@const rowTone = statusTone(check.status)}
            <li class="check">
              <span class="mark {rowTone}" role="img" aria-label={msg(statusLabelKey(check.status))} title={msg(statusLabelKey(check.status))}>
                {#if check.status === 'ok'}
                  <Check size="1.4rem" strokeWidth={2.2} />
                {:else if check.status === 'below_minimum'}
                  <X size="1.4rem" strokeWidth={2.2} />
                {:else if check.status === 'below_recommended'}
                  <TriangleAlert size="1.4rem" strokeWidth={2} />
                {:else}
                  <CircleQuestionMark size="1.4rem" strokeWidth={2} />
                {/if}
              </span>
              <div class="body">
                <span class="kind">{msg(kindLabelKey(check.kind))}</span>
                {#if check.required}<span class="detail">{msg('sysreq.required')}: {check.required}</span>{/if}
                {#if check.recommended}<span class="detail">{msg('sysreq.recommended')}: {check.recommended}</span>{/if}
                <span class="detail have {rowTone}">{msg('sysreq.have')}: {haveText(check) || msg('sysreq.haveUnknown')}</span>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
      <p class="estimate">{msg('sysreq.estimate')}</p>
    {/if}
  </div>
{/if}

<style>
  .sysreq {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
  }

  .line {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .estimate {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .chevron {
    display: inline-flex;
    transition: transform 0.15s ease;
  }

  .chevron.open {
    transform: rotate(180deg);
  }

  .checks {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    list-style: none;
    margin: 0;
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--surface-2);
  }

  .check {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
  }

  .mark {
    display: inline-flex;
    margin-top: var(--space-1);
    color: var(--text-3);
  }

  .mark.success,
  .have.success {
    color: var(--success);
  }

  .mark.warning,
  .have.warning {
    color: var(--warning);
  }

  .mark.danger,
  .have.danger {
    color: var(--danger);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .kind {
    font-size: var(--font-sm);
    font-weight: 600;
  }

  .detail {
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
</style>
