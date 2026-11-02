<script lang="ts">
  import { emulatorMissing, platformBadge } from '../library/emulation';
  import { installErrorText } from '../install/installErrors';
  import { listPlatforms, type PlatformStatus } from '../services/emulators';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let { platform }: { platform: string } = $props();

  let row = $state<PlatformStatus | null>(null);
  let token = 0;

  $effect(() => {
    const id = platform;
    const current = ++token;
    row = null;
    void (async () => {
      try {
        const list = await listPlatforms();
        if (current === token) row = list.find((item) => item.id === id) ?? null;
      } catch (err) {
        if (current === token) toast(installErrorText(err), 'danger');
      }
    })();
  });
</script>

{#if row && emulatorMissing(row)}
  <p class="hint">{msg('emul.emulatorMissing', { console: row.name || platformBadge(platform) })}</p>
{/if}

<style>
  .hint {
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }
</style>
