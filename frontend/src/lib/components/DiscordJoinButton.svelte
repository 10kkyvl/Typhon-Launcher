<script lang="ts">
  import { MessagesSquare } from '@lucide/svelte';
  import { msg } from '../i18n';
  import { openDiscordInvite } from '../services/discord';
  import { toast } from '../stores/toasts';
  import Button from './Button.svelte';

  let {
    label = msg('discord.joinButton'),
    variant = 'secondary',
    size = 'sm',
  }: {
    label?: string;
    variant?: 'primary' | 'secondary' | 'ghost';
    size?: 'md' | 'sm' | 'lg';
  } = $props();

  async function open() {
    try {
      await openDiscordInvite();
    } catch (err) {
      console.error('open discord invite', err);
      toast(msg('discord.openFailed'), 'danger');
    }
  }
</script>

<Button {variant} {size} onclick={open}>
  <MessagesSquare size="1.65rem" strokeWidth={1.8} />
  {label}
</Button>
