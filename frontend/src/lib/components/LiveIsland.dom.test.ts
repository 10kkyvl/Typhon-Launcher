import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { liveIsland, liveIslandCurrent, liveIslandOn } from '../stores/liveIsland';
import { user } from '../testing/dom';
import { HOVER_DELAY } from './hoverIntent';
import LiveIslandHarness from '../testing/LiveIslandHarness.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

let app: HTMLDivElement;

beforeEach(() => {
  liveIsland.clear();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  liveIsland.clear();
  document.body.replaceChildren();
});

function setup(props: { dialog?: boolean; onescape?: () => void } = {}) {
  const view = render(LiveIslandHarness, { target: app, props });
  const keys = user();
  const island = () => app.querySelector<HTMLElement>('[data-part="live-island"]') as HTMLElement;
  const pill = () => island().querySelector<HTMLButtonElement>('.head') as HTMLButtonElement;
  const card = () => island().querySelector<HTMLElement>('.card') as HTMLElement;
  const status = () => screen.getByTestId('live-island-status');
  const alert = () => screen.getByTestId('live-island-alert');
  return { keys, island, pill, card, status, alert, rerender: view.rerender };
}

describe('LiveIsland: what it shows', () => {
  it('is hidden with nothing to say, and claims the messages while mounted', async () => {
    const { island } = setup();
    await tick();
    expect(island().dataset.phase).toBe('hidden');
    expect(island().hasAttribute('inert')).toBe(true);
    expect(get(liveIslandOn)).toBe(true);
  });

  it('shows one message at a time in a compact pill, the next when it goes', async () => {
    const { island, pill } = setup();
    liveIsland.show({ id: 'a', text: 'Saved' });
    liveIsland.show({ id: 'b', text: 'Copied' });
    await tick();
    expect(island().dataset.phase).toBe('compact');
    expect(pill().textContent).toContain('Saved');
    expect(island().textContent).not.toContain('Copied');
    liveIsland.hide('a');
    await tick();
    expect(pill().textContent).toContain('Copied');
  });

  it('shows the repeats of a message as ×N, and says them', async () => {
    const { pill } = setup();
    liveIsland.show({ id: 'a', text: 'Saved', count: 3 });
    await tick();
    expect(pill().textContent).toContain('×3');
    expect(pill().textContent).toContain(msg('ui.toastRepeated', { count: 3 }));
  });

  it('wears the tone and kind of its message', async () => {
    const { island } = setup();
    liveIsland.show({ id: 'a', text: 'Disk is full', tone: 'danger', kind: 'error' });
    await tick();
    expect(island().classList.contains('danger')).toBe(true);
    expect(island().dataset.kind).toBe('error');
  });
});

describe('LiveIsland: what a screen reader is told', () => {
  it('says each message once, when it comes: errors in the alert region, the rest in the status one', async () => {
    const { status, alert } = setup();
    liveIsland.show({ id: 'plain', text: 'Saved' });
    liveIsland.show({ id: 'error', text: 'Disk is full', tone: 'danger' });
    await tick();
    expect(within(alert()).getByText('Disk is full')).toBeTruthy();
    expect(within(status()).getByText('Saved')).toBeTruthy();
    // The waiting message was said when it came, not again when its turn comes.
    liveIsland.hide('error');
    await tick();
    expect(within(status()).getAllByText('Saved')).toHaveLength(1);
    expect(alert().textContent?.trim()).toBe('');
  });

  it('says a save conflict in the alert region, as an error', async () => {
    const { status, alert } = setup();
    liveIsland.show({ id: 'c', text: 'Save conflict', detail: 'Hades', kind: 'conflict', tone: 'neutral' });
    await tick();
    expect(alert().textContent).toContain('Save conflict. Hades');
    expect(status().textContent?.trim()).toBe('');
  });

  it('says the title and the detail with the line', async () => {
    const { alert } = setup();
    liveIsland.show({ id: 'd', text: 'Download failed', title: 'Elden Ring', detail: 'No peers', kind: 'error' });
    await tick();
    expect(alert().textContent).toContain('Download failed. Elden Ring. No peers');
  });
});

