<script lang="ts">
  import FeedRow from '../components/FeedRow.svelte';
  import { feedEvents, reactToEvent } from '../stores/feed';

  // The feed rows wired to the feed store as the activity page wires them: a reaction goes through
  // reactToEvent (optimistic change, pending state, rollback on failure).
</script>

{#each $feedEvents as event (event.id)}
  <FeedRow {event} onreact={(emoji) => void reactToEvent(event.id, emoji)} />
{/each}
