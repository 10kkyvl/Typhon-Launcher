<script lang="ts">
  import { settings as appearanceSettings } from '../stores/settings';
  import { logoArcsAway } from '../stores/updateShow';
  import { displayedAccent } from '../theme/apply';
  import { rgb } from '../theme/accent';
  import LogoArcs from './LogoArcs.svelte';
  const logoChannels = $derived(rgb($displayedAccent).map(v => v / 255));
  // Replace the blue chroma (B-R), retaining neutral shadows and highlights.
  const logoMatrix = $derived(logoChannels.map(k => `${1-k} 0 ${k} 0 0`).join(' ') + ' 0 0 0 1 0');
  const tint = $derived(Boolean($appearanceSettings?.tintLogo));
  // The picture at rest; while the update show flies the arcs, the same logo in vector, so the arcs
  // that are away can leave it one by one (UpdateShow.svelte finds it by `data-logo-origin`).
  const away = $derived($logoArcsAway);
</script>

<svg width="0" height="0" aria-hidden="true" style="position:absolute"><defs><filter id="typhon-accent-logo" color-interpolation-filters="sRGB"><feColorMatrix type="matrix" values={logoMatrix} /></filter></defs></svg>
<span class="mark" data-logo-origin>
  <img style:filter={tint ? 'url(#typhon-accent-logo)' : undefined} class="logo-mark" class:away={away !== null} src="/typhon.png" alt="" draggable="false" />
  {#if away}
    <span class="vector"><LogoArcs tile {tint} {away} /></span>
  {/if}
</span>

<style>
  .mark {
    position: relative;
    display: block;
    flex: none;
    width: 2.8rem;
    height: 2.8rem;
  }

  .logo-mark {
    display: block;
    width: 100%;
    height: 100%;
  }

  .logo-mark.away {
    visibility: hidden;
  }

  .vector {
    position: absolute;
    inset: 0;
  }
</style>
