<script lang="ts">
  import { Users } from '@lucide/svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
</script>

<PageHeader title="Друзья" subtitle="Заявки, список друзей и заблокированные" />
<EmptyState title="Скоро" description="Раздел ещё готовится">
  {#snippet icon()}
    <Users size="2.2rem" strokeWidth={1.6} />
  {/snippet}
</EmptyState>
