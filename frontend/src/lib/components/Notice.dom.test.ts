import { render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../i18n';
import { user } from '../testing/dom';
import NoticeHarness from '../testing/NoticeHarness.svelte';
import noticeSource from './Notice.svelte?raw';

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

// The test run does not load component styles into jsdom, so "unseen" is the class that hides words
// (`visibility: hidden`, checked against the source below), not a computed style.
function seen(el: Element): boolean {
  return el.isConnected && !el.closest('.unseen');
}

it('hides unseen words with visibility, which keeps their place in the layout', () => {
  const style = /<style[^>]*>([\s\S]*?)<\/style>/.exec(noticeSource)?.[1] ?? '';
  expect(/\.unseen\s*\{([^}]*)\}/.exec(style)?.[1].trim()).toBe('visibility: hidden;');
});

// The words of a polite region come into view after the region is on the page, so wait for them.
async function said(role: 'alert' | 'status'): Promise<HTMLElement> {
  const region = screen.getByRole(role);
  await waitFor(() => expect(seen(within(region).getByText(TEXT))).toBe(true));
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

  it('renders its title as a heading of the level a section gives it, and as plain text otherwise', async () => {
    setup({ tone: 'danger', title: 'Not everything could be measured', level: 2 });
    const region = await said('alert');
    const heading = within(region).getByRole('heading', { level: 2, name: 'Not everything could be measured' });
    expect(heading.classList.contains('title')).toBe(true);
    document.getElementById('app')?.replaceChildren();

    setup({ tone: 'danger', title: 'Not everything could be measured' });
    await said('alert');
    expect(screen.queryByRole('heading')).toBeNull();
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

  // Some screen readers do not announce a polite region that is inserted together with its content. Its
  // words are laid out from the first frame, so the plate never changes size, and stay unseen (out of the
  // accessibility tree) until the region has been on the page. jsdom proves the DOM and the styles; that
  // the words coming into view are announced like an insertion is for a screen reader to confirm.
  it.each(['warning', 'success', 'info'] as const)('a %s notice lays its words out at once and shows them once its region is on the page', async (tone) => {
    setup({ tone, title: 'Requests' });
    const region = screen.getByRole('status');
    const title = region.querySelector('.title') as HTMLElement;
    const text = region.querySelector('.text') as HTMLElement;
    expect(text.textContent).toContain(TEXT);
    expect(seen(title)).toBe(false);
    expect(seen(text)).toBe(false);

    await waitFor(() => expect(seen(text)).toBe(true));
    expect(seen(title)).toBe(true);
    expect(region.querySelector('.text')).toBe(text);
    expect(screen.getByRole('status')).toBe(region);
  });

  // An alert is announced when it is inserted with its content: a failure shows its words at once.
  it('a danger notice shows its words in the first frame', () => {
    setup({ tone: 'danger', title: 'Requests' });
    const region = screen.getByRole('alert');
    expect(seen(within(region).getByText(TEXT))).toBe(true);
    expect(seen(within(region).getByText('Requests'))).toBe(true);
  });

  it('removed before its words show, it leaves no frame behind that writes to it', async () => {
    let gone = false;
    const late: number[] = [];
    const frame = window.requestAnimationFrame.bind(window);
    const raf = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((step) =>
      frame((time) => {
        if (gone) late.push(time);
        step(time);
      }),
    );
    const errors = vi.spyOn(console, 'error');
    const { unmount } = render(NoticeHarness, { target: app, props: { tone: 'warning' } });
    expect(raf).toHaveBeenCalled();

    unmount();
    gone = true;
    await new Promise((settled) => setTimeout(settled, 100));

    expect(late).toEqual([]);
    expect(errors).not.toHaveBeenCalled();
    raf.mockRestore();
    errors.mockRestore();
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
