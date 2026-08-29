<script lang="ts">
  import Toggle from '../../lib/components/Toggle.svelte';
  import { lanStats } from '../../lib/stores/lan';
  import { settings, updateSettings } from '../../lib/stores/settings';
</script>

<div class="row">
  <div class="row-text">
    <span class="row-label">Раздача игр по локальной сети</span>
    <span class="row-sub"
      >Функция не проверена в реальной сети — протестирован только режим на одном компьютере. Включение
      открывает сетевой порт для обмена с другими компьютерами в той же локальной сети; игры
      раздаются только им.</span
    >
  </div>
  <Toggle
    checked={$settings?.lanSharing ?? false}
    label="Раздача игр по локальной сети"
    onchange={(v) => updateSettings({ lanSharing: v })}
  />
</div>
{#if $settings?.lanSharing}
  <div class="row">
    <div class="row-text">
      <span class="row-label">Состояние сети</span>
      <span class="row-sub"
        >Известно компьютеров: {$lanStats.peersKnown} · раздач видно: {$lanStats.offersKnown} · раздаём
        сами: {$lanStats.sharesActive} · объявлений отправлено: {$lanStats.announcesSent}, получено:
        {$lanStats.announcesReceived}</span
      >
    </div>
  </div>
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
  }

  .row + .row {
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
</style>
