<script lang="ts">
  import Tabs from '../components/Tabs.svelte';
  import { tabPanel, type TabItem } from '../components/tabs';

  // A tab strip with the panel of the chosen tab under it, between two plain buttons, as the pages
  // wire it. For Tabs.dom.test.ts.
  let { tabs, start, variant }: { tabs: TabItem[]; start: string; variant?: 'underline' | 'pill' } = $props();

  // svelte-ignore state_referenced_locally: the starting value only
  let value = $state(start);
</script>

<button type="button">Before</button>
<Tabs {tabs} {variant} idPrefix="demo" label="Sections" bind:value />
<div {...tabPanel('demo', value)}>
  <p>Content of {value}</p>
</div>
<button type="button">After</button>
