<script lang="ts">
  import { ArrowDown, ArrowUp } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { SHOWCASE_KINDS, type ProfileSettings, type ShowcaseKind } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { SHOWCASE_TITLES } from '../../lib/profile/view';
  import { isOffline, saveProfile, savingProfile } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';

  let {
    open = $bindable(false),
    settings,
  }: {
    open?: boolean;
    settings: ProfileSettings;
  } = $props();

  let draft = $state<ProfileSettings>(copy(settings));
  let error = $state('');

  function copy(value: ProfileSettings): ProfileSettings {
    return { ...value, showcase: [...value.showcase] };
  }

  $effect(() => {
    if (open) {
      draft = copy(settings);
      error = '';
    }
  });

  const flags: { key: keyof Omit<ProfileSettings, 'showcase'>; label: string; sub: string }[] = [
    { key: 'showOnline', label: 'Статус «В сети»', sub: 'Другие видят, что вы в лаунчере' },
    { key: 'showPlaying', label: 'Во что играю', sub: 'Текущая игра и список «Сейчас играю»' },
    { key: 'showActivity', label: 'Недавняя активность', sub: 'Сыгранные игры по дням, без времени запуска' },
    { key: 'showStats', label: 'Статистика', sub: 'Игры, часы, пройдено, играю сейчас' },
  ];

  const selected = $derived(draft.showcase);
  const unselected = $derived(SHOWCASE_KINDS.filter((kind) => !draft.showcase.includes(kind)));

  function add(kind: ShowcaseKind) {
    if (draft.showcase.length >= 3) return;
    draft.showcase = [...draft.showcase, kind];
  }

  function remove(kind: ShowcaseKind) {
    draft.showcase = draft.showcase.filter((k) => k !== kind);
  }

  function move(index: number, delta: number) {
    const next = [...draft.showcase];
    const target = index + delta;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    draft.showcase = next;
  }

  async function save() {
    if ($savingProfile || $isOffline) return;
    error = '';
    try {
      await saveProfile({ profile: $state.snapshot(draft) });
      open = false;
      toast('Настройки профиля сохранены', 'success');
    } catch (err) {
      error = accountErrorText(err, 'Не удалось сохранить');
    }
  }
</script>

<Modal bind:open title="Настройки профиля" width="52rem">
  {#if $isOffline}
    <p class="hint">Настройки профиля меняются только при связи с сервером.</p>
  {/if}

  <div class="group">
    <h4>Что видят другие</h4>
    <p class="hint">Пока профиль видите только вы. Галочки заработают, когда появятся друзья.</p>
    <div class="rows">
      {#each flags as flag (flag.key)}
        <div class="row">
          <div class="row-text">
            <span class="row-label">{flag.label}</span>
            <span class="row-sub">{flag.sub}</span>
          </div>
          <Toggle checked={draft[flag.key]} label={flag.label} disabled={$isOffline} onchange={(v) => (draft[flag.key] = v)} />
        </div>
      {/each}
    </div>
  </div>

  <div class="group">
    <h4>Витрина</h4>
    <p class="hint">До трёх блоков, в выбранном порядке.</p>
    <ul class="showcase">
      {#each selected as kind, index (kind)}
        <li class="showcase-row">
          <span class="row-label">{SHOWCASE_TITLES[kind]}</span>
          <span class="showcase-actions">
            <IconButton label="Выше" size="sm" disabled={index === 0 || $isOffline} onclick={() => move(index, -1)}>
              <ArrowUp size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton label="Ниже" size="sm" disabled={index === selected.length - 1 || $isOffline} onclick={() => move(index, 1)}>
              <ArrowDown size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <Button size="sm" variant="ghost" disabled={$isOffline} onclick={() => remove(kind)}>Убрать</Button>
          </span>
        </li>
      {/each}
      {#each unselected as kind (kind)}
        <li class="showcase-row muted">
          <span class="row-label">{SHOWCASE_TITLES[kind]}</span>
          <Button size="sm" disabled={selected.length >= 3 || $isOffline} onclick={() => add(kind)}>Добавить</Button>
        </li>
      {/each}
    </ul>
  </div>

  {#snippet footer()}
    {#if error}<span class="error">{error}</span>{/if}
    <Button variant="ghost" disabled={$savingProfile} onclick={() => (open = false)}>Отмена</Button>
    <Button variant="primary" disabled={$savingProfile || $isOffline} onclick={save}>
      {$savingProfile ? 'Сохранение…' : 'Сохранить'}
    </Button>
  {/snippet}
</Modal>

<style>
  .group + .group {
    margin-top: var(--space-6);
  }

  h4 {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: 0.4rem;
  }

  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
    margin-bottom: var(--space-3);
  }

  .rows,
  .showcase {
    display: flex;
    flex-direction: column;
    list-style: none;
  }

  .row,
  .showcase-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 1rem 0;
  }

  .row + .row,
  .showcase-row + .showcase-row {
    border-top: 1px solid var(--border);
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .row-label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .row-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .muted .row-label {
    color: var(--text-3);
  }

  .showcase-actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .error {
    font-size: var(--font-xs);
    color: var(--danger);
    margin-right: auto;
  }
</style>
