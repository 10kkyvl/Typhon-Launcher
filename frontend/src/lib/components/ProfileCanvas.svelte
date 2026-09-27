<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ProfileAppearance } from '../services/account';
  import { appearanceAccentStyle, appearanceOf, themeOf } from '../profile/appearance';
  let { appearance, children }: { appearance?: ProfileAppearance; children: Snippet } = $props();
  const a = $derived(appearanceOf(appearance));
  const theme = $derived(themeOf(a));
</script>

<div class="canvas" style={appearanceAccentStyle(a)}
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
