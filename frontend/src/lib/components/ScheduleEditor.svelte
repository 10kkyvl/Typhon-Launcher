<script lang="ts">
  import { Plus, Trash2 } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import type { ScheduleWindow } from '../services/settings';
  import { msg } from '../i18n';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import IconButton from './IconButton.svelte';

  let {
    windows,
    onchange,
  }: {
    windows: ScheduleWindow[];
    onchange: (next: ScheduleWindow[]) => void;
  } = $props();

  const weekdays = [
    { value: 1, label: 'settings.downloadsScheduleMon' },
    { value: 2, label: 'settings.downloadsScheduleTue' },
    { value: 3, label: 'settings.downloadsScheduleWed' },
    { value: 4, label: 'settings.downloadsScheduleThu' },
    { value: 5, label: 'settings.downloadsScheduleFri' },
    { value: 6, label: 'settings.downloadsScheduleSat' },
    { value: 0, label: 'settings.downloadsScheduleSun' },
  ] as const;

  // A rejected save must not take the half-edited row away from the user, so
  // the rows live here and the parent only learns of each change.
  let draft = $state<ScheduleWindow[]>(untrack(() => windows.map((w) => ({ ...w, days: [...w.days] }))));

  function emit() {
    onchange($state.snapshot(draft));
  }

  function toggleDay(index: number, day: number) {
    const days = draft[index].days;
    draft[index].days = days.includes(day) ? days.filter((d) => d !== day) : [...days, day].sort((a, b) => a - b);
    emit();
  }

  function setTime(index: number, field: 'from' | 'to', value: string) {
    draft[index][field] = value;
    emit();
  }

  function add() {
    draft.push({ days: [0, 1, 2, 3, 4, 5, 6], from: '01:00', to: '07:00' });
    emit();
  }

  function remove(index: number) {
    draft.splice(index, 1);
    emit();
  }
</script>

<div class="schedule">
  {#if draft.length === 0}
    <p class="empty">{msg('settings.downloadsScheduleEmpty')}</p>
  {:else}
    {#each draft as item, i (i)}
      <div class="row">
        <div class="days" role="group" aria-label={msg('settings.downloadsScheduleDays')}>
          {#each weekdays as day (day.value)}
            <Chip selected={item.days.includes(day.value)} onclick={() => toggleDay(i, day.value)}>
              {msg(day.label)}
            </Chip>
          {/each}
        </div>
        <div class="times">
          <input
            class="input time"
            type="time"
            aria-label={msg('settings.downloadsScheduleFrom')}
            value={item.from}
            onchange={(e) => setTime(i, 'from', e.currentTarget.value)}
          />
          <span class="dash" aria-hidden="true">—</span>
          <input
            class="input time"
            type="time"
            aria-label={msg('settings.downloadsScheduleTo')}
            value={item.to}
            onchange={(e) => setTime(i, 'to', e.currentTarget.value)}
          />
          <IconButton label={msg('settings.downloadsScheduleRemove')} size="sm" onclick={() => remove(i)}>
            <Trash2 size="1.5rem" strokeWidth={1.8} />
          </IconButton>
        </div>
      </div>
    {/each}
    <p class="hint">{msg('settings.downloadsScheduleOvernight')}</p>
  {/if}
  <div class="actions">
    <Button size="sm" onclick={add}>
      <Plus size="1.4rem" strokeWidth={2} />
      {msg('settings.downloadsScheduleAdd')}
    </Button>
  </div>
</div>

<style>
  .schedule {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3) var(--space-5);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
  }

  .days {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }

  .times {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .time {
    width: 12rem;
  }

  .dash {
    color: var(--text-3);
  }

  .empty,
  .hint {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .hint {
    font-size: var(--font-xs);
  }

  .actions {
    display: flex;
  }
</style>
