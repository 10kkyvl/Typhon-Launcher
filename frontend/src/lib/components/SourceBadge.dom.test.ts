import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { sourceTitle } from '../library/source';
import { user } from '../testing/dom';
import { hintOnFocus } from '../testing/hints';
import { HOVER_DELAY } from './hoverIntent';
import SourceBadge from './SourceBadge.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.useRealTimers();
  document.body.replaceChildren();
});

const tip = () => document.querySelector('[data-part="tooltip"]')?.textContent?.trim() ?? null;

describe('SourceBadge', () => {
  it('has no native title; the full sentence stays in what a screen reader reads', () => {
    render(SourceBadge, { target: app, props: { source: 'steam' } });
    const badge = app.querySelector<HTMLElement>('.badge');

    expect(app.querySelector('[title]')).toBeNull();
    expect(badge?.textContent).toContain(msg('games.externalBadgeTitle', { provider: msg('games.sourceSteam') }));
  });

  it('shows the full sentence as a tooltip when the pointer rests on it', async () => {
    vi.useFakeTimers();
    render(SourceBadge, { target: app, props: { source: 'disk' } });
    const badge = app.querySelector<HTMLElement>('.badge') as HTMLElement;

    await user().hover(badge);
    vi.advanceTimersByTime(HOVER_DELAY);
    await tick();
    await tick();

    expect(tip()).toBe(sourceTitle('disk'));
  });

  it('shows the full sentence as a tooltip on keyboard focus', async () => {
    render(SourceBadge, { target: app, props: { source: 'typhon' } });
    const badge = app.querySelector<HTMLElement>('.badge') as HTMLElement;

    expect(await hintOnFocus(badge)).toBe(sourceTitle('typhon'));
  });
});
