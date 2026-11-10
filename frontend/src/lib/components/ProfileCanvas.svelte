<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ProfileAppearance } from '../services/account';
  import { appearanceAccentStyle, appearanceOf, AUTO_THEME, pageBackground, themeOf } from '../profile/appearance';
  import { LIGHT_INK } from '../profile/appearanceColor';
  import { artStatus, loadArtPalette, type ArtPalette } from '../profile/artPalette';
  let { appearance, autoArt, quiet = false, children }: { appearance?: ProfileAppearance; autoArt?: string; quiet?: boolean; children: Snippet } = $props();
  const a = $derived(appearanceOf(appearance));
  let art = $state<ArtPalette | null>(null);
  const theme = $derived(themeOf(a, art));
  const ink = $derived(theme.ink ?? LIGHT_INK);

  $effect(() => {
    const url = a.theme === AUTO_THEME ? autoArt : '';
    if (!url) {
      art = null;
      if (!quiet) artStatus.set('idle');
      return;
    }
    let live = true;
    void loadArtPalette(url).then((result) => {
      if (!live) return;
      art = result.ok ? result.palette : null;
      if (!quiet) artStatus.set(result.ok ? 'ok' : 'failed');
    });
    return () => { live = false; };
  });
</script>

<div class="canvas" style={appearanceAccentStyle(a, art)}
  style:--profile-bg={theme.background} style:--profile-page={pageBackground(theme)} style:--surface-2={theme.surface}
  style:--surface={theme.background} style:--surface-3={`color-mix(in srgb, ${theme.surface}, white 7%)`}
  style:--text={ink.text} style:--text-2={ink.text2} style:--text-3={ink.text3}
  style:--border={ink.border} style:--border-strong={ink.borderStrong}
>
  {@render children()}
</div>

<style>
  .canvas { min-width: 0; border-radius: var(--radius-lg); background: var(--profile-page), var(--profile-bg); color: var(--text); padding: 0 2rem 2rem; }
  @media (max-width: 800px) { .canvas { padding: 0 1.2rem 1.2rem; } }
</style>
