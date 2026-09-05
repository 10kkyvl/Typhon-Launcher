<script lang="ts">
  import { msg } from '../../lib/i18n';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { lanStats } from '../../lib/stores/lan';
  import { settings, updateSettings } from '../../lib/stores/settings';
</script>

<div class="row">
  <div class="row-text">
    <span class="row-label">{msg('settings.generalLanLabel')}</span>
    <span class="row-sub">{msg('settings.generalLanSub')}</span>
  </div>
  <Toggle
    checked={$settings?.lanSharing ?? false}
    label={msg('settings.generalLanLabel')}
    onchange={(v) => updateSettings({ lanSharing: v })}
  />
</div>
{#if $settings?.lanSharing}
  <div class="row">
    <div class="row-text">
      <span class="row-label">{msg('settings.generalLanStateLabel')}</span>
      <span class="row-sub"
        >{msg('settings.generalLanStatsLine', {
          peersKnown: $lanStats.peersKnown,
          offersKnown: $lanStats.offersKnown,
          sharesActive: $lanStats.sharesActive,
          announcesSent: $lanStats.announcesSent,
          announcesReceived: $lanStats.announcesReceived,
        })}</span
      >
    </div>
  </div>
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .row-label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .row-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
