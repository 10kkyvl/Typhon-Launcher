<script lang="ts">
  import { onDestroy } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import UpdateFunnel from '../../lib/components/UpdateFunnel.svelte';
  import UpdateRing from '../../lib/components/UpdateRing.svelte';
  import { FUNNEL_LAYERS } from '../../lib/components/updateShowTimeline';
  import type { SelfUpdateStatus } from '../../lib/services/selfupdate';
  import { resumeFinishedShow, showFollowsStatus, updateShow } from '../../lib/stores/updateShow';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  /**
   * The launcher-update show played without an update: the show covers the window and takes every
   * press while it runs, so each play ends on its own. The statuses are made up here and go to the
   * show's store only; the launcher's own update status is not touched.
   */
  const APPLY_MS = 4000;
  const applying: SelfUpdateStatus = { state: 'applying', currentVersion: '1.4.0', availableVersion: '1.5.0' };
  const done: SelfUpdateStatus = { state: 'idle', currentVersion: '1.5.0' };
  const failed: SelfUpdateStatus = { state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0', error: 'showcase', errorCode: 'apply' };

  let timer: ReturnType<typeof setTimeout> | undefined;
  let stopMotion: (() => void) | undefined;

  function play(end: SelfUpdateStatus) {
    clearTimeout(timer);
    showFollowsStatus(applying);
    timer = setTimeout(() => showFollowsStatus(end), APPLY_MS);
  }

  // The animations setting's class, for this play only; back as it was once the show has gone.
  function playStill() {
    const root = document.documentElement;
    const had = root.classList.contains('no-anim');
    root.classList.add('no-anim');
    play(done);
    stopMotion?.();
    let off = () => {};
    off = updateShow.subscribe((show) => {
      if (show) return;
      if (!had) root.classList.remove('no-anim');
      queueMicrotask(() => off());
      stopMotion = undefined;
    });
    stopMotion = off;
  }

  onDestroy(() => {
    clearTimeout(timer);
    stopMotion?.();
  });

  const LIT = [0, 0.3, 1, 1, 1];
</script>

<ShowcaseSection title="Launcher update" note="The show that covers the window while an update is applied; each play ends on its own after four seconds.">
  <ShowcaseRow caption="Play it">
    <Button onclick={() => play(done)}>Update that works</Button>
    <Button onclick={() => play(failed)}>Update that fails</Button>
    <Button onclick={() => resumeFinishedShow(true)}>Start after an update</Button>
    <Button onclick={playStill}>With motion off</Button>
  </ShowcaseRow>

  <ShowcaseRow caption="Its funnel, lit from the bottom">
    <span class="funnel"><UpdateFunnel layers={FUNNEL_LAYERS.map(() => 1)} lit={LIT} /></span>
  </ShowcaseRow>

  <ShowcaseRow caption="Its ring with motion off: empty, a third, nearly full">
    {#each [0, 0.33, 0.92] as value (value)}
      <span class="ring"><UpdateRing {value} /></span>
    {/each}
  </ShowcaseRow>
</ShowcaseSection>

<style>
  .funnel {
    display: block;
    width: 24rem;
  }

  .ring {
    display: block;
    width: var(--spinner-lg);
    height: var(--spinner-lg);
  }
</style>
