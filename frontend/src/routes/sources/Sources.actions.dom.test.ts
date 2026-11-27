import { render, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import {
  getRelease,
  getSourceDetails,
  listSources,
  queryReleases,
  refreshAllSources,
  refreshSource,
  removeSource,
  setSourceEnabled,
  type Source,
} from '../../lib/services/sources';
import { sourceErrorText } from '../../lib/sources/sourceErrors';
import { navigate } from '../../lib/stores/router';
import { refreshingAll, sources } from '../../lib/stores/sources';
import { acceptSourcesNotice } from '../../lib/stores/sourcesNotice';
import { user } from '../../lib/testing/dom';
import Sources from './Sources.svelte';
import { acceptNotice, MAIN, MIRROR, menuButton, mountPoint, pick, resetScreen, rowNames, toastsSaid } from './sourcesTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// Go's sources.Service, answering as each test says: no feed is fetched.
vi.mock('../../lib/services/sources', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/sources')>()),
  listSources: vi.fn(),
  refreshSource: vi.fn(),
  refreshAllSources: vi.fn(),
  setSourceEnabled: vi.fn(),
  removeSource: vi.fn(),
  getSourceDetails: vi.fn(),
  queryReleases: vi.fn(),
  getRelease: vi.fn(),
}));
// The notice's acceptance is saved through the settings; here it is taken as saved.
vi.mock('../../lib/stores/sourcesNotice', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/sourcesNotice')>()),
  acceptSourcesNotice: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = mountPoint();
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listSources).mockImplementation(async () => get(sources));
  vi.mocked(refreshSource).mockResolvedValue({ entries: 10, added: 1, review: 0 } as Awaited<ReturnType<typeof refreshSource>>);
  vi.mocked(refreshAllSources).mockResolvedValue([]);
  vi.mocked(setSourceEnabled).mockResolvedValue(undefined);
  vi.mocked(removeSource).mockResolvedValue(undefined);
  vi.mocked(getSourceDetails).mockImplementation(() => new Promise(() => {}));
  vi.mocked(queryReleases).mockImplementation(() => new Promise(() => {}));
  vi.mocked(getRelease).mockImplementation(() => new Promise(() => {}));
  vi.mocked(acceptSourcesNotice).mockImplementation(async () => {
    acceptNotice(true);
    return true;
  });
});

afterEach(() => {
  resetScreen();
  vi.restoreAllMocks();
  for (const fn of [listSources, refreshSource, refreshAllSources, setSourceEnabled, removeSource, getSourceDetails, queryReleases, getRelease, acceptSourcesNotice]) {
    vi.mocked(fn).mockReset();
  }
});

const show = () => render(Sources, { target: app, props: {} });
const removeDialog = () => screen.getByRole('dialog', { name: msg('transfers.sourcesRemoveAction') });

describe('Sources: the row menu', () => {
  // R6: the toggle moved to a switch on the row (Sources.toggle.dom.test.ts).
  it('R6: offers refresh, details and remove, in that order', async () => {
    sources.set([MAIN]);
    show();

    await user().click(menuButton('Main feed'));

    expect(screen.getAllByRole('menuitem').map((item) => item.textContent?.trim())).toEqual([
      msg('transfers.sourcesRefreshNow'),
      msg('transfers.sourcesDetailsAction'),
      msg('transfers.sourcesRemoveAction'),
    ]);
  });

  it('Refresh now asks Go to refresh that source, reads the list again and says what it found', async () => {
    sources.set([MAIN, MIRROR]);
    show();

    await pick(user(), 'Mirror', msg('transfers.sourcesRefreshNow'));

    await waitFor(() => expect(toastsSaid()).toEqual([['success', msg('state.sourcesRefreshResult', { entries: 10, added: 1, review: 0 })]]));
    expect(refreshSource).toHaveBeenCalledWith('s2');
    expect(refreshSource).toHaveBeenCalledTimes(1);
    expect(listSources).toHaveBeenCalled();
  });

  it('Refresh now is not offered as a press on a source that is updating', async () => {
    sources.set([{ ...MAIN, status: 'updating' } as Source]);
    show();

    await user().click(menuButton('Main feed'));
    await user().click(screen.getByRole('menuitem', { name: msg('transfers.sourcesUpdatingEllipsis') }));

    expect(refreshSource).not.toHaveBeenCalled();
  });

  it('Details opens the dialog of that source', async () => {
    sources.set([MAIN, MIRROR]);
    show();

    await pick(user(), 'Mirror', msg('transfers.sourcesDetailsAction'));

    expect(await screen.findByRole('dialog', { name: 'Mirror' })).toBeTruthy();
    expect(getSourceDetails).toHaveBeenCalledWith('s2');
  });
});

