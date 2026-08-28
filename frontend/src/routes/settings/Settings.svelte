<script lang="ts">
  import { Download, Eye, FolderOpen, ListChecks, RefreshCw, ScrollText, Trash2 } from '@lucide/svelte';
  import { onMount, untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import LegalDocumentModal from '../../lib/components/LegalDocumentModal.svelte';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import RateLimitInput from '../../lib/components/RateLimitInput.svelte';
  import Select from '../../lib/components/Select.svelte';
  import SentDataModal from '../../lib/components/SentDataModal.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import ReleaseNotesList from '../../lib/components/ReleaseNotesList.svelte';
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import UpdateBanner from '../../lib/components/UpdateBanner.svelte';
  import { forgetRemote, syncNow } from '../../lib/services/accountSync';
  import { accountSyncReason } from '../../lib/services/accountSyncMessages';
  import { inWails } from '../../lib/services/backend';
  import { listLegalDocuments, type LegalMeta } from '../../lib/services/legal';
  import { logsReason } from '../../lib/services/logsMessages';
  import { getSettings, openFolder, type Settings } from '../../lib/services/settings';
  import {
    exportLogs,
    getAppInfo,
    getSystemInfo,
    type AppInfo,
    type LogBundle,
    type SystemInfo,
  } from '../../lib/services/system';
  import { releaseNotesHistory, requestCheck, selfUpdateChecking, selfUpdateStatus } from '../../lib/stores/selfupdate';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { authState } from '../../lib/stores/user';
  import { bytesLabel, relativeDate } from '../../lib/utils/format';

  let { tab: initialTab }: { tab?: string } = $props();

  const tabs = [
    { id: 'general', label: 'Общие' },
    { id: 'downloads', label: 'Загрузки' },
    { id: 'about', label: 'О программе' },
  ];

  let tab = $state('general');

  $effect(() => {
    const next = initialTab;
    untrack(() => {
      if (next && tabs.some((t) => t.id === next)) tab = next;
    });
  });

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
  let sentDataOpen = $state(false);

  let logsBundle = $state<LogBundle | null>(null);
  let logsSaving = $state(false);

  const accountReady = $derived($authState === 'authenticated');
  let syncingNow = $state(false);
  let forgettingRemote = $state(false);

  onMount(async () => {
    appInfo = await getAppInfo();
    systemInfo = await getSystemInfo();
    try {
      legalDocs = await listLegalDocuments();
    } catch {
      legalError = inWails ? 'Не удалось загрузить список документов.' : 'Правовые документы недоступны вне приложения.';
    }
  });

  async function saveLogs() {
    if (!inWails) {
      toast('Логи доступны только в desktop-сборке');
      return;
    }
    logsSaving = true;
    try {
      const bundle = await exportLogs();
      logsBundle = bundle;
      toast('Логи сохранены в папку «Загрузки»', 'success');
      try {
        await openFolder(bundle.dir);
      } catch {
        toast(bundle.path);
      }
    } catch (err) {
      logsBundle = null;
      toast(logsReason(err), 'danger');
    } finally {
      logsSaving = false;
    }
  }

  async function runSyncNow() {
    if (syncingNow) return;
    syncingNow = true;
    try {
      await syncNow();
      toast('Синхронизация выполнена', 'success');
    } catch (err) {
      toast(accountSyncReason(err, 'Не удалось синхронизировать данные'), 'danger');
    } finally {
      syncingNow = false;
    }
  }

  async function runForgetRemote() {
    if (forgettingRemote) return;
    if (!window.confirm('Удалить синхронизированные данные с сервера? Это действие необратимо.')) return;
    forgettingRemote = true;
    try {
      await forgetRemote();
      settings.set(await getSettings());
      toast('Данные удалены с сервера', 'success');
    } catch (err) {
      toast(accountSyncReason(err, 'Не удалось удалить данные с сервера'), 'danger');
    } finally {
      forgettingRemote = false;
    }
  }

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
  let historyOpen = $state(false);

  function set(patch: Partial<Settings>) {
    updateSettings(patch);
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

  const downloadLimitPresets = [10, 25, 50];
  const uploadLimitPresets = [1, 5, 10];

  const maxActiveOptions = [
    { id: '1', label: '1' },
    { id: '2', label: '2' },
    { id: '3', label: '3' },
    { id: '5', label: '5' },
  ];

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
              <span class="row-label">Запускать при старте системы</span>
              <span class="row-sub">Открывать Typhon при входе в Windows</span>
            </div>
            <Toggle
              checked={current?.launchOnStartup ?? false}
              label="Запуск при старте системы"
              onchange={(v) => set({ launchOnStartup: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Сворачивать в трей</span>
              <span class="row-sub">При закрытии окна прятать лаунчер в область уведомлений</span>
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
              <span class="row-label">Аппаратное ускорение</span>
              <span class="row-sub">Отрисовка интерфейса через GPU. Применится после перезапуска лаунчера</span>
            </div>
            <Toggle
              checked={current?.hardwareAcceleration ?? true}
              label="Аппаратное ускорение"
              onchange={(v) => set({ hardwareAcceleration: v })}
            />
          </div>
        </div>
      </section>

      <section class="group">
        <h3>Приватность</h3>
        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">Анонимная статистика использования</span>
              <span class="row-sub"
                >Отправлять анонимные события о запусках игр, загрузках, установках и обновлениях: только
                идентификатор игры, длительность, объём и код ошибки. По умолчанию выключено.</span
              >
            </div>
            <Toggle
              checked={current?.anonymousUsageStats ?? false}
              label="Анонимная статистика использования"
              onchange={(v) => set({ anonymousUsageStats: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Анонимная диагностика</span>
              <span class="row-sub"
                >Отправляет обезличенные сведения об ошибках и сбоях Typhon для диагностики. Перед отправкой из них
                удаляются пути, имя устройства и сетевые адреса.</span
              >
            </div>
            <Toggle
              checked={current?.anonymousDiagnostics ?? false}
              label="Анонимная диагностика"
              onchange={(v) => set({ anonymousDiagnostics: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Показать отправленные данные</span>
              <span class="row-sub"
                >Последние события и отчёты, ушедшие на сервер, в том виде, в котором они были отправлены</span
              >
            </div>
            <Button size="sm" onclick={() => (sentDataOpen = true)}>
              <Eye size="1.5rem" strokeWidth={1.8} />
              Показать
            </Button>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Состояние сессии</span>
              <span class="row-sub"
                >Typhon передаёт минимальное анонимное состояние активной сессии — включён ли лаунчер и
                идентификатор игры, если она запущена, — для агрегированной статистики сервиса в реальном
                времени. Эти данные не связаны с учётной записью, хранятся недолго и не зависят от переключателя
                выше.</span
              >
              <button
                type="button"
                class="privacy-link"
                onclick={() => openLegalDoc({ id: 'privacy', title: 'Политика конфиденциальности' })}
              >
                Политика конфиденциальности
              </button>
            </div>
          </div>
        </div>
      </section>

      <section class="group">
        <h3>Синхронизация</h3>
        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">Синхронизация между устройствами</span>
              <span class="row-sub"
                >Переносит между вашими устройствами часть настроек приложения, список игр каталога, дату
                последнего запуска и наигранное время. Источники, ссылки на релизы, их названия, пути на
                диске, лимиты скорости и согласия на сбор статистики и диагностики не передаются. По
                умолчанию выключено.</span
              >
              {#if !accountReady}
                <span class="row-sub">Нужен вход в аккаунт — в гостевом режиме синхронизация недоступна.</span>
              {/if}
            </div>
            <Toggle
              checked={current?.accountSync ?? false}
              label="Синхронизация между устройствами"
              disabled={!accountReady}
              onchange={(v) => set({ accountSync: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Синхронизировать сейчас</span>
              <span class="row-sub">Отправить и получить изменения немедленно, не дожидаясь фонового цикла</span>
            </div>
            <Button size="sm" disabled={!accountReady || !current?.accountSync || syncingNow} onclick={runSyncNow}>
              <RefreshCw size="1.5rem" strokeWidth={1.8} />
              {syncingNow ? 'Синхронизация…' : 'Синхронизировать сейчас'}
            </Button>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">Удалить данные с сервера</span>
              <span class="row-sub"
                >Необратимо удаляет всё, что синхронизировано с сервером, и выключает синхронизацию</span
              >
            </div>
            <Button
              size="sm"
              variant="danger"
              disabled={!accountReady || forgettingRemote}
              onclick={runForgetRemote}
            >
              <Trash2 size="1.5rem" strokeWidth={1.8} />
              {forgettingRemote ? 'Удаляем…' : 'Удалить данные с сервера'}
            </Button>
          </div>
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
          <RateLimitInput
            value={current?.downloadRateLimit ?? 0}
            presets={downloadLimitPresets}
            onchange={(bytes) => set({ downloadRateLimit: bytes })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">Ограничение скорости отдачи</span>
            <span class="row-sub">Максимальная скорость исходящего трафика</span>
          </div>
          <RateLimitInput
            value={current?.uploadRateLimit ?? 0}
            presets={uploadLimitPresets}
            onchange={(bytes) => set({ uploadRateLimit: bytes })}
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
            <span class="row-label">Ярлыки на рабочем столе</span>
            <span class="row-sub">Создавать ярлык игры после установки</span>
          </div>
          <Toggle
            checked={current?.desktopShortcuts ?? true}
            label="Ярлыки игр"
            onchange={(v) => set({ desktopShortcuts: v })}
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
        <div class="row">
          <div class="row-text">
            <span class="row-label">История обновлений</span>
            <span class="row-sub">
              {#if $releaseNotesHistory.length > 0}
                Что менялось в лаунчере от версии к версии
              {:else}
                Появится после первой проверки обновлений
              {/if}
            </span>
          </div>
          <Button size="sm" disabled={$releaseNotesHistory.length === 0} onclick={() => (historyOpen = true)}>
            <ScrollText size="1.5rem" strokeWidth={1.8} />
            Что нового
          </Button>
        </div>
        <UpdateBanner />
      </div>
    </section>

    <section class="group">
      <h3>Диагностика</h3>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">Логи лаунчера</span>
            <span class="row-sub">
              {#if logsBundle}
                {logsBundle.name} · {bytesLabel(logsBundle.sizeBytes)} — приложите архив к сообщению об ошибке
              {:else}
                Архив с журналом сохранится в папку «Загрузки» — приложите его к сообщению об ошибке
              {/if}
            </span>
          </div>
          <Button size="sm" disabled={logsSaving} onclick={saveLogs}>
            <Download size="1.5rem" strokeWidth={1.8} />
            {logsSaving ? 'Сохраняем…' : 'Скачать логи'}
          </Button>
        </div>
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

<LegalDocumentModal bind:open={legalOpen} documentId={legalActiveId} title={legalActiveTitle} />
<SourcesNoticeModal bind:open={sourcesNoticeReviewOpen} mode="review" />
<SentDataModal bind:open={sentDataOpen} />
<Modal bind:open={historyOpen} title="История обновлений" width="52rem">
  <ReleaseNotesList notes={$releaseNotesHistory} currentVersion={$selfUpdateStatus.currentVersion} />
</Modal>

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

  .about-logo {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .about-logo h3 {
    margin-bottom: 2px;
  }

  .privacy-link {
    align-self: flex-start;
    color: var(--accent-text);
    font-size: var(--font-xs);
    font-weight: 500;
    padding: 0.2rem 0.3rem;
    margin-left: -0.3rem;
    border-radius: var(--radius-xs);
  }

  .privacy-link:hover {
    color: var(--text);
  }

  @media (min-width: 1600px) {
    .columns {
      grid-template-columns: 1fr 1fr;
      gap: var(--space-12);
    }
  }
</style>
