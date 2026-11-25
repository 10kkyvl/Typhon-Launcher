<script lang="ts">
  import AppShell from '../components/AppShell.svelte';
  import Support from '../../routes/support/Support.svelte';
  import { route } from '../stores/router';

  // The main window's shell with the support page routed as App.svelte routes it, for the focus the
  // shell gives a page after a change (routes/support/Support.focus.dom.test.ts).
</script>

<AppShell>
  {#if $route.name === 'support'}
    <Support ticket={$route.params.ticket} view={$route.params.view} topic={$route.params.topic} subject={$route.params.subject} />
  {/if}
</AppShell>
