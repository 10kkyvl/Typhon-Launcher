import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Source } from '../../lib/services/sources';
import { sourceErrorText } from '../../lib/sources/sourceErrors';
import { sourceProgress } from '../../lib/stores/sourceProgress';
import { refreshingAll, sources } from '../../lib/stores/sources';
import { user } from '../../lib/testing/dom';
import { describedControls, hintOnFocus } from '../../lib/testing/hints';
import Sources from './Sources.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

function source(over: Partial<Source>): Source {
  return {
    id: 's1', name: 'Main feed', type: 'json', url: 'https://example.invalid/feed.json', enabled: true, status: 'active', health: 'healthy',
    lastUpdatedAt: null, lastError: '', fingerprint: '', feedVersion: 1, entries: 10, invalid: 0, matched: 7, review: 2, unmatched: 1, createdAt: '',
    ...over,
  } as Source;
}

let app: HTMLDivElement;

beforeEach(() => {
  // jsdom has no ResizeObserver; the status filter measures its plate with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.unstubAllGlobals();
  sources.set([]);
  document.body.replaceChildren();
});

// Every hint of a source row reaches a keyboard and a screen-reader user: it is the description of a
// control in the Tab order, and keyboard focus on that control shows it.
async function expectReachable(text: string) {
  const controls = describedControls(document, text);
  expect(controls, text).toHaveLength(1);
  expect(await hintOnFocus(controls[0])).toBe(text);
}

describe('Sources: row hints without a mouse', () => {
  it('the insecure-source warning is the description of the source button', async () => {
    sources.set([source({ url: 'http://example.invalid/feed.json', insecure: true })]);
    render(Sources, { target: app, props: {} });
    await expectReachable(msg('transfers.sourcesInsecureTooltip'));
    const [control] = describedControls(document, msg('transfers.sourcesInsecureTooltip'));
    expect(control.textContent).toContain('Main feed');
  });

  it('the reason a source fails is reachable from its status', async () => {
    sources.set([source({ status: 'error', lastError: 'feed answered 503' })]);
    render(Sources, { target: app, props: {} });
    const reason = sourceErrorText('feed answered 503');
    await expectReachable(reason);
    expect(screen.getByRole('button', { name: msg('common.error'), description: reason })).toBeTruthy();
  });

  it('the warnings of a source are reachable from its health mark', async () => {
    sources.set([source({ health: 'warning', warnings: ['duplicate entries'] })]);
    render(Sources, { target: app, props: {} });
    const text = `${msg('transfers.sourcesHealthWarning')}: duplicate entries`;
    await expectReachable(text);
    expect(screen.getByRole('button', { name: msg('transfers.sourcesHealthWarning') })).toBeTruthy();
  });

  it('the match breakdown, unmatched count included, is reachable from the counts', async () => {
    sources.set([source({})]);
    render(Sources, { target: app, props: {} });
    await expectReachable(msg('transfers.sourcesMatchTooltip', { matched: '7', review: '2', unmatched: '1' }));
  });
});

describe('Sources: the status filter', () => {
  it('is a named radio group with a count on every choice, and choosing one filters the list', async () => {
    sources.set([
      source({ id: 's1', name: 'Main feed', status: 'active' }),
      source({ id: 's2', name: 'Old feed', status: 'disabled', enabled: false }),
    ]);
    render(Sources, { target: app, props: {} });

    const filter = screen.getByRole('radiogroup', { name: msg('transfers.sourcesStatusFilterLabel') });
    const all = within(filter).getByRole('radio', { name: `${msg('transfers.sourcesTabAll')} 2` });
    expect(all.getAttribute('aria-checked')).toBe('true');
    expect(screen.queryByRole('tablist')).toBeNull();
    expect(screen.getByText('Main feed')).toBeTruthy();
    expect(screen.getByText('Old feed')).toBeTruthy();

    const disabled = within(filter).getByRole('radio', { name: `${msg('transfers.sourcesTabDisabled')} 1` });
    await user().click(disabled);

    expect(disabled.getAttribute('aria-checked')).toBe('true');
    expect(all.getAttribute('aria-checked')).toBe('false');
    expect(screen.queryByText('Main feed')).toBeNull();
    expect(screen.getByText('Old feed')).toBeTruthy();
  });
});

describe('Sources: an empty list', () => {
  it('says so in a heading right under the page title', () => {
    render(Sources, { target: app, props: {} });

    expect(screen.getByRole('heading', { level: 1, name: msg('transfers.sourcesTitle') })).toBeTruthy();
    expect(screen.getByRole('heading', { level: 2, name: msg('transfers.sourcesEmptyTitle') })).toBeTruthy();
  });

  it('with sources filtered away, says so at the same level', async () => {
    sources.set([source({})]);
    render(Sources, { target: app, props: {} });
    await fireEvent.click(screen.getByRole('radio', { name: (name) => name.startsWith(msg('transfers.sourcesTabDisabled')) }));

    expect(screen.getByRole('heading', { level: 2, name: msg('transfers.sourcesEmptyFilteredTitle') })).toBeTruthy();
  });
});

describe('Sources: refreshing all', () => {
  afterEach(() => {
    refreshingAll.set(false);
    sourceProgress.set({});
  });

  // The button's text already says how far the refresh is; the bar inside only draws it.
  it('the refresh-all button is named by its text alone, the bar in it is hidden', () => {
    sources.set([source({ status: 'updating' })]);
    sourceProgress.set({ s1: { stage: 'match', done: 5, total: 10, startedAt: 0, sawMatch: true } });
    refreshingAll.set(true);
    render(Sources, { target: app, props: {} });

    expect(screen.getByRole('button', { name: msg('transfers.sourcesRefreshAllProgress', { percent: 55 }) })).toBeTruthy();
    expect(screen.getAllByRole('progressbar').map((bar) => bar.getAttribute('aria-label'))).toEqual(['Main feed']);
  });
});
