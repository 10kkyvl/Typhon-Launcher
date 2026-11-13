import { render, screen, within } from '@testing-library/svelte';
import { Events } from '@wailsio/runtime';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { resumeDownload, type Download } from '../services/downloads';
import { downloads, initDownloads } from '../stores/downloads';
import { liveIsland, liveIslandCurrent } from '../stores/liveIsland';
import { resetHistory, route } from '../stores/router';
import { holdToasts, toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import IslandShellHarness from '../testing/IslandShellHarness.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
// The shell renders as in the browser (no backend); the stores under test start as in Wails.
const backend = vi.hoisted(() => ({ wails: false }));
vi.mock('../services/backend', () => ({
  get inWails() {
    return backend.wails;
  },
}));
vi.mock('../services/downloads', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/downloads')>()),
  listDownloads: vi.fn(async () => []),
  resumeDownload: vi.fn(async () => {}),
}));

let app: HTMLDivElement;

function download(patch: Partial<Download> = {}): Download {
  return {
    id: 'dl1',
    name: 'Elden Ring',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:abc',
    infoHash: 'abc',
    destination: '/games',
    status: 'downloading',
    progress: 0.4,
    downloaded: 1,
    total: 2,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-11-13T10:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...patch,
  } as Download;
}

/** Starts the downloads store as App.svelte does in Wails and answers with its event handlers. */
async function startDownloads() {
  vi.mocked(Events.On).mockClear();
  backend.wails = true;
  await initDownloads();
  backend.wails = false;
  const handlers = new Map(vi.mocked(Events.On).mock.calls.map(([name, cb]) => [name as string, cb]));
  return (name: string, data: unknown) => handlers.get(name)?.({ data } as never);
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  toasts.set([]);
  holdToasts(false);
  liveIsland.clear();
  downloads.set([]);
  resetHistory();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  document.documentElement.classList.remove('no-anim');
  toasts.set([]);
  liveIsland.clear();
  downloads.set([]);
  resetHistory();
  vi.unstubAllGlobals();
});

function setup() {
  render(IslandShellHarness, { target: app });
  const island = () => app.querySelector<HTMLElement>('[data-part="live-island"]');
  const pill = () => island()?.querySelector<HTMLButtonElement>('.head') as HTMLButtonElement;
  return { keys: user(), island, pill };
}

describe('a failed download in the island', () => {
  it('is one message, an error, with no toast beside it', async () => {
    const emit = await startDownloads();
    const { island } = setup();
    emit('download:added', download());
    toasts.set([]);
    emit('download:failed', download({ status: 'failed', error: 'download.peers_lost' }));
    await tick();

    expect(get(toasts)).toEqual([]);
    expect(island()?.dataset.kind).toBe('error');
    expect(get(liveIslandCurrent)).toMatchObject({ text: msg('ui.liveIslandDownloadFailed'), title: 'Elden Ring' });
    expect(within(screen.getByTestId('live-island-alert')).getAllByText(/Elden Ring/)).toHaveLength(1);
  });

  it('Retry retries the download and the message goes', async () => {
    const emit = await startDownloads();
    const { keys, island, pill } = setup();
    emit('download:added', download());
    emit('download:failed', download({ status: 'failed' }));
    await tick();
    await keys.click(pill());
    await tick();
    await keys.click(within(island() as HTMLElement).getByRole('button', { name: msg('common.retry') }));

    expect(vi.mocked(resumeDownload)).toHaveBeenCalledWith('dl1');
    expect(liveIsland.has('download-failed:dl1')).toBe(false);
  });

  it('"Open downloads" opens the downloads page', async () => {
    const emit = await startDownloads();
    const { keys, island, pill } = setup();
    emit('download:added', download());
    emit('download:failed', download({ status: 'failed' }));
    await tick();
    await keys.click(pill());
    await tick();
    await keys.click(within(island() as HTMLElement).getByRole('button', { name: msg('ui.liveIslandOpenDownloads') }));

    expect(get(route).name).toBe('downloads');
  });

  it('a download that had failed before the island came says nothing', async () => {
    downloads.set([download({ status: 'failed' })]);
    setup();
    await tick();
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('a failed download in the list that loads after the island came says nothing', async () => {
    setup();
    await tick();
    downloads.set([download({ status: 'failed' }), download({ id: 'dl2', status: 'downloading' })]);
    await tick();
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('the message goes when the download is no longer failed (retried from the Downloads page)', async () => {
    const emit = await startDownloads();
    setup();
    emit('download:added', download());
    emit('download:failed', download({ status: 'failed' }));
    await tick();
    expect(liveIsland.has('download-failed:dl1')).toBe(true);
    emit('download:updated', download({ status: 'queued' }));
    await tick();
    expect(liveIsland.has('download-failed:dl1')).toBe(false);
  });

  it('with motion off it is the toast it always was, and there is no island', async () => {
    document.documentElement.classList.add('no-anim');
    const emit = await startDownloads();
    const { island } = setup();
    emit('download:added', download());
    toasts.set([]);
    emit('download:failed', download({ status: 'failed', error: 'download.peers_lost' }));
    await tick();

    expect(island()).toBeNull();
    expect(get(toasts).map((t) => t.kind)).toEqual(['danger']);
    expect(get(toasts)[0].message).toContain('Elden Ring');
  });
});
