<script lang="ts">
  import CountBadge from '../components/CountBadge.svelte';

  // A control named by its text with a count on it, as a nav item or a conversation row is.
  // For CountBadge.dom.test.ts.
  let { count, max }: { count: number; max?: number } = $props();
</script>

<button type="button"><span>Friends</span><CountBadge {count} {max} label={`${count} requests`} /></button>
