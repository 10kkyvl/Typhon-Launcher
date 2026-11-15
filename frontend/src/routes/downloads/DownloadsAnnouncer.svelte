<script lang="ts">
  /**
   * The downloads screen's one polite live region. It says only what it is given, through `announce`,
   * which the page hands to its lists (what the player did: moved, removed, cleared); it says nothing
   * by itself and never progress. Each saying is a node of its own, so the same words said twice are
   * heard twice, and two sayings of one pass (a cancel and the move it caused) are both heard, in order;
   * a later pass replaces them.
   */
  let sayings = $state<{ id: number; text: string }[]>([]);
  let said = 0;
  let collecting = false;

  export function announce(text: string): void {
    if (!text) return;
    if (!collecting) {
      collecting = true;
      sayings = [];
      queueMicrotask(() => (collecting = false));
    }
    sayings.push({ id: ++said, text });
  }
</script>

<p class="sr-only" aria-live="polite">{#each sayings as saying (saying.id)}<span>{saying.text}</span>{/each}</p>
