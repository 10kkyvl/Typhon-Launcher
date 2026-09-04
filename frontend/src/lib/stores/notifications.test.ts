import { describe, it, expect, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => vi.fn()) },
  Call: { ByID: vi.fn() },
  CancellablePromise: class {},
}));
vi.mock('../services/backend', () => ({ inWails: false }));

vi.mock('../services/downloads', () => ({
  cancelDownload: vi.fn(),
  forceStartDownload: vi.fn(),
  listDownloads: vi.fn(),
  moveDownloadDown: vi.fn(),
  moveDownloadUp: vi.fn(),
  pauseDownload: vi.fn(),
  removeDownload: vi.fn(),
  resumeDownload: vi.fn(),
}));

vi.mock('../services/install', () => ({
  listInstallations: vi.fn(),
}));

vi.mock('../services/sources', () => ({
  addSource: vi.fn(),
  addSourceFile: vi.fn(),
  listSources: vi.fn(),
  refreshAllSources: vi.fn(),
  refreshSource: vi.fn(),
  removeSource: vi.fn(),
  setSourceEnabled: vi.fn(),
}));

vi.mock('../services/updates', () => ({
  buildManifest: vi.fn(),
  cancelUpdate: vi.fn(),
  listUpdates: vi.fn(),
  prepareUpdatePlan: vi.fn(),
  repairGame: vi.fn(),
  rollbackUpdate: vi.fn(),
  startUpdate: vi.fn(),
  verifyGame: vi.fn(),
}));

vi.mock('../services/selfupdate', () => ({
  applyUpdate: vi.fn(),
  checkForUpdate: vi.fn(),
  dismissUpdate: vi.fn(),
  downloadUpdate: vi.fn(),
  getOutcome: vi.fn(),
  getStatus: vi.fn(),
  getReleaseNotes: vi.fn(),
  acknowledgeReleaseNotes: vi.fn(),
  emptyReleaseNotes: () => ({ currentVersion: '', unseen: [], history: [] }),
}));

const READ_KEY = 'typhon.notifications.read';

function makeStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial));
  return {
    getItem: (key: string) => (data.has(key) ? (data.get(key) as string) : null),
    setItem: (key: string, value: string) => {
      data.set(key, value);
    },
    removeItem: (key: string) => {
      data.delete(key);
    },
  };
}

type Storage = ReturnType<typeof makeStorage>;

function status(overrides: Partial<Record<string, unknown>> = {}) {
  return { state: 'idle', currentVersion: '1.0.0', ...overrides };
}

async function load(storage: Storage | undefined) {
  vi.resetModules();
  if (storage) {
    (globalThis as Record<string, unknown>).localStorage = storage;
  } else {
    delete (globalThis as Record<string, unknown>).localStorage;
  }
  const { sources } = await import('./sources');
  const { selfUpdateStatus } = await import('./selfupdate');
  const notifications = await import('./notifications');
  return { sources, selfUpdateStatus, notifications };
}

