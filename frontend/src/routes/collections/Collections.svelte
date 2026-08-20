<script lang="ts">
  import { Bookmark, Plus } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { toast } from '../../lib/stores/toasts';
</script>

<PageHeader title="Коллекции" subtitle="Группируйте игры по собственным подборкам">
  {#snippet actions()}
    <Button variant="primary" onclick={() => toast('Создание коллекций недоступно в demo')}>
      <Plus size={16} strokeWidth={2} />
      Создать коллекцию
    </Button>
  {/snippet}
</PageHeader>

<div class="empty-wrap">
  <EmptyState
    title="Коллекций пока нет"
    description="Создайте первую коллекцию, чтобы объединить игры по жанру, настроению или прохождению."
  >
    {#snippet icon()}
      <Bookmark size={22} strokeWidth={1.8} />
    {/snippet}
    {#snippet actions()}
      <Button onclick={() => toast('Создание коллекций недоступно в demo')}>
        <Plus size={16} strokeWidth={2} />
        Создать коллекцию
      </Button>
    {/snippet}
  </EmptyState>
</div>

<style>
  .empty-wrap {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
</style>
