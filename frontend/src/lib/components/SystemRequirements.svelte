<script module lang="ts">
  import { createReportCache } from '../game/sysreqCache';

  const reports = createReportCache();
</script>

<script lang="ts">
  import { untrack } from 'svelte';
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
  import Button from './Button.svelte';

  let {
    gameId,
    installPath,
    installed = false,
    revision = '',
    heading = true,
  }: { gameId: string; installPath: string; installed?: boolean; revision?: string; heading?: boolean } = $props();

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

  async function load(id: string, path: string, rev: string, fresh: boolean) {
    const current = ++token;
    report = null;
    failure = '';
    if (!id) {
      loading = false;
      return;
    }
    loading = true;
    try {
      const result = await fetchReport(id, path, rev, fresh);
      if (current === token) report = result;
    } catch (err) {
      if (current === token) failure = metadataErrorText(err, msg('sysreq.failed'));
    } finally {
      if (current === token) loading = false;
    }
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
  <div class="sysreq">
    {#if heading}<h3 class="title">{msg('sysreq.heading')}</h3>{/if}
    {#if loading}
      <p class="muted">{msg('sysreq.loading')}</p>
    {:else if failure}
      <div class="line">
        <span class="muted">{failure}</span>
        <Button size="sm" variant="ghost" onclick={() => load(gameId, installPath, revision, true)}>{msg('sysreq.retry')}</Button>
      </div>
    {:else if report}
      <div class="line">
        <span class="badge {tone}">{msg(verdictLabelKey(report))}</span>
        {#if checks.length > 0}
          <button class="toggle" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>
            {expanded ? msg('sysreq.hide') : msg('sysreq.show')}
            <span class="chevron" class:open={expanded}><ChevronDown size="1.4rem" strokeWidth={1.8} /></span>
          </button>
        {/if}
      </div>
      {#if expanded}
        <ul class="checks">
          {#each checks as check (check.kind)}
            {@const rowTone = statusTone(check.status)}
            <li class="check">
              <span class="mark {rowTone}" title={msg(statusLabelKey(check.status))}>
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

  .muted,
  .estimate {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .badge {
    display: inline-flex;
    align-items: center;
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    font-weight: 600;
    background: var(--surface-3);
    color: var(--text-2);
  }

  .badge.success {
    background: var(--success-subtle);
    color: var(--success);
  }

  .badge.warning {
    background: var(--warning-subtle);
    color: var(--warning);
  }

  .badge.danger {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    color: var(--text-2);
    font-size: var(--font-sm);
    cursor: pointer;
  }

  .toggle:hover {
    color: var(--text);
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
    padding: 0;
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
