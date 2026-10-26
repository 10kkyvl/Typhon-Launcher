<script lang="ts">
  import { CircleAlert, CircleCheck, CircleHelp } from '@lucide/svelte';
  import { locale, msg, type MessageKey } from '../i18n';
  import { CONTRAST_TOKENS, contrastReport, formatRatio, type ContrastRow } from '../theme/contrastReport';
  import { resolveColors } from '../theme/resolveColors';
  import { tokenLabelKey } from '../theme/tokens';

  let { vars }: { vars: Record<string, string> } = $props();

  let rows = $state<ContrastRow[]>([]);

  $effect(() => {
    rows = contrastReport(resolveColors(vars, CONTRAST_TOKENS));
  });

  const warnings = $derived(rows.filter((row) => row.ok === false).length);

  const labelKey = (id: string) => `settings.themeContrast.${id}` as MessageKey;
</script>

<section class="contrast" aria-labelledby="theme-contrast-title">
  <header>
    <h4 id="theme-contrast-title">{msg('settings.themeContrast.title')}</h4>
    <span class="summary" class:bad={warnings > 0}>
      {warnings > 0 ? msg('settings.themeContrast.summaryBad', { count: warnings }) : msg('settings.themeContrast.summaryOk')}
    </span>
  </header>
  <ul>
    {#each rows as row (row.id)}
      <li class:bad={row.ok === false}>
        <span class="icon" aria-hidden="true">
          {#if row.ok === true}<CircleCheck size={16} />{:else if row.ok === false}<CircleAlert size={16} />{:else}<CircleHelp size={16} />{/if}
        </span>
        <span class="text">
          <span class="label">{msg(labelKey(row.id))}</span>
          <span class="sub">
            {#if row.ok === false && row.against}
              {msg('settings.themeContrast.lowOn', { surface: msg(tokenLabelKey(row.against)), min: formatRatio(row.min, $locale) })}
            {:else if row.ok === null}
              {msg('settings.themeContrast.unknown')}
            {:else}
              {msg('settings.themeContrast.target', { min: formatRatio(row.min, $locale) })}
            {/if}
          </span>
        </span>
        <span class="ratio">{row.ratio === null ? '—' : formatRatio(row.ratio, $locale)}</span>
      </li>
    {/each}
  </ul>
  <p class="hint">{msg('settings.themeContrast.hint')}</p>
</section>

<style>
  .contrast {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
  }
  h4 {
    margin: 0;
    font-size: var(--font-md);
  }
  .summary {
    font-size: var(--font-sm);
    color: var(--success);
  }
  .summary.bad {
    color: var(--warning);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-2);
  }
  li.bad {
    background: var(--warning-subtle);
  }
  .icon {
    display: inline-flex;
    color: var(--success);
  }
  li.bad .icon {
    color: var(--warning);
  }
  li:not(.bad) .icon:has(:global(.lucide-circle-help)) {
    color: var(--text-3);
  }
  .text {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
  }
  .label {
    font-size: var(--font-sm);
    color: var(--text);
  }
  .sub {
    font-size: var(--font-xs);
    color: var(--text-2);
  }
  .ratio {
    font-variant-numeric: tabular-nums;
    font-size: var(--font-sm);
    color: var(--text);
  }
  .hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