describe('обновление лаунчера в списке уведомлений', () => {
  it('доступная версия — уведомление первое в списке, id содержит версию', async () => {
    const { sources, selfUpdateStatus, notifications } = await load(makeStorage());
    sources.set([{ id: 's1', name: 'Источник', lastError: 'упал' } as never]);
    selfUpdateStatus.set(status({ state: 'available', availableVersion: '1.2.3' }) as never);

    const items = get(notifications.notifications);
    expect(items[0].id).toBe('launcher-update:1.2.3');
    expect(items[0].text).toContain('1.2.3');
    expect(items[0].route).toBe('settings');
  });

  it('готово к установке — своя запись, не совпадает с записью о доступности', async () => {
    const { selfUpdateStatus, notifications } = await load(makeStorage());
    selfUpdateStatus.set(status({ state: 'ready', availableVersion: '1.3.0' }) as never);

    const items = get(notifications.notifications).filter((n) => n.id.startsWith('launcher-update'));
    expect(items).toHaveLength(1);
    expect(items[0].id).toBe('launcher-update-ready:1.3.0');
    expect(items[0].text).toContain('готова к установке');
  });

  it('ошибка обновления — своя запись, id отличается от записи о доступности', async () => {
    const { selfUpdateStatus, notifications } = await load(makeStorage());
    selfUpdateStatus.set(status({ state: 'available', availableVersion: '1.3.0', error: 'сеть недоступна' }) as never);

    const items = get(notifications.notifications).filter((n) => n.id.startsWith('launcher-update'));
    expect(items).toHaveLength(1);
    expect(items[0].id).toMatch(/^launcher-update-error:1\.3\.0:/);
    expect(items[0].id).not.toBe('launcher-update:1.3.0');
    expect(items[0].text).toBe('сеть недоступна');
  });

  it('состояние failed без текста ошибки даёт запись с запасным текстом', async () => {
    const { selfUpdateStatus, notifications } = await load(makeStorage());
    selfUpdateStatus.set(status({ state: 'failed' }) as never);

    const items = get(notifications.notifications).filter((n) => n.id.startsWith('launcher-update'));
    expect(items).toHaveLength(1);
    expect(items[0].text).toBe('Не удалось проверить обновления');
  });

  it('без ошибки, доступности и готовности — записи нет', async () => {
    const { selfUpdateStatus, notifications } = await load(makeStorage());
    selfUpdateStatus.set(status({ state: 'checking' }) as never);

    expect(get(notifications.notifications).some((n) => n.id.startsWith('launcher-update'))).toBe(false);
  });
});

describe('заявки в друзья', () => {
  function request(id: string) {
    return { id, username: id, displayName: id, avatarUrl: '', createdAt: '', mutualCount: 0, commonCount: 0 };
  }

  it('входящие заявки дают уведомление с маршрутом friends', async () => {
    const { notifications } = await load(makeStorage());
    const { friendsPage } = await import('./social');
    friendsPage.set({ friends: [], incoming: [request('a'), request('b')], outgoing: [] });

    const items = get(notifications.notifications).filter((n) => n.id.startsWith('friends:'));
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({ id: 'friends:incoming:2', route: 'friends', terminal: false });
  });

  it('прочитанное уведомление возвращается, когда заявок стало больше', async () => {
    const { notifications } = await load(makeStorage());
    const { friendsPage } = await import('./social');
    friendsPage.set({ friends: [], incoming: [request('a')], outgoing: [] });
    notifications.markAllRead();
    expect(get(notifications.notifications).some((n) => n.id.startsWith('friends:'))).toBe(false);

    friendsPage.set({ friends: [], incoming: [request('a'), request('b')], outgoing: [] });

    const items = get(notifications.notifications).filter((n) => n.id.startsWith('friends:'));
    expect(items).toHaveLength(1);
    expect(items[0].id).toBe('friends:incoming:2');
  });

  it('прочитанное уведомление не возвращается, когда заявок стало меньше', async () => {
    const { notifications } = await load(makeStorage());
    const { friendsPage } = await import('./social');
    friendsPage.set({ friends: [], incoming: [request('a'), request('b'), request('c')], outgoing: [] });
    notifications.markAllRead();

    friendsPage.set({ friends: [], incoming: [request('a'), request('b')], outgoing: [] });

    expect(get(notifications.notifications).some((n) => n.id.startsWith('friends:'))).toBe(false);
  });

  it('без входящих заявок уведомления нет', async () => {
    const { notifications } = await load(makeStorage());
    expect(get(notifications.notifications).some((n) => n.id.startsWith('friends:'))).toBe(false);
  });
});

