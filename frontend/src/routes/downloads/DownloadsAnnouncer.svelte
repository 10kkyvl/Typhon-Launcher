<script lang="ts">
  /**
   * The downloads screen's one polite live region. It says only what it is given, through `announce`,
   * which the page hands to its lists (what the player did: moved, removed, cleared); it says nothing
   * by itself and never progress. Each saying is a node of its own, so the same words said twice are
   * heard twice.
   */
  let said = $state('');
  let sayings = $state(0);

  export function announce(text: string): void {
    if (!text) return;
    said = text;
    sayings += 1;
  }
</script>

<p class="sr-only" aria-live="polite">{#key sayings}{#if said}<span>{said}</span>{/if}{/key}</p>
