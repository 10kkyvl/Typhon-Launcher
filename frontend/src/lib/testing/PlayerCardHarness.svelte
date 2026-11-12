<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import PlayerCardPopover from '../components/PlayerCardPopover.svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import type { UserCard } from '../services/social';

  // A friend row's name with its hover card, between two other controls, a control with a hint of its
  // own after them, and a dialog a test opens over it. For PlayerCardPopover.dom.test.ts.
  let { other = false, beside = false }: { other?: boolean; beside?: boolean } = $props();

  let anchor = $state<HTMLElement>();
  const user: UserCard = { id: 'u1', username: 'ann', displayName: 'Ann Lee', avatarUrl: '' };
</script>

<section>
  <button type="button">Before</button>
  <button type="button" class="who" bind:this={anchor}>Ann Lee</button>
  <PlayerCardPopover {anchor} {user} {beside} />
  <button type="button">After</button>
  <div data-testid="unrelated" style="overflow: auto"><p>Elsewhere</p></div>
  <Tooltip text="Write to Ann"><button type="button">Message</button></Tooltip>
</section>

<Modal open={other} title="Other">
  <p>Over it</p>
</Modal>
