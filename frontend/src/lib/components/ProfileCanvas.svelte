<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ProfileAppearance } from '../services/account';
  import { appearanceAccentStyle, appearanceOf, AUTO_THEME, themeOf } from '../profile/appearance';
  import { artStatus, loadArtPalette, type ArtPalette } from '../profile/artPalette';
  let { appearance, autoArt, children }: { appearance?: ProfileAppearance; autoArt?: string; children: Snippet } = $props();
  const a = $derived(appearanceOf(appearance));
  let art = $state<ArtPalette | null>(null);
  const theme = $derived(themeOf(a, art));

  $effect(() => {
    const url = a.theme === AUTO_THEME ? autoArt : '';
    if (!url) {
      art = null;
      artStatus.set('idle');
      return;
    }
    let live = true;
    void loadArtPalette(url).then((result) => {
      if (!live) return;
      art = result.ok ? result.palette : null;
      artStatus.set(result.ok ? 'ok' : 'failed');
    });
    return () => { live = false; };
  });
</script>

<div class="canvas" style={appearanceAccentStyle(a, art)}
  style:--profile-bg={theme.background} style:--surface-2={theme.surface}
  style:--surface={theme.background} style:--surface-3={`color-mix(in srgb, ${theme.surface}, white 7%)`}
  style:--text="#f1f4f8" style:--text-2="#bec7d2" style:--text-3="#99a5b5"
  style:--border="rgba(200, 220, 240, .12)" style:--border-strong="rgba(200, 220, 240, .23)"
>
  {@render children()}
</div>

<style>
  .canvas { min-width: 0; border-radius: var(--radius-lg); background: var(--profile-bg); color: var(--text); padding: 0 2rem 2rem; }
  @media (max-width: 800px) { .canvas { padding: 0 1.2rem 1.2rem; } }
</style>
