import { render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { sourceErrorText } from '../../lib/sources/sourceErrors';
import { sourceProgress } from '../../lib/stores/sourceProgress';
import { sources } from '../../lib/stores/sources';
import { user } from '../../lib/testing/dom';
import { describedControls } from '../../lib/testing/hints';
import { ruleBody, styleOf } from '../../lib/testing/css';
import { clockTime, longDate, relativeDate } from '../../lib/utils/format';
import Sources from './Sources.svelte';
import row from './SourceRow.svelte?raw';
import { MAIN, MIRROR, menuButton, mountPoint, resetScreen, rowNames, rowOf, source, words } from './sourcesTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

let app: HTMLDivElement;

beforeEach(() => {
  app = mountPoint();
});

afterEach(() => {
  resetScreen();
});

const show = () => render(Sources, { target: app, props: {} });

async function menuItems(name: string) {
  await user().click(menuButton(name));
  return screen.getAllByRole('menuitem').map((item) => [words(item), item.getAttribute('aria-disabled') === 'true']);
}

describe('Sources: a row shows its source', () => {
  it('lists every source by name, in the order Go gave them', () => {
    sources.set([MAIN, MIRROR]);
    show();

    expect(rowNames()).toEqual(['Main feed', 'Mirror']);
  });

  it('says where the source is read from: its address, or the path of a file feed', () => {
    sources.set([MAIN, source({ id: 's2', name: 'Local', type: 'file', url: '', path: 'C:\\feeds\\mine.json' })]);
    show();

    expect(words(rowOf('Main feed'))).toContain('https://example.invalid/feed.json');
    expect(words(rowOf('Local'))).toContain('C:\\feeds\\mine.json');
  });

  it('an active source says so', () => {
    sources.set([MAIN]);
    show();

    expect(within(rowOf('Main feed')).getByText(msg('transfers.sourcesStatusActive'))).toBeTruthy();
    expect(within(rowOf('Main feed')).queryByRole('progressbar')).toBeNull();
  });

  it('a disabled source says so, and its menu offers to enable it', async () => {
    sources.set([source({ enabled: false, status: 'disabled' })]);
    show();

    expect(within(rowOf('Main feed')).getByText(msg('transfers.sourcesStatusDisabled'))).toBeTruthy();
    expect(await menuItems('Main feed')).toContainEqual([msg('transfers.sourcesEnableAction'), false]);
  });

  it('an updating source with no stage heard yet says it is updating, and its refresh item is off', async () => {
    sources.set([source({ status: 'updating' })]);
    show();

    expect(within(rowOf('Main feed')).getByText(msg('transfers.sourcesStatusUpdating'))).toBeTruthy();
    expect(within(rowOf('Main feed')).queryByRole('progressbar')).toBeNull();
    expect(await menuItems('Main feed')).toContainEqual([msg('transfers.sourcesUpdatingEllipsis'), true]);
  });

  it('an updating source shows the live stage and how far it is, as a bar named after the source', () => {
    sources.set([source({ status: 'updating' })]);
    sourceProgress.set({ s1: { stage: 'match', done: 5, total: 10, startedAt: 0, sawMatch: true } });
    show();

    const stage = msg('transfers.sourcesStageMatch', { done: '5', total: '10' });
    const bar = within(rowOf('Main feed')).getByRole('progressbar', { name: 'Main feed' });
    expect(bar.getAttribute('aria-valuetext')).toBe(`${stage}, 55%`);
    expect(words(rowOf('Main feed'))).toContain(stage);
    expect(words(rowOf('Main feed'))).toContain('55%');
  });

  it('a stage with no count yet is drawn as a bar that does not say how far', () => {
    sources.set([source({ status: 'updating' })]);
    sourceProgress.set({ s1: { stage: 'fetch', done: 0, total: 0, startedAt: 0, sawMatch: false } });
    show();

    const bar = within(rowOf('Main feed')).getByRole('progressbar', { name: 'Main feed' });
    expect(bar.hasAttribute('aria-valuenow')).toBe(false);
    expect(words(rowOf('Main feed'))).toContain(msg('transfers.sourcesStageFetch'));
  });

  it('a source whose last refresh failed says why in its row, and its error mark opens it', () => {
    sources.set([source({ status: 'error', lastError: 'typhon:sources.feed_invalid_json: bad json' })]);
    show();

    const reason = sourceErrorText('typhon:sources.feed_invalid_json: bad json');
    expect(words(rowOf('Main feed'))).toContain(reason);
    expect(within(rowOf('Main feed')).getByRole('button', { name: msg('common.error') })).toBeTruthy();
  });

  it('a source with warnings or a failed health check has a mark; a healthy one has none', () => {
    sources.set([
      source({ health: 'warning', warnings: ['duplicate entries'] }),
      source({ id: 's2', name: 'Mirror', health: 'error' }),
      source({ id: 's3', name: 'Fine' }),
    ]);
    show();

    expect(within(rowOf('Main feed')).getByRole('button', { name: msg('transfers.sourcesHealthWarning') })).toBeTruthy();
    expect(within(rowOf('Mirror')).getByRole('button', { name: msg('transfers.sourcesHealthError') })).toBeTruthy();
    const fine = within(rowOf('Fine'));
    for (const name of [msg('transfers.sourcesHealthWarning'), msg('transfers.sourcesHealthError'), msg('common.error')]) {
      expect(fine.queryByRole('button', { name })).toBeNull();
    }
  });

  it('an insecure source carries the warning; a secure one does not', () => {
    sources.set([source({ url: 'http://example.invalid/feed.json', insecure: true }), MIRROR]);
    show();

    const warning = msg('transfers.sourcesInsecureTooltip');
    expect(document.body.querySelectorAll('[aria-describedby]').length).toBeGreaterThan(0);
    const described = (name: string) =>
      Array.from(rowOf(name).querySelectorAll('[aria-describedby]')).some((el) =>
        (el.getAttribute('aria-describedby') ?? '').split(' ').some((id) => document.getElementById(id)?.textContent === warning),
      );
    expect(described('Main feed')).toBe(true);
    expect(described('Mirror')).toBe(false);
  });
});

describe('Sources: when a source was last refreshed', () => {
  it('says it in words; a source never refreshed says that', () => {
    const at = '2026-11-20T09:30:00Z';
    sources.set([source({ lastUpdatedAt: at }), source({ id: 's2', name: 'Mirror' })]);
    show();

    expect(words(rowOf('Main feed'))).toContain(relativeDate(at));
    expect(words(rowOf('Mirror'))).toContain(msg('transfers.sourcesNeverUpdated'));
  });

  // R3: the exact time is text for assistive tech and a hint on hover, as History says a record's time.
  it('R3: says the exact time to assistive tech and on hover', async () => {
    const at = '2026-11-20T09:30:00Z';
    sources.set([source({ lastUpdatedAt: at })]);
    show();

    const date = new Date(at);
    const exact = msg('transfers.sourcesExactTime', { date: longDate(date), time: clockTime(date) });
    const time = rowOf('Main feed').querySelector('time');
    expect(time?.getAttribute('datetime')).toBe(at);
    expect(time?.querySelector('.sr-only')?.textContent).toBe(msg('transfers.sourcesUpdatedAt', { when: exact }));
    expect(time?.tabIndex).toBe(-1);

    await user().hover(time as HTMLElement);
    await waitFor(() => expect(document.querySelector('[data-part="tooltip"]')?.textContent?.trim()).toBe(exact));
  });

  it('R3: has no exact time to give for a source never refreshed', () => {
    sources.set([source({ lastUpdatedAt: null })]);
    show();

    expect(rowOf('Main feed').querySelector('time')).toBeNull();
  });
});

// R3: a native title reaches a mouse only; the screen says what it has to say as text or through a Tooltip.
describe('Sources: no native title', () => {
  it('R3: carries none in a row of any state', () => {
    sources.set([
      source({ status: 'error', lastError: 'typhon:sources.feed_invalid_json: bad json', lastUpdatedAt: '2026-11-20T09:30:00Z' }),
      source({ id: 's2', name: 'Mirror', status: 'updating', insecure: true, url: 'http://mirror.invalid/feed.json' }),
      source({ id: 's3', name: 'Warned', health: 'warning', warnings: ['duplicate entries'] }),
    ]);
    sourceProgress.set({ s2: { stage: 'match', done: 5, total: 10, startedAt: 0, sawMatch: true } });
    show();

    expect(document.querySelector('[title]')).toBeNull();
  });

  // Read from the source: jsdom applies no stylesheet. A cut text with no title would hide its end.
  it('R3: shows the address and the reason of a failure whole, not cut with an ellipsis', () => {
    const css = styleOf(row, 'SourceRow');
    for (const selector of ['.source-url', '.source-error']) {
      expect(ruleBody(css, selector)['text-overflow'], selector).toBeUndefined();
      expect(ruleBody(css, selector)['overflow-wrap'], selector).toBe('anywhere');
    }
  });
});

describe('Sources: the counts of a row', () => {
  it('show how many entries the feed holds and how many matched, with those to review apart', () => {
    sources.set([MAIN, MIRROR]);
    show();

    const main = within(rowOf('Main feed')).getByRole('button', { name: msg('transfers.sourcesMatchReviewName', { matched: '7', review: '2' }) });
    expect(words(main)).toBe(`${msg('transfers.sourcesMatchName', { matched: '7' })} ${msg('transfers.sourcesReviewCount', { review: '2' })}`);
    const mirror = within(rowOf('Mirror')).getByRole('button', { name: msg('transfers.sourcesMatchName', { matched: '3' }) });
    expect(words(mirror)).toBe(msg('transfers.sourcesMatchName', { matched: '3' }));
    expect(words(rowOf('Main feed'))).toContain(msg('transfers.sourcesEntriesCount', { count: 10 }));
    expect(words(rowOf('Mirror'))).toContain(msg('transfers.sourcesEntriesCount', { count: 4 }));
  });

  it('open the source’s details', async () => {
    sources.set([MAIN]);
    show();

    await user().click(within(rowOf('Main feed')).getByRole('button', { name: msg('transfers.sourcesMatchReviewName', { matched: '7', review: '2' }) }));

    expect(await screen.findByRole('dialog', { name: 'Main feed' })).toBeTruthy();
  });
});

// R5: a list of rows, as Downloads and History draw theirs: each source one item, its name the row's
// main control, and what each number means said in the row instead of by a header no cell is tied to.
describe('Sources: the list', () => {
  it('R5: is a list named by the page, one item per source', () => {
    sources.set([MAIN, MIRROR]);
    show();

    const list = screen.getByRole('list', { name: msg('transfers.sourcesTitle') });
    expect(within(list).getAllByRole('listitem').map((item) => item.getAttribute('data-source-row'))).toEqual(['s1', 's2']);
  });

  it('R5: draws no header row of column names', () => {
    sources.set([MAIN]);
    show();

    for (const key of ['transfers.sourcesColName', 'transfers.sourcesColStatus', 'transfers.sourcesColUpdated', 'transfers.sourcesColEntries', 'transfers.sourcesColMatched'] as const) {
      expect(screen.queryByText(msg(key)), key).toBeNull();
    }
  });

  it('R5: the source’s name is the row’s first control, named by the name alone, and opens the details', async () => {
    sources.set([MAIN, MIRROR]);
    show();

    const name = within(rowOf('Mirror')).getByRole('button', { name: 'Mirror' });
    expect(name.classList.contains('text-link')).toBe(true);
    expect(rowOf('Mirror').querySelector('button')).toBe(name);

    await user().click(name);

    expect(await screen.findByRole('dialog', { name: 'Mirror' })).toBeTruthy();
  });

  it('R5: says when the source was refreshed in words of its own', () => {
    const at = '2026-11-20T09:30:00Z';
    sources.set([source({ lastUpdatedAt: at }), source({ id: 's2', name: 'Mirror' })]);
    show();

    const time = rowOf('Main feed').querySelector('time [aria-hidden="true"]');
    expect(words(time)).toBe(msg('transfers.sourcesUpdatedAt', { when: relativeDate(at) }));
    expect(words(rowOf('Mirror'))).toContain(msg('transfers.sourcesNeverUpdated'));
  });

  it('R5: the insecure mark is a picture beside the name; the warning is the name’s description', () => {
    sources.set([source({ url: 'http://example.invalid/feed.json', insecure: true })]);
    show();

    const name = within(rowOf('Main feed')).getByRole('button', { name: 'Main feed' });
    expect(describedControls(document, msg('transfers.sourcesInsecureTooltip'))).toEqual([name]);
    expect(rowOf('Main feed').querySelector('.insecure')?.getAttribute('aria-hidden')).toBe('true');
  });
});
