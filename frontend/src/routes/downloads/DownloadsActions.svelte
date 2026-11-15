<script lang="ts">
  import { ChevronDown, Plus, Settings } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import { maxActiveDownloadOptions } from '../../lib/services/settings';
  import { navigate } from '../../lib/stores/router';
  import { settings, updateSettingsReporting } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { codeText, msg } from '../../lib/i18n';

  let { onadd }: { onadd: () => void } = $props();

  // Shown only once the settings are read: before that there is no stored value to mark.
  const concurrencyValue = $derived($settings ? String($settings.maxActiveDownloads) : null);

  // The store shows the pick at once and puts the stored value back when Go refuses it.
  async function pickConcurrency(id: string) {
    await updateSettingsReporting({ maxActiveDownloads: Number(id) }, (err) => {
      const reason = codeText(err, '');
      toast(
        reason
          ? msg('downloads.page.concurrencySaveFailed', { reason })
          : msg('downloads.page.concurrencySaveFailedPlain'),
        'danger',
      );
    });
  }
</script>

<div class="controls">
  {#if concurrencyValue !== null}
    <DropdownMenu
      items={maxActiveDownloadOptions.map((o) => ({ ...o, checked: o.id === concurrencyValue }))}
      onselect={pickConcurrency}
    >
      {#snippet trigger({ props })}
        <Button {...props}>
          {msg('transfers.downloadsConcurrency', { value: concurrencyValue })}
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </Button>
      {/snippet}
    </DropdownMenu>
  {/if}
  <IconButton label={msg('transfers.downloadsSettingsLabel')} onclick={() => navigate('settings', { tab: 'downloads' })}>
    <Settings size="1.7rem" strokeWidth={1.8} />
  </IconButton>
  <Button variant="primary" onclick={() => onadd()}>
    <Plus size="1.5rem" strokeWidth={2} />
    {msg('transfers.downloadsAddAction')}
  </Button>
</div>

<style>
  .controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2) var(--space-3);
  }
</style>
