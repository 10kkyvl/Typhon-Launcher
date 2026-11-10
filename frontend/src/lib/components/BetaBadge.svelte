<script lang="ts">
  import { onMount } from 'svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import type { MenuItem } from './DropdownMenu.svelte';
  import { inWails } from '../services/backend';
  import { betaReportRoute, isBeta } from '../services/selfupdateChannel';
  import { logsReason } from '../services/logsMessages';
  import { appInfo, exportLogs } from '../services/system';
  import { navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { authState } from '../stores/user';
  import { msg } from '../i18n';

  let channel = $state<string | undefined>();
  let version = $state('');

  onMount(() => {
    appInfo()
      .then((info) => {
        channel = info.channel;
        version = info.version;
      })
      .catch((err) => toast(logsReason(err), 'danger'));
  });

  const items = $derived<MenuItem[]>([{ id: 'report', label: msg('ui.betaReport') }]);

  async function report() {
    if (betaReportRoute($authState) === 'ticket') {
      navigate('support', { view: 'new', topic: 'launcher', subject: msg('tickets.betaSubject', { version }) });
      return;
    }
    if (!inWails) {
      toast(msg('settings.aboutLogsDesktopOnly'));
      return;
    }
    try {
      await exportLogs();
      toast(msg('ui.betaReportLogsSaved'), 'success');
    } catch (err) {
      toast(logsReason(err), 'danger');
    }
  }
</script>

{#if isBeta(channel)}
  <DropdownMenu {items} align="left" onselect={report}>
    {#snippet trigger({ toggle })}
      <button class="badge" title={msg('ui.betaBadgeTitle', { version })} onclick={toggle}>{msg('ui.betaBadge')}</button>
    {/snippet}
  </DropdownMenu>
{/if}

<style>
  .badge {
    padding: 0 0.6rem;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--accent-on, #fff);
    font-size: 1rem;
    font-weight: 700;
    letter-spacing: 0.05em;
    line-height: 1.8rem;
  }
</style>
