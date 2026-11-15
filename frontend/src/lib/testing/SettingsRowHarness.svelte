<script lang="ts">
  import IntegerInput from '../components/IntegerInput.svelte';
  import Select from '../components/Select.svelte';
  import SettingsRow from '../components/SettingsRow.svelte';
  import Slider from '../components/Slider.svelte';
  import Toggle from '../components/Toggle.svelte';

  // Every base control a settings row holds, each wired through the ids the row hands it, and `many`
  // more rows to count ids over.
  let { many = 0, reason }: { many?: number; reason?: string } = $props();

  const OPTIONS = [
    { id: 'ru', label: 'Русский' },
    { id: 'en', label: 'English' },
  ];
</script>

<SettingsRow label="Запускать с системой" hint="Лаунчер откроется при входе">
  {#snippet children({ labelId, hintId })}
    <Toggle labelledby={labelId} describedby={hintId} />
  {/snippet}
</SettingsRow>

<SettingsRow label="Язык" hint="Язык интерфейса">
  {#snippet children({ labelId, hintId })}
    <Select options={OPTIONS} value="ru" labelledby={labelId} describedby={hintId} />
  {/snippet}
</SettingsRow>

<SettingsRow label="Масштаб" hint="Размер всего интерфейса" stack>
  {#snippet children({ hintId })}
    <Slider position={2} max={4} label="Масштаб" hideLabel valueText="100%" describedby={hintId} onchange={() => {}} />
  {/snippet}
</SettingsRow>

<SettingsRow label="Потоки загрузки" hint="Сколько частей качать сразу" labelFor="threads">
  {#snippet children({ hintId })}
    <IntegerInput id="threads" value={4} min={1} max={16} describedby={hintId} onchange={() => {}} />
  {/snippet}
</SettingsRow>

<SettingsRow label="Анимации" hint="Плавные переходы" note="Система просит меньше движения">
  {#snippet children({ labelId, hintId })}
    <Toggle labelledby={labelId} describedby={hintId} />
  {/snippet}
</SettingsRow>

<SettingsRow label="Оверлей" hint="Панель поверх игры" disabledReason={reason}>
  {#snippet children({ labelId, hintId, disabled })}
    <Toggle labelledby={labelId} describedby={hintId} {disabled} />
  {/snippet}
</SettingsRow>

<SettingsRow label="Папки библиотеки">
  {#snippet below({ labelId })}
    <ul aria-labelledby={labelId}>
      <li>D:\Games</li>
    </ul>
  {/snippet}
</SettingsRow>

{#each { length: many } as _, index (index)}
  <SettingsRow label={`Строка ${index}`} hint="Подсказка">
    {#snippet children({ labelId, hintId })}
      <Toggle labelledby={labelId} describedby={hintId} />
    {/snippet}
  </SettingsRow>
{/each}
