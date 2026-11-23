<script lang="ts">
  import { msg } from '../../lib/i18n';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { lanStats } from '../../lib/stores/lan';
  import { settings, updateSettings } from '../../lib/stores/settings';
</script>

<SettingsRow label={msg('settings.generalLanLabel')} hint={msg('settings.generalLanSub')}>
  {#snippet children({ labelId, hintId })}
    <Toggle
      checked={$settings?.lanSharing ?? false}
      labelledby={labelId}
      describedby={hintId}
      onchange={(v) => updateSettings({ lanSharing: v })}
    />
  {/snippet}
</SettingsRow>
{#if $settings?.lanSharing}
  <!-- What the sharing sees on the network: a status line, no control. -->
  <SettingsRow
    label={msg('settings.generalLanStateLabel')}
    hint={msg('settings.generalLanStatsLine', {
      peersKnown: $lanStats.peersKnown,
      offersKnown: $lanStats.offersKnown,
      sharesActive: $lanStats.sharesActive,
      announcesSent: $lanStats.announcesSent,
      announcesReceived: $lanStats.announcesReceived,
    })}
  />
{/if}
