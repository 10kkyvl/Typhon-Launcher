<script lang="ts">
  import { Eye, EyeOff } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';
  import { maskEmail } from '../utils/email';
  import { msg } from '../i18n';

  let { email }: { email: string } = $props();

  let revealed = $state(false);

  const masked = $derived(maskEmail(email));

  $effect(() => {
    email;
    revealed = false;
  });
</script>

<span class="masked-email">
  <span class="value" class:hidden={!revealed}>{revealed ? email : masked || '—'}</span>
  {#if email}
    <IconButton
      size="sm"
      label={revealed ? msg('ui.hideEmail') : msg('ui.showEmail')}
      onclick={() => (revealed = !revealed)}
    >
      {#if revealed}
        <EyeOff size="1.6rem" strokeWidth={1.8} />
      {:else}
        <Eye size="1.6rem" strokeWidth={1.8} />
      {/if}
    </IconButton>
  {/if}
</span>

<style>
  .masked-email {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
  }

  .value {
    overflow-wrap: anywhere;
  }

  .hidden {
    letter-spacing: 0.04em;
  }
</style>
