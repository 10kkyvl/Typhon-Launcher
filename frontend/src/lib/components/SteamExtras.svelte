<script lang="ts">
  import { ExternalLink } from '@lucide/svelte';
  import { t } from '../i18n';
  import { dlcView, hasRequirements, requirementColumns } from '../game/extras';
  import type { SteamInfo } from '../services/metadata';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import Tabs from './Tabs.svelte';
  import { tabPanel } from './tabs';

  let { steam, onopendlc }: { steam: SteamInfo | null | undefined; onopendlc: (appId: number) => void } = $props();

  const uid = $props.id();

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

{#snippet mark()}
  <ExternalLink />
{/snippet}

<!-- A DLC's store page has no address here: the launcher builds it and opens it, so it is a chip button. -->
{#if hasRequirements(columns)}
  <section class="block">
    <h2 class="heading" id="{uid}-requirements">{$t('games.detailRequirementsHeading')}</h2>
    {#if bothColumns}
      <Tabs tabs={requirementTabs} idPrefix={uid} labelledby="{uid}-requirements" variant="pill" bind:value={requirementsTab} />
    {:else}
      <p class="column-title">
        {shown === 'minimum' ? $t('games.detailRequirementsMinimum') : $t('games.detailRequirementsRecommended')}
      </p>
    {/if}
    <div {...bothColumns ? tabPanel(uid, requirementsTab) : {}}>
      <dl class="rows">
        {#each rows as row (row.key)}
          <div class="row">
            <dt>{row.label}</dt>
            <dd>{row.value}</dd>
          </div>
        {/each}
      </dl>
    </div>
  </section>
{/if}

{#if dlc.total > 0}
  <section class="block">
    <h2 class="heading">{$t('games.detailDlcHeading', { count: dlc.total })}</h2>
    <ul class="dlc" id="{uid}-dlc">
      {#each dlc.items as item (item.appId)}
        <li>
          <Chip onclick={() => onopendlc(item.appId)} end={mark}><span class="dlc-name">{item.name}</span></Chip>
        </li>
      {/each}
    </ul>
    {#if dlc.collapsible}
      <div class="toggle">
        <Button variant="ghost" size="sm" aria-expanded={dlcExpanded} aria-controls="{uid}-dlc" onclick={() => (dlcExpanded = !dlcExpanded)}>
          {dlcExpanded ? $t('games.detailDlcCollapse') : $t('games.detailDlcShowAll')}
        </Button>
      </div>
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



  /* A long name ends in an ellipsis, not past the column: the chip has no limit of its own. */
  .dlc-name {
    max-width: 28rem;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .toggle {
    margin-top: var(--space-3);
  }
</style>
