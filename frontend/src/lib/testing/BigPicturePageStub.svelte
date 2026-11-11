<script lang="ts">
  // Stands in for a Big Picture page in BigPictureShell.dom.test.ts: one game whose Play button
  // launches through the shell, and what that launch answered.
  let { onlaunch }: { onlaunch?: (id: string) => Promise<boolean> } = $props();

  let answer = $state('');

  async function play() {
    if (!onlaunch) return;
    try {
      answer = String(await onlaunch('game-1'));
    } catch (err) {
      answer = `error: ${String(err)}`;
    }
  }

  export function handleCommand(): boolean {
    return false;
  }
</script>

<section>
  <button type="button" data-bp-focus="play" data-bp-default onclick={play}>Play</button>
  <button type="button" data-bp-focus="details">Details</button>
  <output>{answer}</output>
</section>
