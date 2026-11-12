<script lang="ts" module>
  /** How long the retry demo's "request" takes, so its busy state can be seen. */
  export const RETRY_MS = 800;
</script>

<script lang="ts">
  import { Sparkles } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import type { SegmentOption } from '../../lib/components/segmented';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { toast } from '../../lib/stores/toasts';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const uid = $props.id();
  const TONES = ['info', 'success', 'warning', 'danger'] as const;

  let closed = $state(false);

  // The retry demo: a load that failed, its Retry answering as the page is set to answer.
  const OUTCOMES: SegmentOption[] = [
    { id: 'fail', label: 'Fails' },
    { id: 'succeed', label: 'Succeeds' },
  ];
  let outcome = $state('fail');
  let failed = $state(true);
  let retrying = $state(false);
  let attempts = $state(0);
  const againId = `${uid}-again`;

  async function retry(): Promise<boolean> {
    attempts += 1;
    await new Promise<void>((resolve) => setTimeout(resolve, RETRY_MS));
    failed = outcome === 'fail';
    return !failed;
  }
</script>

<ShowcaseSection title="Notices" note="A notice of each tone, with a title, an action, a close button, compact, and one whose Retry the page makes fail or succeed.">
  <ShowcaseRow caption="Tones, titled" stack>
    {#each TONES as tone (tone)}
      <Notice {tone} title={`A ${tone} notice`} announce={false}>What the {tone} tone says, in the words of the page.</Notice>
    {/each}
  </ShowcaseRow>

  <ShowcaseRow caption="With an action, compact, with its own icon, closable" stack>
    <Notice tone="warning" announce={false}>
      The library folder is missing.
      {#snippet action()}<Button size="sm" onclick={() => toast('Choose a folder pressed', 'info')}>Choose a folder</Button>{/snippet}
    </Notice>
    <Notice tone="info" compact announce={false}>Compact: inside a card or a dialog, tighter and smaller.</Notice>
    <Notice tone="success" announce={false}>
      {#snippet icon()}<Sparkles size="1.6rem" />{/snippet}
      A notice with an icon of its own.
    </Notice>
    {#if closed}
      <TextLink onclick={() => (closed = false)}>Show the closable notice again</TextLink>
    {:else}
      <Notice tone="info" announce={false} onclose={() => (closed = true)}>Closing it moves focus on to the next control.</Notice>
    {/if}
  </ShowcaseRow>

  <ShowcaseRow caption="Retry" stack>
    <div class="outcome">
      <span class="label" id="{uid}-outcome">The next retry</span>
      <SegmentedControl labelledby="{uid}-outcome" options={OUTCOMES} bind:value={outcome} />
      <span class="label">Attempts: {attempts}</span>
    </div>
    {#if failed || retrying}
      <Notice tone="danger" title="The list could not be loaded" bind:retrying retry={{ label: 'Retry', pending: 'Retrying…', run: retry, focus: () => document.getElementById(againId) }}>
        A failed retry says this again and keeps focus on Retry; a successful one replaces the notice and moves focus to what replaced it.
      </Notice>
    {:else}
      <div class="loaded">
        <span>The list loaded.</span>
        <Button id={againId} onclick={() => (failed = true)}>Fail the load again</Button>
      </div>
    {/if}
  </ShowcaseRow>
</ShowcaseSection>

<style>
  .outcome,
  .loaded {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .label {
    font-size: var(--font-sm);
    color: var(--text-2);
  }
</style>
