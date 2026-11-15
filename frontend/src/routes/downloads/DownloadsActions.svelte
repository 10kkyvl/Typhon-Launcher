<script lang="ts">
  import { ChevronDown, Plus, Settings } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import { maxActiveDownloadOptions } from '../../lib/services/settings';
  import { navigate } from '../../lib/stores/router';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { msg } from '../../lib/i18n';

  let { onadd }: { onadd: () => void } = $props();

  const concurrencyValue = $derived(String($settings?.maxActiveDownloads ?? 2));

  function pickConcurrency(id: string) {
    updateSettings({ maxActiveDownloads: Number(id) });
  }
</script>

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
<IconButton label={msg('transfers.downloadsSettingsLabel')} onclick={() => navigate('settings', { tab: 'downloads' })}>
  <Settings size="1.7rem" strokeWidth={1.8} />
</IconButton>
<Button variant="primary" onclick={() => onadd()}>
  <Plus size="1.5rem" strokeWidth={2} />
  {msg('transfers.downloadsAddAction')}
</Button>
