<script lang="ts">
  import LogoMark from '../components/LogoMark.svelte';
  import UpdateOverlay from '../components/UpdateOverlay.svelte';
  import UpdateShow from '../components/UpdateShow.svelte';

  let { logo = true, page = true }: { logo?: boolean; page?: boolean } = $props();
</script>

<!-- What the desktop shell gives the update show: the sidebar logo to fly from, a page with its
     heading and a control under it, the show App.svelte mounts over every screen, and the overlay
     the shell mounts. -->
{#if logo}<aside><LogoMark /></aside>{/if}
{#if page}
  <main>
    <h1 tabindex="-1">Library</h1>
    <button type="button">Play</button>
  </main>
{/if}
<UpdateShow />
<UpdateOverlay />
