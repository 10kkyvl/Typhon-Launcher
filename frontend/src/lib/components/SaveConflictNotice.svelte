<script lang="ts">
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import { msg } from '../i18n';
  import type { CloudConflict } from '../services/cloudsaves';
  import { dateTime } from '../utils/format';

  /**
   * The words of a save conflict on the game page: when each copy changed and the device of the cloud
   * one, or the conflict in general words while the status has no details, and one action that asks
   * for the choice. It draws what it is given; CloudConflictBanner decides when it shows.
   */
  let {
    conflict,
    onresolve,
  }: {
    conflict?: Pick<CloudConflict, 'localUpdatedAt' | 'remoteUpdatedAt' | 'remoteDevice'>;
    onresolve: () => void;
  } = $props();

  function when(iso: string): string {
    const value = new Date(iso);
    return Number.isNaN(value.getTime()) ? iso : dateTime(value);
  }

  const line = $derived.by(() => {
    if (!conflict) return msg('cloudsaves.conflictTitle');
    const params = { local: when(conflict.localUpdatedAt), remote: when(conflict.remoteUpdatedAt) };
    return conflict.remoteDevice
      ? msg('cloudsaves.bannerWhenDevice', { ...params, device: conflict.remoteDevice })
      : msg('cloudsaves.bannerWhen', params);
  });
</script>

<Notice tone="warning">
  {line}
  {#snippet action()}
    <Button size="sm" variant="primary" onclick={onresolve}>{msg('cloudsaves.resolve')}</Button>
  {/snippet}
</Notice>
