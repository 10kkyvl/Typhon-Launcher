<script lang="ts">
  import {
    Bell,
    Download,
    FolderOpen,
    Info,
    ListChecks,
    LogOut,
    Monitor,
    Settings as SettingsIcon,
    Trash2,
    UserRound,
    Wifi,
  } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import Select from '../../lib/components/Select.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { user } from '../../lib/mock/user';
  import { inWails } from '../../lib/services/backend';
  import { openFolder, selectFolder, type Settings } from '../../lib/services/settings';
  import { getAppInfo, getSystemInfo, type AppInfo, type SystemInfo } from '../../lib/services/system';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { bytesLabel } from '../../lib/utils/format';

  let tab = $state('general');

  const tabs = [
    { id: 'general', label: 'Общие', icon: SettingsIcon },
    { id: 'downloads', label: 'Загрузки', icon: Download },
    { id: 'connection', label: 'Соединение', icon: Wifi },
    { id: 'interface', label: 'Интерфейс', icon: Monitor },
    { id: 'notifications', label: 'Уведомления', icon: Bell },
    { id: 'account', label: 'Аккаунт', icon: UserRound },
    { id: 'about', label: 'О программе', icon: Info },
  ];

  let overlay = $state(true);
  let autoUpdate = $state(true);
  let descriptions = $state(true);

  const current = $derived($settings);
  const scaleValue = $derived(String(Math.round(($settings?.uiScale ?? 1) * 100)));

  let appInfo = $state<AppInfo | null>(null);
  let systemInfo = $state<SystemInfo | null>(null);

  onMount(async () => {
    appInfo = await getAppInfo();
    systemInfo = await getSystemInfo();
  });

  type PathKey = 'gamesPath' | 'downloadsPath' | 'screenshotsPath';

  const folderRows: { key: PathKey; label: string; title: string }[] = [
    { key: 'gamesPath', label: 'Папка с играми', title: 'Выберите папку с играми' },
    { key: 'downloadsPath', label: 'Папка загрузок', title: 'Выберите папку загрузок' },
    { key: 'screenshotsPath', label: 'Папка скриншотов', title: 'Выберите папку скриншотов' },
  ];

  function set(patch: Partial<Settings>) {
    updateSettings(patch);
  }

  async function browseFolder(key: PathKey, title: string) {
    if (!inWails) {
      toast('Выбор папки доступен только в desktop-сборке');
      return;
    }
    try {
      const path = await selectFolder(title);
      if (path) set({ [key]: path });
    } catch {
      toast('Не удалось открыть диалог выбора папки', 'danger');
    }
  }

  async function openPath(path: string | undefined) {
    if (!path) return;
    try {
      await openFolder(path);
    } catch {
      toast('Папка недоступна', 'danger');
    }
  }

  const MB = 1024 * 1024;

  const downloadLimitOptions = [
    { id: 'none', label: 'Без ограничений' },
    { id: '10', label: '10 МБ/с' },
    { id: '25', label: '25 МБ/с' },
    { id: '50', label: '50 МБ/с' },
  ];

  const uploadLimitOptions = [
    { id: 'none', label: 'Без ограничений' },
    { id: '1', label: '1 МБ/с' },
    { id: '5', label: '5 МБ/с' },
    { id: '10', label: '10 МБ/с' },
  ];

  const maxActiveOptions = [
    { id: '1', label: '1' },
    { id: '2', label: '2' },
    { id: '3', label: '3' },
    { id: '5', label: '5' },
  ];

  function rateId(bytes: number | undefined, options: { id: string }[]) {
    const id = String((bytes ?? 0) / MB);
    return options.some((o) => o.id === id) ? id : 'none';
  }

  const downloadLimit = $derived(rateId(current?.downloadRateLimit, downloadLimitOptions));
  const uploadLimit = $derived(rateId(current?.uploadRateLimit, uploadLimitOptions));
  const maxActiveValue = $derived.by(() => {
    const id = String(current?.maxActiveDownloads ?? 2);
    return maxActiveOptions.some((o) => o.id === id) ? id : '2';
  });

  const cleanupPolicyOptions = [
    { id: 'keep', label: 'Оставлять загруженные файлы' },
    { id: 'ask', label: 'Спрашивать' },
    { id: 'delete', label: 'Удалять после установки' },
  ];

  const cleanupPolicy = $derived.by(() => {
    const id = current?.installCleanupPolicy ?? 'keep';
    return cleanupPolicyOptions.some((o) => o.id === id) ? id : 'keep';
  });

  const sourceRefreshOptions = [
    { id: 'manual', label: 'Вручную' },
    { id: '1h', label: 'Каждый час' },
    { id: '6h', label: 'Каждые 6 часов' },
    { id: '12h', label: 'Каждые 12 часов' },
    { id: '24h', label: 'Раз в сутки' },
  ];

  const sourceRefreshInterval = $derived.by(() => {
    const id = current?.sourceRefreshInterval ?? '6h';
    return sourceRefreshOptions.some((o) => o.id === id) ? id : '6h';
  });

  const keepPreviousOptions = [
    { id: 'off', label: 'Не сохранять' },
    { id: 'first_launch', label: 'До первого успешного запуска' },
    { id: '24h', label: '24 часа' },
  ];

  const keepPreviousVersion = $derived.by(() => {
    const id = current?.keepPreviousVersion ?? 'first_launch';
    return keepPreviousOptions.some((o) => o.id === id) ? id : 'first_launch';
  });

  let scheduleEnabled = $state(false);

  let port = $state('42815');
  let upnp = $state(true);
  let proxy = $state(false);

  let notifyDone = $state(true);
  let notifyUpdates = $state(true);
  let notifyAchievements = $state(true);
  let notifySound = $state(false);

  let resetOpen = $state(false);

  const cleanupItems = [
    { id: 'cache', label: 'Очистить кэш', sub: 'Временные файлы приложений и загрузок', size: '2,45 ГБ' },
    { id: 'shaders', label: 'Очистить кэш шейдеров', sub: 'Скомпилированные шейдеры для игр', size: '1,12 ГБ' },
    { id: 'logs', label: 'Очистить журналы', sub: 'Файлы логов и диагностические данные', size: '156 МБ' },
  ];
