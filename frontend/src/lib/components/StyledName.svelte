<script lang="ts">
  import type { NameStyle } from '../services/account';

  let { name, styleName = 'plain' }: { name: string; styleName?: NameStyle } = $props();
</script>

<span class="name {styleName}">{name}</span>

<style>
  .gradient {
    color: var(--accent-text);
    background: linear-gradient(90deg, var(--accent-text), color-mix(in srgb, var(--accent-text) 40%, #fff));
    -webkit-background-clip: text;
    background-clip: text;
    -webkit-text-fill-color: transparent;
  }

  .glow {
    text-shadow:
      0 0 0.6rem color-mix(in srgb, var(--accent) 70%, transparent),
      0 0 1.8rem color-mix(in srgb, var(--accent) 40%, transparent);
  }
</style>
