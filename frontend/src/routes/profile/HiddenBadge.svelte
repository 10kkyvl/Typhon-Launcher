<script lang="ts">
  import { EyeOff } from '@lucide/svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { msg } from '../../lib/i18n';

  // Who the block is hidden from is the badge's hint: a Tab stop of its own, so it reaches keyboard users.
  let { text = msg('social.hiddenDefaultHint') }: { text?: string } = $props();
</script>

<StatusBadge kind="neutral" label={msg('social.hiddenLabel')} hint={text}>
  {#snippet icon()}<EyeOff strokeWidth={1.8} />{/snippet}
</StatusBadge>
