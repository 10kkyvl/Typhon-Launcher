<script lang="ts">
  import Notice from '../components/Notice.svelte';

  // A page with a load error and its Retry, as the screens build it: the error shows while the load
  // has failed or its retry runs; on success the loaded part takes its place. `run` is the retry's
  // action; it may change the message (`message`) before it answers. `to` names where focus goes on
  // success: a control (`target`), the loaded part (`region`), or nothing (the page heading).
  // `present` takes the whole page away, as leaving it does. `holds` off: the page shows the error only
  // while its load has failed and drops it as soon as the load works, before the retry has answered
  // (as EmulatorsCard does); its Detect waits on the retry. For Notice.retry.dom.test.ts.
  let {
    run,
    message = 'The list could not be loaded',
    tone = 'danger',
    announce,
    to = 'none',
    present = true,
    holds = true,
    title,
    level,
  }: {
    run: () => Promise<boolean>;
    message?: string;
    tone?: 'danger' | 'warning';
    announce?: boolean;
    to?: 'target' | 'region' | 'none';
    present?: boolean;
    holds?: boolean;
    title?: string;
    level?: 2 | 3;
  } = $props();

  let failed = $state(true);
  let retrying = $state(false);
  let target = $state<HTMLElement>();
  let region = $state<HTMLElement>();

  async function attempt(): Promise<boolean> {
    const ok = await run();
    failed = !ok;
    // The page redraws before the answer reaches the notice.
    await Promise.resolve();
    return ok;
  }
</script>

{#if present}
  <main>
    <h1 tabindex="-1">Emulators</h1>
    <button type="button">Before</button>
    {#if failed || (holds && retrying)}
      <Notice
        {tone}
        {announce}
        {title}
        {level}
        bind:retrying
        retry={{
          label: 'Retry',
          run: attempt,
          focus: to === 'target' ? () => target : undefined,
          within: to === 'region' ? () => region : undefined,
        }}
      >
        {message}
      </Notice>
    {:else}
      <div class="rows" bind:this={region}>
        <p>PlayStation 2</p>
        <button type="button">Choose a program</button>
      </div>
    {/if}
    <button type="button" bind:this={target} disabled={retrying}>Detect</button>
  </main>
{/if}