</script>

<h1 class="page-title">Настройки</h1>

<div class="tabs">
  {#each tabs as t (t.id)}
    <button class="tab" class:selected={tab === t.id} onclick={() => (tab = t.id)}>
      <t.icon size="1.7rem" strokeWidth={1.8} />
      {t.label}
    </button>
  {/each}
</div>

{#if tab === 'general'}
  <div class="columns">
    <div class="column">
      <section class="group">
        <h3>Запуск и поведение</h3>
        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">Запускать Typhon при старте системы</span>
              <span class="row-sub">Приложение будет запускаться автоматически</span>
            </div>
            <Toggle
              checked={current?.launchOnStartup ?? false}
              label="Запускать при старте"
              onchange={(v) => set({ launchOnStartup: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Сворачивать в трей</span>
              <span class="row-sub">При закрытии окна сворачивать в область уведомлений</span>
            </div>
            <Toggle
              checked={current?.minimizeToTray ?? true}
              label="Сворачивать в трей"
              onchange={(v) => set({ minimizeToTray: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Показывать оверлей в игре</span>
              <span class="row-sub">Игровой оверлей для доступа к функциям Typhon</span>
            </div>
            <Toggle bind:checked={overlay} label="Оверлей" />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Аппаратное ускорение</span>
              <span class="row-sub">Использовать GPU для улучшения производительности</span>
            </div>
            <Toggle
              checked={current?.hardwareAcceleration ?? true}
              label="Аппаратное ускорение"
              onchange={(v) => set({ hardwareAcceleration: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Автоматические обновления</span>
              <span class="row-sub">Обновлять установленные игры в фоне</span>
            </div>
            <Toggle bind:checked={autoUpdate} label="Автообновления" />
          </div>
        </div>
      </section>

      <section class="group">
        <h3>Папки</h3>
        <div class="rows">
          {#each folderRows as folder (folder.key)}
            <div class="row folder-row">
              <span class="row-label folder-label">{folder.label}</span>
              <div class="folder-controls">
                <input
                  type="text"
                  value={current?.[folder.key] ?? ''}
                  onchange={(e) => set({ [folder.key]: e.currentTarget.value })}
                />
                <Button size="sm" onclick={() => browseFolder(folder.key, folder.title)}>Обзор</Button>
                <IconButton label="Открыть папку" size="sm" onclick={() => openPath(current?.[folder.key])}>
                  <FolderOpen size="1.6rem" strokeWidth={1.8} />
                </IconButton>
              </div>
            </div>
          {/each}
        </div>
      </section>
    </div>

    <div class="column">
      <section class="group">
        <h3>Интерфейс</h3>
        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">Тема</span>
              <span class="row-sub">Выберите внешний вид приложения</span>
            </div>
            <Select
              value={current?.theme ?? 'dark'}
              width="18rem"
              options={[
                { id: 'dark', label: 'Тёмная' },
                { id: 'system', label: 'Как в системе' },
              ]}
              onchange={(id) => set({ theme: id })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Язык</span>
              <span class="row-sub">Язык интерфейса Typhon</span>
            </div>
            <Select
              value={current?.language ?? 'ru'}
              width="18rem"
              options={[
                { id: 'ru', label: 'Русский' },
                { id: 'en', label: 'English' },
              ]}
              onchange={(id) => set({ language: id })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Размер интерфейса</span>
              <span class="row-sub">Масштаб элементов интерфейса</span>
            </div>
            <Select
              value={scaleValue}
              width="18rem"
              options={[
                { id: '90', label: '90%' },
                { id: '100', label: '100% (по умолчанию)' },
                { id: '110', label: '110%' },
                { id: '125', label: '125%' },
              ]}
              onchange={(id) => set({ uiScale: Number(id) / 100 })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Анимации интерфейса</span>
              <span class="row-sub">Включить плавные переходы и анимации</span>
            </div>
            <Toggle
              checked={current?.animationsEnabled ?? true}
              label="Анимации"
              onchange={(v) => set({ animationsEnabled: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Показывать описания игр</span>
              <span class="row-sub">Отображать описания на карточках игр в библиотеке</span>
            </div>
            <Toggle bind:checked={descriptions} label="Описания игр" />
          </div>
        </div>
      </section>

      <section class="group">
        <h3>Очистка данных</h3>
        <div class="rows">
          {#each cleanupItems as item (item.id)}
            <div class="row">
              <div class="row-text">
                <span class="row-label">{item.label}</span>
                <span class="row-sub">{item.sub}</span>
              </div>
              <div class="cleanup-controls">
                <span class="cleanup-size">{item.size}</span>
                <Button size="sm" onclick={() => toast(`${item.label.replace('Очистить ', '')}: очищено`, 'success')}>
                  Очистить
                </Button>
              </div>
            </div>
          {/each}
        </div>
        <div class="danger-zone">
          <div class="row-text">
            <span class="row-label">Все данные приложения</span>
            <span class="row-sub">Сбросить настройки и удалить все данные Typhon</span>
          </div>
          <Button variant="danger" onclick={() => (resetOpen = true)}>
            <Trash2 size="1.5rem" strokeWidth={1.8} />
            Сбросить
          </Button>
        </div>
      </section>
    </div>
  </div>
{:else if tab === 'downloads'}
  <div class="single-column">
    <section class="group">
      <h3>Загрузки</h3>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">Ограничение скорости загрузки</span>
            <span class="row-sub">Максимальная скорость входящего трафика</span>
          </div>
          <Select
            value={downloadLimit}
            width="20rem"
            options={downloadLimitOptions}
            onchange={(id) => set({ downloadRateLimit: id === 'none' ? 0 : Number(id) * MB })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Ограничение скорости отдачи</span>
            <span class="row-sub">Максимальная скорость исходящего трафика</span>
          </div>
          <Select
            value={uploadLimit}
            width="20rem"
            options={uploadLimitOptions}
            onchange={(id) => set({ uploadRateLimit: id === 'none' ? 0 : Number(id) * MB })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Одновременные загрузки</span>
            <span class="row-sub">Количество активных загрузок</span>
          </div>
          <Select
            value={maxActiveValue}
            width="20rem"
            options={maxActiveOptions}
            onchange={(id) => set({ maxActiveDownloads: Number(id) })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Раздавать после загрузки</span>
            <span class="row-sub">Продолжать отдачу завершённых загрузок</span>
          </div>
          <Toggle
            checked={current?.seedAfterDownload ?? false}
            label="Раздача"
            onchange={(v) => set({ seedAfterDownload: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Загружать по расписанию</span>
            <span class="row-sub">Ограничивать загрузки в определённые часы</span>
          </div>
          <Toggle bind:checked={scheduleEnabled} label="Расписание" />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Обновление источников</span>
            <span class="row-sub">Как часто проверять источники на новые релизы</span>
          </div>
          <Select
            value={sourceRefreshInterval}
            width="20rem"
            options={sourceRefreshOptions}
            onchange={(id) => set({ sourceRefreshInterval: id })}
          />
        </div>
      </div>
    </section>

    <section class="group">
      <h3>Установка</h3>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">После установки</span>
            <span class="row-sub">Что делать с загруженными файлами</span>
          </div>
          <Select
            value={cleanupPolicy}
            width="26rem"
            options={cleanupPolicyOptions}
            onchange={(id) => set({ installCleanupPolicy: id })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Автоустановка portable/архивов</span>
            <span class="row-sub">Устанавливать сразу после завершения загрузки</span>
          </div>
          <Toggle
            checked={current?.autoInstall ?? false}
            label="Автоустановка"
            onchange={(v) => set({ autoInstall: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Проверять установку после завершения</span>
            <span class="row-sub">Искать исполняемый файл и проверять содержимое папки</span>
          </div>
          <Toggle
            checked={current?.verifyAfterInstall ?? true}
            label="Проверка установки"
            onchange={(v) => set({ verifyAfterInstall: v })}
          />
        </div>
      </div>
    </section>

    <section class="group">
      <h3>Обновления</h3>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">Проверять автоматически</span>
            <span class="row-sub">Искать новые релизы после обновления источников</span>
          </div>
          <Toggle
            checked={current?.updateCheckAutomatically ?? true}
            label="Проверка обновлений"
            onchange={(v) => set({ updateCheckAutomatically: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Скачивать обновления автоматически</span>
            <span class="row-sub">Данные загружаются заранее, установка остаётся ручной</span>
          </div>
          <Toggle
            checked={current?.updateAutoDownload ?? false}
            label="Автозагрузка обновлений"
            onchange={(v) => set({ updateAutoDownload: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Устанавливать автоматически</span>
            <span class="row-sub">Пока недоступно: установка всегда подтверждается вручную</span>
          </div>
          <Toggle checked={false} disabled label="Автоустановка обновлений" />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Резервная копия сохранений</span>
            <span class="row-sub">Создавать снимок сохранений, когда их расположение известно</span>
          </div>
          <Toggle
            checked={current?.updateSaveBackup ?? true}
            label="Резервная копия"
            onchange={(v) => set({ updateSaveBackup: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Хранить предыдущую версию</span>
            <span class="row-sub">Позволяет откатиться, если обновление оказалось неудачным</span>
          </div>
          <Select
            value={keepPreviousVersion}
            width="28rem"
            options={keepPreviousOptions}
            onchange={(id) => set({ keepPreviousVersion: id })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Переиспользовать файлы торрента</span>
            <span class="row-sub">Скачивать только изменившиеся блоки, когда раскладка совпадает</span>
          </div>
          <Toggle
            checked={current?.allowTorrentReuse ?? true}
            label="Повторное использование файлов"
            onchange={(v) => set({ allowTorrentReuse: v })}
          />
        </div>
      </div>
    </section>
  </div>
{:else if tab === 'connection'}
  <div class="single-column">
    <section class="group">
      <h3>Соединение</h3>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">Порт для входящих соединений</span>
            <span class="row-sub">Используется для обмена данными с пирами</span>
          </div>
          <input class="port-input" type="text" bind:value={port} />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">UPnP / NAT-PMP</span>
            <span class="row-sub">Автоматически пробрасывать порт на маршрутизаторе</span>
          </div>
          <Toggle bind:checked={upnp} label="UPnP" />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Прокси-сервер</span>
            <span class="row-sub">Направлять трафик через прокси</span>
          </div>
          <Toggle bind:checked={proxy} label="Прокси" />
        </div>
      </div>
    </section>
  </div>
{:else if tab === 'interface'}
  <div class="single-column">
    <section class="group">
      <h3>Интерфейс</h3>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">Тема</span>
          </div>
          <Select
            value={current?.theme ?? 'dark'}
            width="20rem"
            options={[
              { id: 'dark', label: 'Тёмная' },
              { id: 'system', label: 'Как в системе' },
            ]}
            onchange={(id) => set({ theme: id })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Размер интерфейса</span>
          </div>
          <Select
            value={scaleValue}
            width="20rem"
            options={[
              { id: '90', label: '90%' },
              { id: '100', label: '100% (по умолчанию)' },
              { id: '110', label: '110%' },
              { id: '125', label: '125%' },
            ]}
            onchange={(id) => set({ uiScale: Number(id) / 100 })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Анимации интерфейса</span>
          </div>
          <Toggle
            checked={current?.animationsEnabled ?? true}
            label="Анимации"
            onchange={(v) => set({ animationsEnabled: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Показывать описания игр</span>
          </div>
          <Toggle bind:checked={descriptions} label="Описания" />
        </div>
      </div>
    </section>
  </div>
{:else if tab === 'notifications'}
  <div class="single-column">
    <section class="group">
      <h3>Уведомления</h3>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">Завершение загрузки</span>
            <span class="row-sub">Уведомлять, когда игра установлена</span>
          </div>
          <Toggle bind:checked={notifyDone} label="Загрузки" />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Обновления игр</span>
            <span class="row-sub">Уведомлять о доступных обновлениях</span>
          </div>
          <Toggle bind:checked={notifyUpdates} label="Обновления" />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Достижения</span>
            <span class="row-sub">Показывать уведомления о полученных достижениях</span>
          </div>
          <Toggle bind:checked={notifyAchievements} label="Достижения" />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Звук уведомлений</span>
            <span class="row-sub">Воспроизводить звук при уведомлениях</span>
          </div>
          <Toggle bind:checked={notifySound} label="Звук" />
        </div>
      </div>
    </section>
  </div>
{:else if tab === 'account'}
  <div class="single-column">
    <section class="group">
      <h3>Аккаунт</h3>
      <div class="account-card">
        <img class="account-avatar" src={user.avatar} alt="" draggable="false" />
        <div class="account-info">
          <span class="account-name">{user.name}</span>
          <span class="account-status">{user.status} · Локальный профиль</span>
        </div>
        <Button onclick={() => toast('Редактирование профиля недоступно в demo')}>Изменить профиль</Button>
      </div>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">Синхронизация сохранений</span>
            <span class="row-sub">Резервное копирование сохранений между устройствами</span>
          </div>
          <Toggle checked={false} label="Синхронизация" onchange={() => toast('Синхронизация недоступна в demo')} />
        </div>
      </div>
      <div class="group-foot">
        <Button variant="danger" onclick={() => toast('Выход недоступен в demo')}>
          <LogOut size="1.5rem" strokeWidth={1.8} />
          Выйти из профиля
        </Button>
      </div>
    </section>
  </div>
{:else if tab === 'about'}
  <div class="single-column">
    <section class="group about">
      <div class="about-logo">
        <img src="/typhon.svg" alt="" width="44" height="44" draggable="false" />
        <div>
          <h3>Typhon Launcher</h3>
          <span class="row-sub">Версия {appInfo?.version ?? '—'} · {appInfo?.platform ?? ''}/{appInfo?.arch ?? ''}</span>
        </div>
      </div>
      <div class="rows">
        {#if systemInfo}
          <div class="row">
            <div class="row-text">
              <span class="row-label">Система</span>
              <span class="row-sub">{systemInfo.os}</span>
            </div>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Процессор</span>
              <span class="row-sub">{systemInfo.cpu} · {systemInfo.cores} потоков</span>
            </div>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Память</span>
              <span class="row-sub">{bytesLabel(systemInfo.ramBytes)}</span>
            </div>
          </div>
        {/if}
        <div class="row">
          <div class="row-text">
            <span class="row-label">Проверить обновления клиента</span>
            <span class="row-sub">Установлена последняя версия</span>
          </div>
          <Button size="sm" onclick={() => toast('У вас последняя версия', 'success')}>
            <ListChecks size="1.5rem" strokeWidth={1.8} />
            Проверить
          </Button>
        </div>
      </div>
      <div class="about-links">
        <button class="about-link" onclick={() => toast('Недоступно в demo')}>Условия использования</button>
        <span class="about-sep">·</span>
        <button class="about-link" onclick={() => toast('Недоступно в demo')}>Политика конфиденциальности</button>
      </div>
    </section>
  </div>
{/if}

<footer class="footer">
  <span>Typhon Launcher {appInfo?.version ?? ''}</span>
  <div class="footer-links">
    <button class="about-link" onclick={() => toast('Недоступно в demo')}>Условия использования</button>
    <span class="about-sep">|</span>
    <button class="about-link" onclick={() => toast('Недоступно в demo')}>Политика конфиденциальности</button>
  </div>
</footer>

<Modal bind:open={resetOpen} title="Сбросить все данные?">
  <p class="modal-text">
    Настройки, кэш и локальные данные Typhon будут удалены. Установленные игры останутся на диске. Это действие нельзя
    отменить.
  </p>
  {#snippet footer()}
    <Button onclick={() => (resetOpen = false)}>Отмена</Button>
    <Button
      variant="danger"
      onclick={() => {
        resetOpen = false;
        toast('Сброс недоступен в demo', 'danger');
      }}
    >
      Сбросить все данные
    </Button>
  {/snippet}
</Modal>

<style>
  .page-title {
    font-size: var(--font-title);
    letter-spacing: -0.01em;
    margin: var(--space-4) 0 var(--space-5);
  }

  .tabs {
    display: flex;
    gap: 0.4rem;
    padding: 0.4rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    width: fit-content;
    max-width: 100%;
    overflow-x: auto;
    margin-bottom: var(--space-6);
  }

  .tab {
    display: inline-flex;
    align-items: center;
    gap: 0.8rem;
    height: 4.4rem;
    padding: 0 1.6rem;
    border-radius: 0.8rem;
    font-size: 1.5rem;
    font-weight: 500;
    color: var(--text-2);
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .tab:hover {
    color: var(--text);
  }

  .tab.selected {
    background: var(--accent);
    color: #fff;
  }

  .columns {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-5);
    align-items: start;
  }

  .column {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    min-width: 0;
  }

  .single-column {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    max-width: 72rem;
  }

  .group {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-5) var(--space-6);
  }

  .group h3 {
    font-size: 1.7rem;
    margin-bottom: var(--space-4);
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-5);
    padding: 1.2rem 0;
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
    font-size: 1.5rem;
    font-weight: 500;
  }

  .row-sub {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .folder-row {
    flex-direction: column;
    align-items: stretch;
    gap: 0.8rem;
  }

  .folder-label {
    font-size: 1.4rem;
  }

  .folder-controls {
    display: flex;
    gap: 0.8rem;
  }

  .folder-controls input {
    flex: 1;
    min-width: 0;
    height: 3.6rem;
    padding: 0 1.1rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    font-size: 1.4rem;
    color: var(--text-2);
    outline: none;
    transition: border-color var(--dur) var(--ease);
  }

  .folder-controls input:hover {
    border-color: var(--border-strong);
  }

  .folder-controls input:focus {
    border-color: rgba(104, 117, 232, 0.55);
    color: var(--text);
  }

  .group-foot {
    margin-top: var(--space-4);
  }

  .cleanup-controls {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex-shrink: 0;
  }

  .cleanup-size {
    font-size: 1.4rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .danger-zone {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-5);
    margin-top: var(--space-4);
    padding: var(--space-4);
    border: 1px solid rgba(217, 105, 105, 0.25);
    border-radius: var(--radius-md);
    background: rgba(217, 105, 105, 0.05);
  }

  .port-input {
    width: 12rem;
    height: 4rem;
    padding: 0 1.2rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: 1.5rem;
    color: var(--text);
    outline: none;
    text-align: right;
    font-variant-numeric: tabular-nums;
    transition: border-color var(--dur) var(--ease);
  }

  .port-input:focus {
    border-color: rgba(104, 117, 232, 0.55);
  }

  .account-card {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-4);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    margin-bottom: var(--space-4);
  }

  .account-avatar {
    width: 5.2rem;
    height: 5.2rem;
    border-radius: 50%;
    object-fit: cover;
  }

  .account-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .account-name {
    font-size: 1.6rem;
    font-weight: 600;
  }

  .account-status {
    font-size: 1.4rem;
    color: var(--text-3);
  }

  .about-logo {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .about-logo h3 {
    margin-bottom: 2px;
  }

  .about-links {
    margin-top: var(--space-4);
  }

  .about-link {
    font-size: 1.3rem;
    color: var(--text-3);
    border-radius: 0.4rem;
    transition: color var(--dur) var(--ease);
  }

  .about-link:hover {
    color: var(--text-2);
  }

  .about-sep {
    margin: 0 0.8rem;
    color: var(--text-3);
  }

  .footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: var(--space-8);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .modal-text {
    font-size: 1.5rem;
    line-height: 1.55;
    color: var(--text-2);
  }

  @media (max-width: 1240px) {
    .columns {
      grid-template-columns: 1fr;
    }
  }
</style>
