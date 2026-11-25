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
    describedby,
  }: {
    label?: string;
    variant?: 'primary' | 'secondary' | 'ghost';
    size?: 'md' | 'sm' | 'lg';
    /** Id of the words that say what joining is for (a settings row's hint). */
    describedby?: string;
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

<Button {variant} {size} aria-describedby={describedby} onclick={open}>
  <MessagesSquare size="1.5rem" strokeWidth={1.8} />
  {label}
</Button>
