<script lang="ts">
  import Lightbox, { type LightboxItem } from '../components/Lightbox.svelte';

  // A thumbnail that opens the image viewer, as on a game page. For Lightbox.dom.test.ts.
  let { items }: { items: LightboxItem[] } = $props();

  let open = $state(false);
</script>

<button type="button" onclick={() => (open = true)}>Screenshot</button>
<Lightbox bind:open {items} />
