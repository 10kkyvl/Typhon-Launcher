<script lang="ts">
  import Card from '../Card.svelte';
  import EmptyState from '../EmptyState.svelte';
  import { msg } from '../../i18n';

  let { bio }: { bio: string } = $props();
</script>

<Card title={msg('profile.blockAbout')}>
  {#if bio.trim() === ''}
    <div class="none">
      <EmptyState size="inline" title={msg('profile.aboutEmpty')} />
    </div>
  {:else}
    <p class="bio">{bio}</p>
  {/if}
</Card>

<style>
  .none {
    padding: var(--space-4) 0;
  }

  .bio {
    margin: 0;
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
