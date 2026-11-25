<script lang="ts">
  import AppShell from '../components/AppShell.svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../components/PageHeader.svelte';
  import UserProfile from '../../routes/user/UserProfile.svelte';
  import { navigate, route } from '../stores/router';

  // The main window's shell with the user page routed as App.svelte routes it, and two stand-in pages
  // to leave it for, for an answer that arrives after the player went elsewhere
  // (routes/user/User.shell.dom.test.ts).
</script>

<AppShell>
  {#if $route.name === 'user'}
    <UserProfile username={$route.params.username} />
  {:else if $route.name === 'friends'}
    <PageHeader title="Friends" />
  {:else}
    <PageHeader title="Library" />
    <Button onclick={() => navigate('user', { username: 'ann' })}>Open Ann</Button>
  {/if}
</AppShell>
