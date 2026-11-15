<script lang="ts">
  import type { Snippet } from 'svelte';
  import SectionHeader from '../../lib/components/SectionHeader.svelte';

  /**
   * One section of the downloads screen: its heading with the count and a slot for the section's
   * actions ("Clear the list"), and the section named by that heading. `id` is the heading's, one of
   * `downloadsSectionIds` (sections.ts), which the lists hand focus to when their last row leaves.
   */
  let {
    title,
    count,
    id,
    actions,
    children,
  }: { title: string; count: number; id: string; actions?: Snippet; children: Snippet } = $props();
</script>

<section class="section" aria-labelledby={id}>
  <SectionHeader {title} {count} {id} {actions} />
  {@render children()}
</section>

<style>
  .section {
    margin-bottom: var(--space-8);
    max-width: 140rem;
  }
</style>
