<script lang="ts">
  import CompletedSection from './CompletedSection.svelte';

  /**
   * Both sections of the completed downloads in one place, "Installing" above "Completed", for a page
   * that mounts them together; a page that places them apart mounts two `CompletedSection`s.
   */
  let {
    onopen,
    oninstall,
    onannounce,
    away,
  }: {
    onopen: (id: string) => void;
    oninstall: (id: string) => void;
    onannounce?: (text: string) => void;
    away?: () => HTMLElement | null | undefined;
  } = $props();
</script>

<CompletedSection part="installing" {onopen} {oninstall} {onannounce} {away} />
<CompletedSection part="completed" {onopen} {oninstall} {onannounce} {away} />
