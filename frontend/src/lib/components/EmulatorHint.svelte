<script lang="ts">
  import { copiesOnFirstLaunch, emulatorMissing, platformBadge, type EmulatedGame } from '../library/emulation';
  import { installErrorText } from '../install/installErrors';
  import { gameInEmulator, listPlatforms, type PlatformStatus } from '../services/emulators';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';
  import Notice from './Notice.svelte';

  let { platform, game = null }: { platform: string; game?: EmulatedGame | null } = $props();

  let row = $state<PlatformStatus | null>(null);
  let copies = $state(false);
  let token = 0;
  let copyToken = 0;

  $effect(() => {
    const id = game?.id ?? '';
    const pending = copiesOnFirstLaunch(game);
    const current = ++copyToken;
    copies = false;
    if (!pending || !id) return;
    void (async () => {
      try {
        const stored = await gameInEmulator(id);
        if (current === copyToken) copies = !stored;
      } catch (err) {
        if (current === copyToken) toast(installErrorText(err), 'danger');
      }
    })();
  });

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

{#if !game && row && emulatorMissing(row)}
  <p class="hint">{msg('emul.emulatorMissing', { console: row.name || platformBadge(platform) })}</p>
{/if}
{#if copies}
  <p class="hint">{msg('emul.vitaFirstLaunch')}</p>
{/if}
{#if row?.warnings.includes('discord_exit_hang')}
  <Notice tone="warning" compact announce={false}>{msg('emul.discordExitHang')}</Notice>
{/if}

<style>
  .hint {
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }
</style>
