import { render, screen, waitFor, within } from '@testing-library/svelte';
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

const TEXT = 'The catalog could not be loaded';

// The words of a live region arrive after the region does, so wait for them.
async function said(role: 'alert' | 'status'): Promise<HTMLElement> {
  const region = screen.getByRole(role);
  await waitFor(() => expect(region.textContent).toContain(TEXT));
  return region;
}

describe('Notice', () => {
  it('a failure is an alert: it is announced at once', async () => {
    setup({ tone: 'danger' });
    const region = await said('alert');
    expect(region.closest<HTMLElement>('.notice')?.dataset.tone).toBe('danger');
  });

  it('a warning, a success and a plain message are a status: announced politely', async () => {
    for (const tone of ['warning', 'success', 'info']) {
      setup({ tone });
      const region = await said('status');
      expect(region.closest<HTMLElement>('.notice')?.dataset.tone).toBe(tone);
      expect(screen.queryByRole('alert')).toBeNull();
      document.getElementById('app')?.replaceChildren();
    }
  });

  it('shows its title above the text, and its icon is decoration', async () => {
    setup({ tone: 'warning', title: 'Not everything could be measured' });
    const region = await said('status');
    const title = region.querySelector('.title');
    expect(title?.textContent).toBe('Not everything could be measured');
    expect(region.querySelector('.text')?.textContent?.trim()).toBe(TEXT);
    expect(region.closest('.notice')?.querySelector('.icon')?.getAttribute('aria-hidden')).toBe('true');
  });

  // A live region reads out all of itself on every change inside it: the buttons stay out of it, so a
  // button whose label changes is not a new announcement, and the message is read without button names.
  it('announces its title and text only, with the action and the close button outside the region', async () => {
    setup({ tone: 'danger', title: 'Requests', withAction: true, closable: true });
    const region = await said('alert');
    expect(region.textContent).toContain('Requests');
    expect(within(region).queryAllByRole('button')).toEqual([]);
    const plate = region.closest('.notice') as HTMLElement;
    for (const name of ['Retry', 'Close']) {
      const button = within(plate).getByRole('button', { name });
      expect(region.contains(button), name).toBe(false);
    }
  });

  // Some screen readers do not announce a polite region that is inserted together with its content.
  // jsdom cannot hear the announcement: this proves the order in the DOM, a screen reader the rest.
  it.each(['danger', 'warning'] as const)('a %s notice puts its region on the page first and its words into it after', async (tone) => {
    setup({ tone, title: 'Requests' });
    const role = tone === 'danger' ? 'alert' : 'status';
    const region = screen.getByRole(role);
    expect(region.textContent?.trim()).toBe('');
    expect(screen.queryByText(TEXT)).toBeNull();
    await waitFor(() => expect(region.textContent).toContain(TEXT));
    expect(screen.getByRole(role)).toBe(region);
    expect(region.textContent).toContain('Requests');
  });

  // A standing fact of the page is read in the flow of the page, not announced on every visit.
  it('without announce it is no live region and shows its words at once', () => {
    setup({ tone: 'danger', title: 'History', announce: false, withAction: true });
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();
    const text = screen.getByText(TEXT);
    expect(text.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
    expect(screen.getByText('History')).not.toBeNull();
  });

  it('carries an action the page gives it, and pressing it does not close the notice', async () => {
    const onretry = vi.fn();
    const { keys } = setup({ tone: 'danger', withAction: true, onretry });
    await keys.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onretry).toHaveBeenCalledOnce();
    expect(screen.getByRole('alert')).not.toBeNull();
  });

  it('adds no room for an action the page does not give it', () => {
    setup({ tone: 'danger' });
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
    expect(document.querySelector('.notice .action')).toBeNull();
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
    expect(screen.getByRole('alert').closest('.notice')?.classList.contains('compact')).toBe(true);
  });
});
