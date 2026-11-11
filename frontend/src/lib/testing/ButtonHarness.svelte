<script lang="ts">
  import Button from '../components/Button.svelte';

  // A submit button in a form, its disabled state and reason set by the test, with handlers spread
  // onto it the way a menu trigger spreads its props. For Button.dom.test.ts.
  let {
    disabled = false,
    reason,
    onclick,
    onsubmit,
    onkeydown,
    onpointerdown,
  }: {
    disabled?: boolean;
    reason?: string;
    onclick?: () => void;
    onsubmit?: () => void;
    onkeydown?: () => void;
    onpointerdown?: () => void;
  } = $props();
</script>

<form
  onsubmit={(event) => {
    event.preventDefault();
    onsubmit?.();
  }}
>
  <input aria-label="Name" />
  <Button type="submit" {disabled} {reason} {onclick} {onkeydown} {onpointerdown}>Save</Button>
</form>
