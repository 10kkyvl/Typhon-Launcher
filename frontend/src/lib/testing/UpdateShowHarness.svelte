<script lang="ts">
  import LogoMark from '../components/LogoMark.svelte';
  import UpdateOverlay from '../components/UpdateOverlay.svelte';

  let { logo = true, page = true }: { logo?: boolean; page?: boolean } = $props();
</script>

<!-- What the desktop shell gives the update show: the sidebar logo to fly from, a page with its
     heading and a control under it, and the overlay App.svelte mounts after the shell. -->
{#if logo}<aside><LogoMark /></aside>{/if}
{#if page}
  <main>
    <h1 tabindex="-1">Library</h1>
    <button type="button">Play</button>
  </main>
{/if}
<UpdateOverlay />