describe('LiveIsland: keyboard and focus', () => {
  it('Enter opens it into its card with focus inside; Escape closes it and focus is on the pill', async () => {
    const { keys, island, pill, card } = setup();
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    pill().focus();
    await keys.keyboard('{Enter}');
    await tick();
    expect(island().dataset.phase).toBe('expanded');
    expect(pill().getAttribute('aria-expanded')).toBe('true');
    expect(card().hasAttribute('inert')).toBe(false);
    expect(card().contains(document.activeElement)).toBe(true);

    await keys.keyboard('{Escape}');
    await tick();
    expect(island().dataset.phase).toBe('compact');
    expect(document.activeElement).toBe(pill());
    expect(pill().getAttribute('aria-expanded')).toBe('false');
  });

  it('the open card\'s head is named by what it says, with collapsing as its description', async () => {
    const { keys, pill, card } = setup();
    liveIsland.show({ id: 'd', text: 'Download failed', title: 'Elden Ring', detail: 'No peers', kind: 'error' });
    await tick();
    await keys.click(pill());
    await tick();
    const head = within(card()).getByRole('button', { name: 'Elden Ring Download failed No peers', description: msg('ui.liveIslandCollapse') });
    expect(head.getAttribute('aria-expanded')).toBe('true');
  });

  it('Space opens it as well', async () => {
    const { keys, island, pill } = setup();
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    pill().focus();
    await keys.keyboard(' ');
    await tick();
    expect(island().dataset.phase).toBe('expanded');
  });

  it('takes no focus when a message comes', async () => {
    setup();
    const before = screen.getByRole('button', { name: 'Before' });
    before.focus();
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    expect(document.activeElement).toBe(before);
  });

  it('a message that goes while focus is inside gives focus back where it was before', async () => {
    const { keys, pill } = setup();
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    const before = screen.getByRole('button', { name: 'Before' });
    before.focus();
    await keys.tab();
    expect(document.activeElement).toBe(pill());
    await keys.keyboard('{Enter}');
    await tick();
    liveIsland.hide('a');
    await tick();
    await tick();
    expect(document.activeElement).toBe(before);
  });

  it('where focus came from belongs to one message: a later message does not send focus back there', async () => {
    const { keys, pill } = setup();
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    const before = screen.getByRole('button', { name: 'Before' });
    before.focus();
    await keys.tab();
    expect(document.activeElement).toBe(pill());
    // A press on a blank spot: focus leaves for <body>, with no element to name.
    await keys.click(document.body);
    liveIsland.hide('a');
    await tick();
    liveIsland.show({ id: 'b', text: 'Copied' });
    await tick();
    await keys.click(pill());
    liveIsland.hide('b');
    await tick();
    await tick();
    expect(document.activeElement).not.toBe(before);
  });

  it('a message that goes with focus left inside does not hold the next one\'s clock, focusout or not', async () => {
    vi.useFakeTimers();
    const { pill } = setup();
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    // Focus with nowhere known to go back to, and no focusout when the island hides (a webview may send none).
    pill().focus();
    await tick();
    liveIsland.hide('a');
    await tick();
    await tick();
    liveIsland.show({ id: 'b', text: 'Copied', ttl: 1000 });
    await tick();
    vi.advanceTimersByTime(1000);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('Escape in a page field that answers it itself acts once: a card opened by the pointer stays', async () => {
    vi.useFakeTimers();
    const onescape = vi.fn();
    const { keys, island } = setup({ onescape });
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    island().dispatchEvent(new PointerEvent('pointerenter', { pointerType: 'mouse' }));
    vi.advanceTimersByTime(HOVER_DELAY);
    await tick();
    expect(island().dataset.phase).toBe('expanded');

    screen.getByRole('textbox', { name: 'Search' }).focus();
    await keys.keyboard('{Escape}');
    await tick();
    expect(onescape).toHaveBeenCalledTimes(1);
    expect(island().dataset.phase).toBe('expanded');
  });

  it('a press outside closes the open card; one on the page with the card closed changes nothing', async () => {
    const { keys, island, pill } = setup();
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    await keys.click(pill());
    await tick();
    expect(island().dataset.phase).toBe('expanded');
    await keys.click(screen.getByRole('button', { name: 'Before' }));
    await tick();
    expect(island().dataset.phase).toBe('compact');
    await keys.click(screen.getByRole('button', { name: 'Before' }));
    await tick();
    expect(island().dataset.phase).toBe('compact');
  });

  it('with a dialog open, Escape closes the dialog and the island keeps showing', async () => {
    const { keys, island } = setup({ dialog: true });
    await tick();
    const dialog = screen.getByRole('dialog', { name: 'Install' });
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(island().closest('[inert]')).toBeNull();

    await keys.keyboard('{Escape}');
    await tick();
    expect(screen.queryByRole('dialog', { name: 'Install' })).toBeNull();
    expect(island().dataset.phase).toBe('compact');
  });

  it('with a dialog open, Escape is the dialog\'s even from inside the open island', async () => {
    const { keys, island, pill } = setup({ dialog: true });
    await tick();
    liveIsland.show({ id: 'a', text: 'Saved' });
    await tick();
    await keys.click(pill());
    await tick();
    expect(island().dataset.phase).toBe('expanded');
    expect(island().contains(document.activeElement)).toBe(true);

    await keys.keyboard('{Escape}');
    await tick();
    expect(screen.queryByRole('dialog', { name: 'Install' })).toBeNull();
    expect(get(liveIslandCurrent)?.id).toBe('a');
  });
});

describe('LiveIsland: actions, dismissal and the clock', () => {
  it('runs an action once and the message goes', async () => {
    const { keys, pill } = setup();
    const run = vi.fn();
    liveIsland.show({ id: 'a', text: 'Removed', actions: [{ label: 'Undo', run }] });
    await tick();
    await keys.click(pill());
    await tick();
    const undo = screen.getByRole('button', { name: 'Undo' });
    await keys.click(undo);
    undo.click();
    await tick();
    expect(run).toHaveBeenCalledTimes(1);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  // What the action opens (a dialog) takes the focused element as the place to give focus back to:
  // that must be where the player was, not the island's button, which goes with the message.
  it('an action pressed from the keyboard runs with focus back where it was before the island', async () => {
    const { keys, pill } = setup();
    let focusedAtRun: Element | null = null;
    liveIsland.show({ id: 'a', text: 'Removed', actions: [{ label: 'Undo', run: () => (focusedAtRun = document.activeElement) }] });
    await tick();
    const before = screen.getByRole('button', { name: 'Before' });
    before.focus();
    await keys.tab();
    expect(document.activeElement).toBe(pill());
    await keys.keyboard('{Enter}');
    await tick();
    await keys.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Undo' }));

    await keys.keyboard('{Enter}');

    expect(focusedAtRun).toBe(before);
    await tick();
    expect(document.activeElement).toBe(before);
  });

  // A press that gave the button no focus (a click in the macOS WebKit view) moves nothing.
  it('an action pressed without focus in the island leaves focus where it is', async () => {
    const { pill } = setup();
    let focusedAtRun: Element | null = null;
    liveIsland.show({ id: 'a', text: 'Removed', actions: [{ label: 'Undo', run: () => (focusedAtRun = document.activeElement) }] });
    await tick();
    const search = screen.getByRole('textbox', { name: 'Search' });
    search.focus();
    fireEvent.click(pill());
    await tick();

    fireEvent.click(screen.getByRole('button', { name: 'Undo' }));

    expect(focusedAtRun).toBe(search);
    await tick();
    expect(document.activeElement).toBe(search);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('draws at most two actions, the first as the primary one, and a dismiss button', async () => {
    const { keys, pill, card } = setup();
    const run = vi.fn();
    liveIsland.show({ id: 'a', text: 'A', actions: [{ label: 'One', run }, { label: 'Two', run }, { label: 'Three', run }] });
    await tick();
    await keys.click(pill());
    await tick();
    const buttons = within(card()).getAllByRole('button').map((b) => b.textContent?.trim());
    expect(buttons).toEqual(['A', 'One', 'Two', msg('ui.toastDismiss')]);
    expect(within(card()).getByRole('button', { name: 'One' }).classList.contains('primary')).toBe(true);
  });

  it('dismissal takes the message away, through its own close when it has one', async () => {
    const { keys, pill } = setup();
    const close = vi.fn(() => liveIsland.hide('a'));
    liveIsland.show({ id: 'a', text: 'A', close });
    await tick();
    await keys.click(pill());
    await tick();
    await keys.click(screen.getByRole('button', { name: msg('ui.toastDismiss') }));
    expect(close).toHaveBeenCalledTimes(1);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('stops the clock while open, hovered or holding focus', async () => {
    vi.useFakeTimers();
    const { keys, pill, island } = setup();
    liveIsland.show({ id: 'a', text: 'Saved', ttl: 1000 });
    await tick();

    island().dispatchEvent(new PointerEvent('pointerenter', { pointerType: 'mouse' }));
    await tick();
    vi.advanceTimersByTime(5000);
    expect(get(liveIslandCurrent)?.id).toBe('a');
    island().dispatchEvent(new PointerEvent('pointerleave', { pointerType: 'mouse' }));
    await tick();

    pill().focus();
    await tick();
    vi.advanceTimersByTime(5000);
    expect(get(liveIslandCurrent)?.id).toBe('a');

    await keys.keyboard('{Enter}');
    await tick();
    vi.advanceTimersByTime(5000);
    expect(get(liveIslandCurrent)?.id).toBe('a');
  });

  it('runs the clock while nobody reads it', async () => {
    vi.useFakeTimers();
    setup();
    liveIsland.show({ id: 'a', text: 'Saved', ttl: 1000 });
    await tick();
    vi.advanceTimersByTime(1000);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });
});
