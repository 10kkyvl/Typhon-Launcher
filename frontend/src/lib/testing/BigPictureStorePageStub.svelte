<script lang="ts">
  // Stands in for the Big Picture game page in BigPictureStoreLinks.dom.test.ts: its Play button and
  // the store links it draws (StoreLinks, as GamePage does), and which store the launcher was asked to open.
  import StoreLinks from '../components/StoreLinks.svelte';

  let opened = $state('');

  export function handleCommand(): boolean {
    return false;
  }
</script>

<section>
  <button type="button" data-bp-focus="game:play" data-bp-default>Play</button>
  <StoreLinks
    links={{ steam: 'https://store.steampowered.com/app/1145360', gog: 'gog-galaxy://game' }}
    onopen={(store) => (opened += `${store};`)}
    focusPrefix="game:store:"
  />
  <output>{opened}</output>
</section>
