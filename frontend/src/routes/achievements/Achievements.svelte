<script lang="ts">
  import { Trophy } from '@lucide/svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
</script>

<PageHeader title="Достижения" />

<EmptyState
  title="Достижений пока нет"
  description="Typhon не собирает достижения — источники не отдают их, а придумывать значения лаунчер не будет."
>
  {#snippet icon()}
    <Trophy size="2rem" strokeWidth={1.8} />
  {/snippet}
</EmptyState>