describe('Sources: removing a source', () => {
  it('asks first, naming the source, and removes nothing on Cancel', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    const keys = user();

    await pick(keys, 'Main feed', msg('transfers.sourcesRemoveAction'));

    expect(within(removeDialog()).getByText(msg('modals.sourceDetailsRemoveText', { name: 'Main feed' }))).toBeTruthy();
    await keys.click(within(removeDialog()).getByRole('button', { name: msg('common.cancel') }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(removeSource).not.toHaveBeenCalled();
    expect(rowNames()).toEqual(['Main feed', 'Mirror']);
  });

  it('on the confirmation asks Go to remove that source, and its row leaves', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    const keys = user();

    await pick(keys, 'Main feed', msg('transfers.sourcesRemoveAction'));
    await keys.click(within(removeDialog()).getByRole('button', { name: msg('transfers.sourcesRemoveAction') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(removeSource).toHaveBeenCalledWith('s1');
    expect(rowNames()).toEqual(['Mirror']);
  });

  it('keeps the confirmation open with Go’s reason when Go refused, and the row stays', async () => {
    vi.mocked(removeSource).mockRejectedValue(new Error('typhon:sources.source_busy: refresh running'));
    sources.set([MAIN, MIRROR]);
    show();
    const keys = user();

    await pick(keys, 'Main feed', msg('transfers.sourcesRemoveAction'));
    await keys.click(within(removeDialog()).getByRole('button', { name: msg('transfers.sourcesRemoveAction') }));

    await waitFor(() => expect(within(removeDialog()).getByRole('alert').textContent).toContain(sourceErrorText(new Error('typhon:sources.source_busy'))));
    expect(within(removeDialog()).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(rowNames()).toEqual(['Main feed', 'Mirror']);
    expect(toastsSaid()).toEqual([]);
  });

  // R2: the row and its menu are gone; focus goes to the row now in its place (Sources.focus.dom.test.ts).
  it('R2: hands focus to the row now in its place once the row is gone', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    const keys = user();

    menuButton('Main feed').focus();
    await keys.keyboard('{Enter}');
    screen.getByRole('menuitem', { name: msg('transfers.sourcesRemoveAction') }).focus();
    await keys.keyboard('{Enter}');
    within(removeDialog()).getByRole('button', { name: msg('transfers.sourcesRemoveAction') }).focus();
    await keys.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(rowNames()).toEqual(['Mirror']);
    await waitFor(() => expect(document.activeElement).toBe(menuButton('Mirror')));
  });
});

describe('Sources: the header actions', () => {
  it('Refresh all asks Go to refresh every source and reads the list again', async () => {
    let finish: (value: never[]) => void = () => {};
    vi.mocked(refreshAllSources).mockImplementation(() => new Promise((resolve) => (finish = resolve)));
    sources.set([MAIN]);
    show();

    await user().click(screen.getByRole('button', { name: msg('transfers.sourcesRefreshAll') }));

    expect(refreshAllSources).toHaveBeenCalledTimes(1);
    expect(get(refreshingAll)).toBe(true);
    expect(screen.getByRole('button', { name: msg('transfers.sourcesUpdatingEllipsis') }).getAttribute('aria-busy')).toBe('true');
    finish([]);
    await waitFor(() => expect(get(refreshingAll)).toBe(false));
    expect(listSources).toHaveBeenCalled();
  });

  it('Add shows the notice about what leaves the device first, then the add dialog once it is accepted', async () => {
    acceptNotice(false);
    sources.set([MAIN]);
    show();
    const keys = user();

    await keys.click(screen.getByRole('button', { name: msg('transfers.sourcesAddAction') }));

    expect(screen.getByRole('dialog', { name: msg('modals.sourcesNoticeGateTitle') })).toBeTruthy();
    expect(screen.queryByRole('dialog', { name: msg('modals.addSourceTitle') })).toBeNull();
    await keys.click(screen.getByRole('button', { name: msg('modals.sourcesNoticeContinue') }));

    expect(acceptSourcesNotice).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole('dialog', { name: msg('modals.addSourceTitle') })).toBeTruthy();
    expect(screen.queryByRole('dialog', { name: msg('modals.sourcesNoticeGateTitle') })).toBeNull();
  });

  it('Add goes straight to the add dialog once the notice was accepted', async () => {
    acceptNotice(true);
    sources.set([MAIN]);
    show();

    await user().click(screen.getByRole('button', { name: msg('transfers.sourcesAddAction') }));

    expect(screen.getByRole('dialog', { name: msg('modals.addSourceTitle') })).toBeTruthy();
    expect(screen.queryByRole('dialog', { name: msg('modals.sourcesNoticeGateTitle') })).toBeNull();
  });

  it('the empty list’s own Add goes through the same notice', async () => {
    acceptNotice(false);
    show();

    const adds = screen.getAllByRole('button', { name: msg('transfers.sourcesAddAction') });
    expect(adds).toHaveLength(2);
    await user().click(adds[1]);

    expect(screen.getByRole('dialog', { name: msg('modals.sourcesNoticeGateTitle') })).toBeTruthy();
  });
});

describe('Sources: a link into a source', () => {
  it('opens the details of the source the route names, at the release it names', async () => {
    sources.set([MAIN, MIRROR]);
    navigate('sources', { sourceId: 's2', releaseId: 'r9' });
    show();

    expect(await screen.findByRole('dialog', { name: 'Mirror' })).toBeTruthy();
    expect(getSourceDetails).toHaveBeenCalledWith('s2');
    expect(getRelease).toHaveBeenCalledWith('r9');
  });

  it('opens nothing for a route that names no source', () => {
    sources.set([MAIN]);
    navigate('sources');
    show();

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(getSourceDetails).not.toHaveBeenCalled();
  });
});