describe('markAllRead', () => {
  it('убирает все текущие уведомления из списка', async () => {
    const { sources, notifications } = await load(makeStorage());
    sources.set([
      { id: 's1', name: 'A', lastError: 'x' } as never,
      { id: 's2', name: 'B', lastError: 'y' } as never,
    ]);
    expect(get(notifications.notifications)).toHaveLength(2);

    notifications.markAllRead();

    expect(get(notifications.notifications)).toHaveLength(0);
  });

  it('после markAllRead новое уведомление с другим id появляется', async () => {
    const { sources, notifications } = await load(makeStorage());
    sources.set([{ id: 's1', name: 'A', lastError: 'x' } as never]);
    notifications.markAllRead();
    expect(get(notifications.notifications)).toHaveLength(0);

    sources.set([
      { id: 's1', name: 'A', lastError: 'x' } as never,
      { id: 's2', name: 'B', lastError: 'y' } as never,
    ]);

    const items = get(notifications.notifications);
    expect(items).toHaveLength(1);
    expect(items[0].id).toMatch(/^source:s2:/);
  });

  it('прочитанная ошибка источника снова показывается, когда текст ошибки изменился', async () => {
    const { sources, notifications } = await load(makeStorage());
    sources.set([{ id: 's1', name: 'A', lastError: 'таймаут' } as never]);
    notifications.markAllRead();
    expect(get(notifications.notifications)).toHaveLength(0);

    sources.set([{ id: 's1', name: 'A', lastError: 'истёк токен' } as never]);

    const items = get(notifications.notifications);
    expect(items).toHaveLength(1);
    expect(items[0].text).toBe('истёк токен');
  });

  it('прочитанная ошибка обновления игры снова показывается, когда причина сбоя изменилась', async () => {
    const { notifications } = await load(makeStorage());
    const { updates } = await import('./updates');
    updates.set([{ gameId: 'g1', title: 'Игра', error: 'нет места' } as never]);
    notifications.markAllRead();
    expect(get(notifications.notifications)).toHaveLength(0);

    updates.set([{ gameId: 'g1', title: 'Игра', error: 'битый архив' } as never]);

    const items = get(notifications.notifications);
    expect(items).toHaveLength(1);
    expect(items[0].text).toContain('битый архив');
  });

  it('прочитанное для версии X уведомление о лаунчере снова показывается для версии Y', async () => {
    const { selfUpdateStatus, notifications } = await load(makeStorage());
    selfUpdateStatus.set(status({ state: 'available', availableVersion: '1.0.1' }) as never);
    notifications.markAllRead();
    expect(get(notifications.notifications)).toHaveLength(0);

    selfUpdateStatus.set(status({ state: 'available', availableVersion: '1.0.2' }) as never);

    const items = get(notifications.notifications);
    expect(items).toHaveLength(1);
    expect(items[0].id).toBe('launcher-update:1.0.2');
  });
});

describe('прочитанные id и localStorage', () => {
  it('вычищаются, когда уведомление исчезает из живого списка', async () => {
    const storage = makeStorage();
    const { sources, notifications } = await load(storage);
    sources.set([{ id: 's1', name: 'A', lastError: 'x' } as never]);
    notifications.markAllRead();
    expect(JSON.parse(storage.getItem(READ_KEY) as string)).toHaveLength(1);

    sources.set([]);

    expect(JSON.parse(storage.getItem(READ_KEY) as string)).toEqual([]);
  });

  it('битый JSON в localStorage не роняет модуль и считается «ничего не прочитано»', async () => {
    const storage = makeStorage({ [READ_KEY]: '{ не json' });
    const { sources, notifications } = await load(storage);
    sources.set([{ id: 's1', name: 'A', lastError: 'x' } as never]);

    expect(get(notifications.notifications)).toHaveLength(1);
  });

  it('отсутствие localStorage не роняет модуль', async () => {
    const { sources, notifications } = await load(undefined);
    sources.set([{ id: 's1', name: 'A', lastError: 'x' } as never]);

    expect(() => notifications.markAllRead()).not.toThrow();
    expect(get(notifications.notifications)).toHaveLength(0);
  });
});
