<script lang="ts">
  import { ExternalLink } from '@lucide/svelte';
  import { t } from '../i18n';
  import { dlcView, hasRequirements, requirementColumns } from '../game/extras';
  import type { SteamInfo } from '../services/metadata';
  import Tabs from './Tabs.svelte';

  let { steam, onopendlc }: { steam: SteamInfo | null | undefined; onopendlc: (appId: number) => void } = $props();

  let requirementsTab = $state('minimum');
  let dlcExpanded = $state(false);

  const columns = $derived(requirementColumns(steam));
  const bothColumns = $derived(columns.minimum.length > 0 && columns.recommended.length > 0);
  const shown = $derived(
    bothColumns ? requirementsTab : columns.minimum.length > 0 ? 'minimum' : 'recommended',
  );
  const rows = $derived(shown === 'minimum' ? columns.minimum : columns.recommended);
  const requirementTabs = $derived([
    { id: 'minimum', label: $t('games.detailRequirementsMinimum') },
    { id: 'recommended', label: $t('games.detailRequirementsRecommended') },
  ]);
  const dlc = $derived(dlcView(steam?.dlc, dlcExpanded));
</script>

{#if hasRequirements(columns)}
  <section class="block">
    <h2 class="heading">{$t('games.detailRequirementsHeading')}</h2>
    {#if bothColumns}
      <Tabs tabs={requirementTabs} bind:value={requirementsTab} variant="pill" />
    {:else}
      <p class="column-title">
        {shown === 'minimum' ? $t('games.detailRequirementsMinimum') : $t('games.detailRequirementsRecommended')}
      </p>
    {/if}
    <dl class="rows">
      {#each rows as row (row.label)}
        <div class="row">
          <dt>{row.label}</dt>
          <dd>{row.value}</dd>
        </div>
      {/each}
    </dl>
  </section>
{/if}

{#if dlc.total > 0}
  <section class="block">
    <h2 class="heading">{$t('games.detailDlcHeading', { count: dlc.total })}</h2>
    <ul class="dlc">
      {#each dlc.items as item (item.appId)}
        <li>
          <button class="dlc-link" onclick={() => onopendlc(item.appId)}>
            <span class="dlc-name">{item.name}</span>
            <ExternalLink size="1.4rem" strokeWidth={1.8} />
          </button>
        </li>
      {/each}
    </ul>
    {#if dlc.collapsible}
      <button class="toggle" onclick={() => (dlcExpanded = !dlcExpanded)}>
        {dlcExpanded ? $t('games.detailDlcCollapse') : $t('games.detailDlcShowAll')}
      </button>
    {/if}
  </section>
{/if}

<style>
  .block {
    margin-top: var(--space-10);
  }

  .heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  .column-title {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .rows {
    display: flex;
    flex-direction: column;
    margin-top: var(--space-3);
  }

  .row {
    display: flex;
    align-items: baseline;
    gap: var(--space-4);
    padding: 0.9rem 0;
    border-top: 1px solid var(--border);
    font-size: var(--font-sm);
  }

  dt {
    flex: 0 0 16rem;
    color: var(--text-3);
  }

  dd {
    flex: 1;
    min-width: 0;
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .dlc {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .dlc-link {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    min-height: 2.8rem;
    max-width: 100%;
    padding: 0 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-2);
    font-size: var(--font-xs);
    font-weight: 500;
    transition:
      color var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .dlc-link:hover {
    color: var(--text);
    border-color: var(--border-strong);
  }

  .dlc-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .toggle {
    margin-top: var(--space-3);
    color: var(--accent);
    font-size: var(--font-xs);
    font-weight: 500;
  }
</style>
