import { render, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { listSources, setSourceEnabled, type Source } from '../../lib/services/sources';
import { sourceErrorText } from '../../lib/sources/sourceErrors';
import { sources } from '../../lib/stores/sources';
import { user } from '../../lib/testing/dom';
import Sources from './Sources.svelte';
import { MAIN, menuButton, mountPoint, OLD, resetScreen, rowOf, toastsSaid } from './sourcesTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// Go's sources.Service: SetSourceEnabled answers as each test says, the list read back is Go's.
vi.mock('../../lib/services/sources', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/sources')>()),
  listSources: vi.fn(),
  setSourceEnabled: vi.fn(),
}));

let app: HTMLDivElement;
let goHolds: Source[];

beforeEach(() => {
  app = mountPoint();
  goHolds = [MAIN, OLD];
  sources.set(goHolds);
  vi.mocked(listSources).mockImplementation(async () => goHolds);
});

afterEach(() => {
  resetScreen();
  vi.mocked(listSources).mockReset();
  vi.mocked(setSourceEnabled).mockReset();
});

const show = () => render(Sources, { target: app, props: {} });
// Named by the source's name its row shows.
const switchOf = (name: string) => within(rowOf(name)).getByRole('switch', { name });
const goSets = (id: string, enabled: boolean) => {
  goHolds = goHolds.map((item) => (item.id === id ? ({ ...item, enabled, status: enabled ? 'active' : 'disabled' } as Source) : item));
};

// R6: the row turns its source on and off itself, as on the prototype.
describe('Sources: the switch on a row', () => {
  it('R6: shows whether the source is on, named after it', () => {
    show();

    expect(switchOf('Main feed').getAttribute('aria-checked')).toBe('true');
    expect(switchOf('Old feed').getAttribute('aria-checked')).toBe('false');
  });

  it('R6: asks Go to turn the source off, and shows what Go then holds', async () => {
    vi.mocked(setSourceEnabled).mockImplementation(async (id, enabled) => goSets(id, enabled));
    show();

    await user().click(switchOf('Main feed'));

    expect(setSourceEnabled).toHaveBeenCalledWith('s1', false);
    await waitFor(() => expect(get(sources).find((item) => item.id === 's1')?.enabled).toBe(false));
    expect(switchOf('Main feed').getAttribute('aria-checked')).toBe('false');
  });

  it('R6: turns a disabled source on', async () => {
    vi.mocked(setSourceEnabled).mockImplementation(async (id, enabled) => goSets(id, enabled));
    show();

    await user().click(switchOf('Old feed'));

    expect(setSourceEnabled).toHaveBeenCalledWith('s3', true);
    await waitFor(() => expect(switchOf('Old feed').getAttribute('aria-checked')).toBe('true'));
  });

  it('R6: moves at once while Go answers, and a second press then asks nothing more', async () => {
    let answer: () => void = () => {};
    vi.mocked(setSourceEnabled).mockImplementation(() => new Promise<void>((resolve) => (answer = resolve)));
    show();
    const keys = user();

    await keys.click(switchOf('Main feed'));
    expect(switchOf('Main feed').getAttribute('aria-checked')).toBe('false');
    await keys.click(switchOf('Main feed'));

    expect(setSourceEnabled).toHaveBeenCalledTimes(1);
    answer();
  });

  it('R6: a refusal rolls the switch back and is said once, focus staying on the switch', async () => {
    vi.mocked(setSourceEnabled).mockRejectedValue(new Error('typhon:sources.source_busy: refresh running'));
    show();
    const keys = user();
    switchOf('Main feed').focus();

    await keys.keyboard(' ');

    await waitFor(() => expect(toastsSaid()).toEqual([['danger', sourceErrorText(new Error('typhon:sources.source_busy'))]]));
    expect(switchOf('Main feed').getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(switchOf('Main feed'));
  });

  it('R6: the row menu no longer carries the toggle', async () => {
    show();

    await user().click(menuButton('Main feed'));

    expect(screen.getAllByRole('menuitem').map((item) => item.textContent?.trim())).toEqual([
      msg('transfers.sourcesRefreshNow'),
      msg('transfers.sourcesDetailsAction'),
      msg('transfers.sourcesRemoveAction'),
    ]);
  });
});
