import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../i18n';
import { user } from '../testing/dom';
import NoticeHarness from '../testing/NoticeHarness.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  locale.set('en');
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

function setup(props: Record<string, unknown> = {}) {
  render(NoticeHarness, { target: app, props });
  return { keys: user() };
}

describe('Notice', () => {
  it('a failure is an alert: it is announced at once', () => {
    setup({ tone: 'danger' });
    const notice = screen.getByRole('alert');
    expect(notice.textContent).toContain('The catalog could not be loaded');
    expect(notice.dataset.tone).toBe('danger');
  });

  it('a warning, a success and a plain message are a status: announced politely', () => {
    for (const tone of ['warning', 'success', 'info']) {
      setup({ tone });
      const notice = screen.getByRole('status');
      expect(notice.dataset.tone).toBe(tone);
      expect(screen.queryByRole('alert')).toBeNull();
      document.getElementById('app')?.replaceChildren();
    }
  });

  it('shows its title above the text, and its icon is decoration', () => {
    setup({ tone: 'warning', title: 'Not everything could be measured' });
    const notice = screen.getByRole('status');
    const title = notice.querySelector('.title');
    expect(title?.textContent).toBe('Not everything could be measured');
    expect(notice.querySelector('.text')?.textContent?.trim()).toBe('The catalog could not be loaded');
    expect(notice.querySelector('.icon')?.getAttribute('aria-hidden')).toBe('true');
  });

  it('carries an action the page gives it, and pressing it does not close the notice', async () => {
    const onretry = vi.fn();
    const { keys } = setup({ tone: 'danger', withAction: true, onretry });
    await keys.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onretry).toHaveBeenCalledOnce();
    expect(screen.getByRole('alert')).not.toBeNull();
  });

  it('has no close button unless the page lets it be closed', () => {
    setup({ tone: 'danger' });
    expect(screen.queryByRole('button', { name: 'Close' })).toBeNull();
  });

  it('its close button is named, removes it, and focus goes on to the next control, not to the page', async () => {
    const { keys } = setup({ tone: 'warning', closable: true });
    const close = screen.getByRole('button', { name: 'Close' });
    close.focus();

    await keys.keyboard('{Enter}');

    expect(screen.queryByRole('status')).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
  });

  it('closed with nothing after it, focus goes back to the control before it', async () => {
    const { keys } = setup({ tone: 'warning', closable: true });
    screen.getByRole('button', { name: 'After' }).remove();
    screen.getByRole('button', { name: 'Close' }).focus();

    await keys.keyboard('{Enter}');

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Before' }));
  });

  it('is compact where it sits inside a card or a dialog', () => {
    setup({ tone: 'danger', compact: true });
    expect(screen.getByRole('alert').classList.contains('compact')).toBe(true);
  });
});
