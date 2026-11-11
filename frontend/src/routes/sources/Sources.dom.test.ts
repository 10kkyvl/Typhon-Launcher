import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Source } from '../../lib/services/sources';
import { sourceErrorText } from '../../lib/sources/sourceErrors';
import { sources } from '../../lib/stores/sources';
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
  // jsdom has no ResizeObserver; the status filter tabs measure their indicator with one.
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
