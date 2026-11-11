<script lang="ts">
  import Button from '../components/Button.svelte';
  import PlayProfileMenu from '../components/PlayProfileMenu.svelte';

  // The play button with its launch-profile chevron, as the game page builds it. For Button.dom.test.ts.
  let { onplay, onpick }: { onplay?: () => void; onpick?: (id: string) => void } = $props();

  const game = { profiles: [{ id: 'p1', name: 'DirectX 11', exePath: '', args: '', env: '' }], lastProfile: '', emulation: null };
</script>

<button type="button">Before</button>
<span role="group" aria-label="Play">
  <Button variant="primary" size="lg" join="end" onclick={onplay}>Play</Button>
  <PlayProfileMenu game={game as never} onpick={(id) => onpick?.(id)} />
</span>
<button type="button">After</button>
