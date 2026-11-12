<script lang="ts">
  import { platformBadge } from '../library/emulation';
  import StatusBadge from './StatusBadge.svelte';

  let { platform }: { platform: string } = $props();
</script>

<StatusBadge label={platformBadge(platform)} dot={false} />
