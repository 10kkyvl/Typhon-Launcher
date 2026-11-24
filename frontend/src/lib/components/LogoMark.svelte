<script lang="ts">
  import { settings as appearanceSettings } from '../stores/settings';
  import { displayedAccent } from '../theme/apply';
  import { rgb } from '../theme/accent';
  const logoChannels = $derived(rgb($displayedAccent).map(v => v / 255));
  // Replace the blue chroma (B-R), retaining neutral shadows and highlights.
  const logoMatrix = $derived(logoChannels.map(k => `${1-k} 0 ${k} 0 0`).join(' ') + ' 0 0 0 1 0');
</script>

<svg width="0" height="0" aria-hidden="true" style="position:absolute"><defs><filter id="typhon-accent-logo" color-interpolation-filters="sRGB"><feColorMatrix type="matrix" values={logoMatrix} /></filter></defs></svg>
<img style:filter={$appearanceSettings?.tintLogo ? 'url(#typhon-accent-logo)' : undefined} class="logo-mark" src="/typhon.png" alt="" draggable="false" />

<style>
  .logo-mark {
    width: 2.8rem;
    height: 2.8rem;
  }
</style>
