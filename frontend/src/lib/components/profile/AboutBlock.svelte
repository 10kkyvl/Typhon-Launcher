<script lang="ts">
  import Card from '../Card.svelte';
  import { msg } from '../../i18n';

  let { bio }: { bio: string } = $props();
</script>

<Card title={msg('profile.blockAbout')}>
  <p class="bio">{bio}</p>
</Card>

<style>
  .bio {
    margin: 0;
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
