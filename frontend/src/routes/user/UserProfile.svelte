<script lang="ts">
  import { UserRound } from '@lucide/svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';

  let { username }: { username?: string } = $props();
</script>

<PageHeader title={username ? `@${username}` : 'Профиль'} />
<EmptyState title="Скоро" description="Профиль пользователя ещё готовится">
  {#snippet icon()}
    <UserRound size="2.2rem" strokeWidth={1.6} />
  {/snippet}
</EmptyState>
