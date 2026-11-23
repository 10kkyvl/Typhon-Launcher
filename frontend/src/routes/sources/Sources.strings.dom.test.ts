import { render, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { sourceProgress } from '../../lib/stores/sourceProgress';
import { sources } from '../../lib/stores/sources';
import { describedControls } from '../../lib/testing/hints';
import Sources from './Sources.svelte';
import { mountPoint, resetScreen, rowOf, source } from './sourcesTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// The two messages a translator may word and order differently, drawn so a test can tell them from a
// sentence the code glued together.
vi.mock('../../lib/i18n', async (actual) => {
  const real = await actual<typeof import('../../lib/i18n')>();
  const marked: Record<string, (p: Record<string, unknown>) => string> = {
    'transfers.sourcesHealthHint': (p) => `«${p.warnings}» ← ${p.label}`,
    'transfers.sourcesStageProgress': (p) => `${p.percent} % / ${p.stage}`,
  };
  return { ...real, msg: ((key: string, params?: Record<string, unknown>) => marked[key]?.(params ?? {}) ?? real.msg(key as never, params as never)) as typeof real.msg };
});

let app: HTMLDivElement;

beforeEach(() => {
  app = mountPoint();
});

afterEach(() => {
  resetScreen();
});

// R4: a sentence glued from its parts cannot be reordered by a translation.
describe('Sources: the sentences of a row come from the catalog', () => {
  it('R4: the health hint is the catalog’s message with its label and warnings', () => {
    sources.set([source({ health: 'warning', warnings: ['duplicate entries'] })]);
    render(Sources, { target: app, props: {} });

    const text = `«duplicate entries» ← ${msg('transfers.sourcesHealthWarning')}`;
    expect(describedControls(document, text)).toHaveLength(1);
  });

  it('R4: the live stage the bar reports is the catalog’s message with its stage and percent', () => {
    sources.set([source({ status: 'updating' })]);
    sourceProgress.set({ s1: { stage: 'match', done: 5, total: 10, startedAt: 0, sawMatch: true } });
    render(Sources, { target: app, props: {} });

    const bar = within(rowOf('Main feed')).getByRole('progressbar', { name: 'Main feed' });
    expect(bar.getAttribute('aria-valuetext')).toBe(`55 % / ${msg('transfers.sourcesStageMatch', { done: '5', total: '10' })}`);
  });
});
