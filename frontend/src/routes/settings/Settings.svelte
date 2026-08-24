<script lang="ts">
  import { FolderOpen, ListChecks, Trash2 } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import AvatarEditor from '../../lib/components/AvatarEditor.svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import LegalDocumentModal from '../../lib/components/LegalDocumentModal.svelte';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import MaskedEmail from '../../lib/components/MaskedEmail.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import Select from '../../lib/components/Select.svelte';
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import UpdateBanner from '../../lib/components/UpdateBanner.svelte';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { inWails } from '../../lib/services/backend';
  import { listLegalDocuments, type LegalMeta } from '../../lib/services/legal';
  import { openFolder, type Settings } from '../../lib/services/settings';
  import { getAppInfo, getSystemInfo, type AppInfo, type SystemInfo } from '../../lib/services/system';
  import { navigate } from '../../lib/stores/router';
  import { requestCheck, selfUpdateChecking, selfUpdateStatus } from '../../lib/stores/selfupdate';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { authState, currentUser, leaveGuest, signOut } from '../../lib/stores/user';
  import { bytesLabel, relativeDate } from '../../lib/utils/format';

  let tab = $state('general');

  const tabs = [
    { id: 'general', label: 'Общие' },
    { id: 'downloads', label: 'Загрузки' },
    { id: 'connection', label: 'Соединение' },
    { id: 'interface', label: 'Интерфейс' },
    { id: 'notifications', label: 'Уведомления' },
    { id: 'account', label: 'Аккаунт' },
    { id: 'about', label: 'О программе' },
  ];

  let overlay = $state(true);
  let autoUpdate = $state(true);
  let descriptions = $state(true);

  const current = $derived($settings);
  const scaleValue = $derived(String(Math.round(($settings?.uiScale ?? 1) * 100)));

  let appInfo = $state<AppInfo | null>(null);
  let systemInfo = $state<SystemInfo | null>(null);

  let legalDocs = $state<LegalMeta[]>([]);
  let legalError = $state('');
  let legalOpen = $state(false);
  let legalActiveId = $state<string | null>(null);
  let legalActiveTitle = $state('');
  let sourcesNoticeReviewOpen = $state(false);

  onMount(async () => {
    appInfo = await getAppInfo();
    systemInfo = await getSystemInfo();
    try {
      legalDocs = await listLegalDocuments();
    } catch {
      legalError = inWails ? 'Не удалось загрузить список документов.' : 'Правовые документы недоступны вне приложения.';
    }
  });

  function openLegalDoc(meta: LegalMeta) {
    legalActiveId = meta.id;
    legalActiveTitle = meta.title;
    legalOpen = true;
  }

  type PathKey = 'gamesPath' | 'downloadsPath' | 'screenshotsPath';

  const folderRows: { key: PathKey; label: string }[] = [
    { key: 'gamesPath', label: 'Игры' },
    { key: 'downloadsPath', label: 'Загрузки' },
    { key: 'screenshotsPath', label: 'Скриншоты' },
  ];

  let librarySetupOpen = $state(false);

  function set(patch: Partial<Settings>) {
    updateSettings(patch);
  }

  let avatarFailed = $state(false);
  let signingOut = $state(false);
  const isGuest = $derived($authState === 'guest');

  $effect(() => {
    $currentUser?.avatarUrl;
    avatarFailed = false;
  });

  const avatarInitial = $derived(
    $currentUser ? ($currentUser.displayName || $currentUser.username).slice(0, 1).toUpperCase() : '?',
  );

  const memberSince = $derived(
    $currentUser
      ? new Date($currentUser.createdAt).toLocaleDateString('ru-RU', {
          year: 'numeric',
          month: 'long',
          day: 'numeric',
        })
      : '',
  );

  async function onSignOut() {
    if (signingOut) return;
    signingOut = true;
    try {
      if (isGuest) await leaveGuest();
      else await signOut();
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось выйти'), 'danger');
    } finally {
      signingOut = false;
    }
  }

  function openLibrarySetup() {
    if (!inWails) {
      toast('Выбор папки доступен только в desktop-сборке');
      return;
    }
    librarySetupOpen = true;
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
    const id = current?.installCleanupPolicy ?? 'delete';
    return cleanupPolicyOptions.some((o) => o.id === id) ? id : 'delete';
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

<PageHeader title="Настройки" />

<div class="tabs-wrap">
  <Tabs {tabs} bind:value={tab} />
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
              <span class="row-label">Discord Rich Presence</span>
              <span class="row-sub">Показывать в Discord, во что вы играете</span>
            </div>
            <Toggle
              checked={current?.discordRichPresence ?? false}
              label="Discord Rich Presence"
              onchange={(v) => set({ discordRichPresence: v })}
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
        <h3>Библиотека</h3>
        <div class="rows">
          <div class="row folder-row">
            <span class="row-label folder-label">Папка библиотеки</span>
            <div class="folder-controls">
              <input
                class="input sm"
                type="text"
                readonly
                placeholder="Не настроена"
                value={current?.libraryPath ?? ''}
              />
              <Button size="sm" onclick={openLibrarySetup}>
                {current?.libraryPath ? 'Изменить' : 'Выбрать'}
              </Button>
              <IconButton
                label="Открыть папку"
                size="sm"
                disabled={!current?.libraryPath}
                onclick={() => openPath(current?.libraryPath)}
              >
                <FolderOpen size="1.6rem" strokeWidth={1.8} />
              </IconButton>
            </div>
          </div>
          {#if current?.libraryPath}
            {#each folderRows as folder (folder.key)}
              <div class="row folder-row">
                <span class="row-label folder-label">{folder.label}</span>
                <div class="folder-controls">
                  <input class="input sm" type="text" readonly value={current?.[folder.key] ?? ''} />
                  <IconButton label="Открыть папку" size="sm" onclick={() => openPath(current?.[folder.key])}>
                    <FolderOpen size="1.6rem" strokeWidth={1.8} />
                  </IconButton>
                </div>
              </div>
            {/each}
          {:else}
            <span class="row-sub">
              Игры, загрузки и скриншоты хранятся внутри папки библиотеки. Пока она не выбрана, скачивать нечего.
            </span>
          {/if}
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
              width="22rem"
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
              width="22rem"
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
              width="22rem"
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
            <span class="row-label">Отдавать во время загрузки</span>
            <span class="row-sub"
              >Разрешает передавать другим участникам уже загруженные части во время активной BitTorrent-загрузки.</span
            >
          </div>
          <Toggle
            checked={current?.uploadWhileDownloading ?? false}
            label="Отдача"
            onchange={(v) => set({ uploadWhileDownloading: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Раздавать после загрузки</span>
            <span class="row-sub">Продолжать отдавать завершённую загрузку другим участникам.</span>
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
            <span class="row-label">Не создавать ярлыки</span>
            <span class="row-sub"
              >Отклонять ярлыки на рабочем столе и папки в меню «Пуск», а созданные установщиком —
              удалять: игры запускаются из лаунчера</span
            >
          </div>
          <Toggle
            checked={current?.installSkipShortcuts ?? true}
            label="Без ярлыков"
            onchange={(v) => set({ installSkipShortcuts: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Отклонять дополнения установщика</span>
            <span class="row-sub"
              >DirectX, .NET, Visual C++, ассоциации файлов и прочие предложения. Если игра не
              запускается без них — выключите</span
            >
          </div>
          <Toggle
            checked={current?.installSkipExtras ?? true}
            label="Без дополнений"
            onchange={(v) => set({ installSkipExtras: v })}
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
          <input class="input sm port-input" type="text" bind:value={port} />
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
      {#if isGuest}
        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">Гостевой режим</span>
              <span class="row-sub">Аккаунта нет. Библиотека, загрузки и источники работают локально.</span>
            </div>
            <Button size="sm" variant="primary" disabled={signingOut} onclick={onSignOut}>
              {signingOut ? 'Открываю…' : 'Войти в аккаунт'}
            </Button>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Профиль</span>
              <span class="row-sub">Имя, аватар и email доступны только с аккаунтом.</span>
            </div>
            <Button size="sm" onclick={() => navigate('profile')}>Открыть профиль</Button>
          </div>
        </div>
      {:else if !$currentUser}
        <p class="row-sub">Вы не авторизованы.</p>
      {:else}
        <div class="account-card">
          <div class="account-avatar-block">
            {#if avatarFailed || !$currentUser.avatarUrl}
              <span class="account-avatar-fallback">{avatarInitial}</span>
            {:else}
              <img
                class="account-avatar"
                src={$currentUser.avatarUrl}
                alt=""
                draggable="false"
                onerror={() => (avatarFailed = true)}
              />
            {/if}
          </div>
          <div class="account-info">
            <span class="account-name">{$currentUser.displayName}</span>
            <span class="account-status">@{$currentUser.username}</span>
          </div>
          <div class="account-avatar-actions">
            <AvatarEditor size="sm" />
            <Button size="sm" onclick={() => navigate('profile')}>Открыть профиль</Button>
          </div>
        </div>

        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">Email</span>
              <span class="row-sub"><MaskedEmail email={$currentUser.email} /></span>
            </div>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Участник с</span>
              <span class="row-sub">{memberSince}</span>
            </div>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Сессия</span>
              <span class="row-sub">Хранится в диспетчере учётных данных Windows</span>
            </div>
            <Button size="sm" variant="danger" disabled={signingOut} onclick={onSignOut}>
              {signingOut ? 'Выход…' : 'Выйти'}
            </Button>
          </div>
        </div>
      {/if}
    </section>
  </div>
{:else if tab === 'about'}
  <div class="single-column">
    <section class="group about">
      <div class="about-logo">
        <img src="/typhon.png" alt="" width="44" height="44" draggable="false" />
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
            <span class="row-sub">
              {#if $selfUpdateChecking}
                Проверка обновлений…
              {:else}
                Установлена версия {$selfUpdateStatus.currentVersion || appInfo?.version || '—'}
                {#if $selfUpdateStatus.checkedAt}
                  · проверено {relativeDate($selfUpdateStatus.checkedAt)}
                {/if}
              {/if}
            </span>
          </div>
          <Button size="sm" disabled={$selfUpdateChecking} onclick={requestCheck}>
            <ListChecks size="1.5rem" strokeWidth={1.8} />
            {$selfUpdateChecking ? 'Проверка…' : 'Проверить'}
          </Button>
        </div>
        <UpdateBanner />
      </div>
    </section>

    <section class="group">
      <h3>Правовая информация</h3>
      {#if legalError}
        <p class="row-sub">{legalError}</p>
      {:else}
        <div class="rows">
          {#each legalDocs as meta (meta.id)}
            <div class="row">
              <div class="row-text">
                <span class="row-label">{meta.title}</span>
              </div>
              <Button size="sm" onclick={() => openLegalDoc(meta)}>Открыть</Button>
            </div>
          {/each}
          <div class="row">
            <div class="row-text">
              <span class="row-label">Уведомление об источниках</span>
              <span class="row-sub">Правила добавления сторонних источников релизов</span>
            </div>
            <Button size="sm" onclick={() => (sourcesNoticeReviewOpen = true)}>Открыть</Button>
          </div>
        </div>
      {/if}
    </section>
  </div>
{/if}

<LibrarySetupModal
  bind:open={librarySetupOpen}
  title={current?.libraryPath ? 'Сменить папку библиотеки' : 'Куда устанавливать игры'}
  note={current?.libraryPath
    ? 'Уже установленные игры останутся в прежней папке — лаунчер их не переносит. В новую библиотеку попадёт всё, что скачается дальше.'
    : ''}
/>

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

<LegalDocumentModal bind:open={legalOpen} documentId={legalActiveId} title={legalActiveTitle} />
<SourcesNoticeModal bind:open={sourcesNoticeReviewOpen} mode="review" />

<style>
  .tabs-wrap {
    margin-bottom: var(--space-8);
  }

  .columns {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--space-8);
    align-items: start;
    max-width: 96rem;
  }

  .column {
    min-width: 0;
  }

  .single-column {
    max-width: 96rem;
  }

  .group {
    margin-bottom: var(--space-10);
  }

  .group h3 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-3);
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

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

  .folder-row {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-2);
  }

  .folder-label {
    font-size: var(--font-sm);
  }

  .folder-controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .cleanup-controls {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex-shrink: 0;
  }

  .cleanup-size {
    font-size: var(--font-sm);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .danger-zone {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-5);
    margin-top: var(--space-4);
    padding: var(--space-4) var(--space-5);
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
  }

  .port-input {
    width: 12rem;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  .account-card {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-4);
    background: var(--surface);
    border-radius: var(--radius-lg);
    margin-bottom: var(--space-4);
  }

  .account-avatar-block {
    flex-shrink: 0;
    width: 6.4rem;
    height: 6.4rem;
  }

  .account-avatar {
    width: 6.4rem;
    height: 6.4rem;
    border-radius: 50%;
    object-fit: cover;
  }

  .account-avatar-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 6.4rem;
    height: 6.4rem;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--text-2);
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .account-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .account-name {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .account-status {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .account-avatar-actions {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    flex-shrink: 0;
    flex-wrap: wrap;
    justify-content: flex-end;
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

  .modal-text {
    font-size: var(--font-md);
    line-height: 1.55;
    color: var(--text-2);
    max-width: var(--prose-max);
  }

  @media (min-width: 1600px) {
    .columns {
      grid-template-columns: 1fr 1fr;
      gap: var(--space-12);
    }
  }
</style>
