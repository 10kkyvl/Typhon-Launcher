<script lang="ts">
  import type { ReleaseChangeKind, ReleaseNote } from '../services/selfupdate';

  let { notes, currentVersion = '' }: { notes: ReleaseNote[]; currentVersion?: string } = $props();

  const kindLabels: Record<ReleaseChangeKind, string> = {
    added: 'Добавлено',
    changed: 'Изменено',
    fixed: 'Исправлено',
    removed: 'Удалено',
  };

  const kindOrder: ReleaseChangeKind[] = ['added', 'changed', 'fixed', 'removed'];

  function groups(note: ReleaseNote) {
    return kindOrder
      .map((kind) => ({ kind, items: (note.changes ?? []).filter((c) => c.kind === kind) }))
      .filter((group) => group.items.length > 0);
  }

  function publishedLabel(iso: string) {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '';
    return date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
  }
</script>

<div class="notes">
  {#each notes as note (note.version)}
    <article class="note">
      <header>
        <h4>Версия {note.version}</h4>
        {#if note.version === currentVersion}
          <span class="current">установлена</span>
        {/if}
        {#if publishedLabel(note.publishedAt)}
          <span class="date">{publishedLabel(note.publishedAt)}</span>
        {/if}
      </header>
      {#if note.summary}
        <p class="summary">{note.summary}</p>
      {/if}
      {#each groups(note) as group (group.kind)}
        <section class="group">
          <span class="kind kind-{group.kind}">{kindLabels[group.kind]}</span>
          <ul>
            {#each group.items as change, i (change.text + i)}
              <li>{change.text}</li>
            {/each}
          </ul>
        </section>
      {/each}
    </article>
  {/each}
</div>

<style>
  .notes {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .note {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  header {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  h4 {
    margin: 0;
    font-size: var(--font-md);
    font-weight: 600;
  }

  .current {
    padding: 0.1rem var(--space-2);
    border-radius: var(--radius-xs);
    background: var(--accent-subtle);
    color: var(--accent-text);
    font-size: var(--font-xs);
  }

  .date {
    margin-left: auto;
    color: var(--text-3);
    font-size: var(--font-xs);
  }

  .summary {
    margin: 0;
    color: var(--text-2);
    font-size: var(--font-sm);
    line-height: 1.5;
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .kind {
    align-self: flex-start;
    padding: 0.1rem var(--space-2);
    border-radius: var(--radius-xs);
    font-size: var(--font-xs);
    font-weight: 500;
  }

  .kind-added {
    background: var(--success-subtle);
    color: var(--success);
  }

  .kind-changed {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .kind-fixed {
    background: var(--warning-subtle);
    color: var(--warning);
  }

  .kind-removed {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  ul {
    margin: 0;
    padding-left: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  li {
    color: var(--text-2);
    font-size: var(--font-sm);
    line-height: 1.5;
  }
</style>
