<script lang="ts">
  import type { NameStyle } from '../services/account';
  import { nameGradientEnd } from '../profile/appearanceColor';
  import { validAccent } from '../theme/accent';

  let { name, styleName = 'plain', nameColor = '' }: { name: string; styleName?: NameStyle; nameColor?: string } = $props();

  const color = $derived(validAccent(nameColor) ? nameColor : '');
  const end = $derived(color ? nameGradientEnd(color) : '');
</script>

<span class="name {styleName}" class:custom={!!color} style:--name-color={color || undefined} style:--name-end={end || undefined}>{name}</span>

<style>
  .plain.custom {
    color: var(--name-color);
  }

  .gradient {
    --from: var(--name-color, var(--accent-text));
    --to: var(--name-end, color-mix(in srgb, var(--accent-text) 40%, #fff));
    color: var(--from);
    background: linear-gradient(90deg, var(--from), var(--to));
    -webkit-background-clip: text;
    background-clip: text;
    -webkit-text-fill-color: transparent;
  }

  .glow {
    --halo: var(--name-color, var(--accent));
    text-shadow:
      0 0 0.6rem color-mix(in srgb, var(--halo) 70%, transparent),
      0 0 1.8rem color-mix(in srgb, var(--halo) 40%, transparent);
  }
</style>
