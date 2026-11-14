import { render } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { sourceTitle } from '../library/source';
import { user } from '../testing/dom';
import SourceBadge from './SourceBadge.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

// The badge sits on a card or in a row before their controls: a focus stop of its own would take the
// focus a card hands back, and inside a row's button it would be a control in a control.
describe('SourceBadge', () => {
  it('is a plain label: no native title, no tabindex', () => {
    render(SourceBadge, { target: app, props: { source: 'steam' } });

    expect(app.querySelector('[title]')).toBeNull();
    expect(app.querySelector('[tabindex]')).toBeNull();
  });

  it('is not reached by Tab', async () => {
    const before = document.createElement('button');
    before.textContent = 'Before';
    const after = document.createElement('button');
    after.textContent = 'After';
    app.append(before);
    render(SourceBadge, { target: app, props: { source: 'disk' } });
    app.append(after);
    before.focus();

    await user().tab();

    expect(document.activeElement).toBe(after);
  });

  it('keeps the full sentence in its text, for a screen reader', () => {
    render(SourceBadge, { target: app, props: { source: 'steam' } });
    const badge = app.querySelector<HTMLElement>('.badge');

    expect(badge?.textContent).toContain(msg('games.externalBadgeTitle', { provider: msg('games.sourceSteam') }));
    expect(badge?.textContent).toContain(sourceTitle('steam'));
  });
});
