import { render, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { listSources, removeSource, type Source } from '../../lib/services/sources';
import { sources } from '../../lib/stores/sources';
import { user } from '../../lib/testing/dom';
import Sources from './Sources.svelte';
import { MAIN, MIRROR, menuButton, mountPoint, OLD, resetScreen, rowNames, rowOf } from './sourcesTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/sources', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/sources')>()),
  listSources: vi.fn(),
  removeSource: vi.fn(),
}));

let app: HTMLDivElement;
let scrolled: Element[];

beforeEach(() => {
  app = mountPoint();
  scrolled = [];
  Element.prototype.scrollIntoView = vi.fn(function (this: Element) {
    scrolled.push(this);
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listSources).mockImplementation(async () => get(sources));
  vi.mocked(removeSource).mockResolvedValue(undefined);
});

afterEach(() => {
  resetScreen();
  vi.restoreAllMocks();
  vi.mocked(listSources).mockReset();
  vi.mocked(removeSource).mockReset();
});

const show = () => render(Sources, { target: app, props: {} });
const addButton = () => screen.getAllByRole('button', { name: msg('transfers.sourcesAddAction') })[0];
const removeDialog = () => screen.queryByRole('dialog', { name: msg('transfers.sourcesRemoveAction') });
/** The row's own control that opens the source: the first of the row's controls. */
const leadOf = (name: string) => rowOf(name).querySelector<HTMLElement>('[data-row-control="open"]') as HTMLElement;

/** Removes `name` by the keyboard alone: its menu, Remove, the confirmation's Remove. */
async function removeByKeys(name: string) {
  const keys = user();
  menuButton(name).focus();
  await keys.keyboard('{Enter}');
  screen.getByRole('menuitem', { name: msg('transfers.sourcesRemoveAction') }).focus();
  await keys.keyboard('{Enter}');
  within(removeDialog() as HTMLElement).getByRole('button', { name: msg('transfers.sourcesRemoveAction') }).focus();
  await keys.keyboard('{Enter}');
  await waitFor(() => expect(removeDialog()).toBeNull());
  return keys;
}

// R2: a removed row takes its menu and the focus with it; focus goes on by the Downloads screen's rule.
describe('Sources: focus after a source is removed', () => {
  it('R2: goes to the same control of the row now in its place', async () => {
    sources.set([MAIN, MIRROR, OLD]);
    show();

    await removeByKeys('Main feed');

    expect(rowNames()).toEqual(['Mirror', 'Old feed']);
    await waitFor(() => expect(document.activeElement).toBe(menuButton('Mirror')));
    expect(scrolled).toContain(rowOf('Mirror'));
  });

  it('R2: goes to the row above when the last row went', async () => {
    sources.set([MAIN, MIRROR, OLD]);
    show();

    await removeByKeys('Old feed');

    await waitFor(() => expect(document.activeElement).toBe(menuButton('Mirror')));
  });

  it('R2: goes to Add when no row is left', async () => {
    sources.set([MAIN]);
    show();

    await removeByKeys('Main feed');

    expect(screen.getByRole('heading', { level: 2, name: msg('transfers.sourcesEmptyTitle') })).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(addButton()));
  });

  it('goes back to the row’s menu button when the confirmation is cancelled', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    const keys = user();
    menuButton('Main feed').focus();
    await keys.keyboard('{Enter}');
    screen.getByRole('menuitem', { name: msg('transfers.sourcesRemoveAction') }).focus();
    await keys.keyboard('{Enter}');

    await keys.keyboard('{Escape}');

    expect(removeDialog()).toBeNull();
    expect(document.activeElement).toBe(menuButton('Main feed'));
  });

  it('stays in the confirmation when Go refused, on its Retry', async () => {
    vi.mocked(removeSource).mockRejectedValue(new Error('typhon:sources.source_busy: busy'));
    sources.set([MAIN, MIRROR]);
    show();
    const keys = user();
    menuButton('Main feed').focus();
    await keys.keyboard('{Enter}');
    screen.getByRole('menuitem', { name: msg('transfers.sourcesRemoveAction') }).focus();
    await keys.keyboard('{Enter}');
    within(removeDialog() as HTMLElement).getByRole('button', { name: msg('transfers.sourcesRemoveAction') }).focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(document.activeElement).toBe(within(removeDialog() as HTMLElement).getByRole('button', { name: msg('common.retry') })));
  });
});

describe('Sources: focus on a row Go takes away', () => {
  it('R2: goes to the same control of the row now in its place', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    leadOf('Main feed').focus();

    sources.update((list) => list.filter((item) => item.id !== 's1'));

    await waitFor(() => expect(document.activeElement).toBe(leadOf('Mirror')));
  });

  it('R2: from its open menu goes to the menu button of the row now in its place', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    const keys = user();
    menuButton('Main feed').focus();
    await keys.keyboard('{Enter}');
    expect(screen.getByRole('menu')).toBeTruthy();

    sources.update((list) => list.filter((item) => item.id !== 's1'));

    await waitFor(() => expect(document.activeElement).toBe(menuButton('Mirror')));
  });

  it('R2: a row that leaves the status filter shown hands focus on the same way', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    await user().click(screen.getByRole('radio', { name: (name) => name.startsWith(msg('transfers.sourcesTabActive')) }));
    menuButton('Main feed').focus();

    sources.update((list) => list.map((item) => (item.id === 's1' ? ({ ...item, status: 'disabled', enabled: false } as Source) : item)));

    await waitFor(() => expect(document.activeElement).toBe(menuButton('Mirror')));
  });

  it('leaves alone focus that is not in a row', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    const refreshAll = screen.getByRole('button', { name: msg('transfers.sourcesRefreshAll') });
    refreshAll.focus();

    sources.update((list) => list.filter((item) => item.id !== 's1'));

    await waitFor(() => expect(rowNames()).toEqual(['Mirror']));
    expect(document.activeElement).toBe(refreshAll);
  });

  it('keeps focus on a control of a row that stays', async () => {
    sources.set([MAIN, MIRROR]);
    show();
    menuButton('Mirror').focus();

    sources.update((list) => list.filter((item) => item.id !== 's1'));

    await waitFor(() => expect(rowNames()).toEqual(['Mirror']));
    expect(document.activeElement).toBe(menuButton('Mirror'));
  });
});
