import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { liveIsland, liveIslandCurrent } from '../stores/liveIsland';
import { ACTION_MS, PLAIN_MS, QUEUE_STALE_MS, URGENT_MS, holdToasts, toast, toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import IslandShellHarness from '../testing/IslandShellHarness.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// What a toast raised anywhere becomes in the main window: with motion on, a message in the live
// island and nothing in the corner stack; with motion off (the animations setting puts `no-anim` on
// the document), the stack as before and no island.

let app: HTMLDivElement;

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  toasts.set([]);
  holdToasts(false);
  liveIsland.clear();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  document.documentElement.classList.remove('no-anim');
  toasts.set([]);
  holdToasts(false);
  liveIsland.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const settle = async () => {
  await Promise.resolve();
  await tick();
};

function setup(props: { dialog?: boolean } = {}) {
  render(IslandShellHarness, { target: app, props });
  const island = () => app.querySelector<HTMLElement>('[data-part="live-island"]');
  const pill = () => island()?.querySelector<HTMLButtonElement>('.head') as HTMLButtonElement;
  const stack = () => screen.getByRole('region', { name: msg('ui.toastsLabel') });
  const inStack = () => [...app.querySelectorAll('[data-part="toast"]')].map((el) => el.textContent?.trim() ?? '');
  // The pill's line and its repeats; what waits behind it is counted apart.
  const onIsland = () =>
    island()?.dataset.phase === 'hidden' ? '' : [...pill().querySelectorAll('.text, .times')].map((el) => el.textContent?.trim()).join(' ');
  return { keys: user(), island, pill, stack, inStack, onIsland };
}

describe('a toast with motion on and off', () => {
  it('with motion on, shows in the island and not in the stack', async () => {
    const { inStack, onIsland } = setup();
    toast('Saved');
    await tick();
    expect(onIsland()).toContain('Saved');
    expect(inStack()).toEqual([]);
  });

  it('with motion off, shows in the stack and there is no island', async () => {
    document.documentElement.classList.add('no-anim');
    const { inStack, island } = setup();
    toast('Saved');
    await tick();
    expect(island()).toBeNull();
    expect(inStack().join()).toContain('Saved');
  });

  it('switching the setting moves the next message to the other place', async () => {
    const { inStack, onIsland, island } = setup();
    toast('First');
    await tick();
    expect(onIsland()).toContain('First');

    document.documentElement.classList.add('no-anim');
    await settle();
    toast('Second');
    await tick();
    expect(island()).toBeNull();
    expect(inStack().join()).toContain('Second');

    document.documentElement.classList.remove('no-anim');
    await settle();
    toast('Third', 'danger');
    await tick();
    expect(onIsland()).toContain('Third');
    expect(inStack()).toEqual([]);
  });
});

describe('what a toast has survives in the island', () => {
  it('its kind: tone, icon and the alert region for an error', async () => {
    const { island } = setup();
    toast('Disk is full', 'danger');
    await tick();
    expect(island()?.classList.contains('danger')).toBe(true);
    expect(island()?.querySelector('.head svg')).not.toBeNull();
    expect(within(screen.getByTestId('live-island-alert')).getByText('Disk is full')).toBeTruthy();
    expect(screen.getByTestId('live-island-status').textContent?.trim()).toBe('');
  });

  it.each([
    ['info', 'lucide-info'],
    ['success', 'lucide-circle-check'],
    ['warning', 'lucide-triangle-alert'],
    ['danger', 'lucide-circle-alert'],
  ] as const)('its icon: a %s toast wears %s', async (kind, icon) => {
    const { island } = setup();
    toast('Message', kind);
    await tick();
    expect(island()?.querySelector(`.head svg.${icon}`)).not.toBeNull();
  });

  it('its action: runs once, and the message goes', async () => {
    const { keys, pill, island } = setup();
    const run = vi.fn();
    toast('Removed', 'info', { label: 'Undo', run });
    await tick();
    await keys.click(pill());
    await tick();
    const undo = within(island() as HTMLElement).getByRole('button', { name: 'Undo' });
    await keys.click(undo);
    undo.click();
    await tick();
    expect(run).toHaveBeenCalledTimes(1);
    expect(get(toasts)).toEqual([]);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('its repeats, as ×N', async () => {
    const { onIsland } = setup();
    toast('Saved');
    toast('Saved');
    await tick();
    expect(onIsland()).toContain('×2');
  });

  it('dismissal lets the toast go', async () => {
    const { keys, pill, island } = setup();
    toast('Saved');
    await tick();
    await keys.click(pill());
    await tick();
    await keys.click(within(island() as HTMLElement).getByRole('button', { name: msg('ui.toastDismiss') }));
    expect(get(toasts)).toEqual([]);
  });

  it.each([
    ['plain', PLAIN_MS, () => toast('Saved')],
    ['urgent', URGENT_MS, () => toast('Disk is full', 'danger')],
    ['with an action', ACTION_MS, () => toast('Removed', 'info', { label: 'Undo', run: () => {} })],
  ])('its time on screen: %s', async (_name, ms, raise) => {
    vi.useFakeTimers();
    const { onIsland } = setup();
    raise();
    await tick();
    vi.advanceTimersByTime(ms - 1);
    await tick();
    expect(onIsland()).not.toBe('');
    vi.advanceTimersByTime(1);
    await tick();
    expect(get(toasts)).toEqual([]);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('its clock stands while the island is hovered, focused or open', async () => {
    vi.useFakeTimers();
    const { keys, island, pill } = setup();
    toast('Saved');
    await tick();
    island()?.dispatchEvent(new PointerEvent('pointerenter', { pointerType: 'mouse' }));
    await tick();
    vi.advanceTimersByTime(PLAIN_MS * 3);
    expect(get(toasts).map((t) => t.message)).toEqual(['Saved']);
    island()?.dispatchEvent(new PointerEvent('pointerleave', { pointerType: 'mouse' }));
    pill().focus();
    await tick();
    vi.advanceTimersByTime(PLAIN_MS * 3);
    expect(get(toasts).map((t) => t.message)).toEqual(['Saved']);
    await keys.keyboard('{Enter}');
    await tick();
    vi.advanceTimersByTime(PLAIN_MS * 3);
    expect(get(toasts).map((t) => t.message)).toEqual(['Saved']);
  });
});

describe('motion turned off while the island holds messages', () => {
  it('hands the toasts back to the stack within its slots, the rest waiting', async () => {
    const { inStack } = setup();
    for (let i = 1; i <= 5; i++) toast(`Note ${i}`);
    await tick();
    document.documentElement.classList.add('no-anim');
    await settle();
    expect(inStack()).toHaveLength(2);
  });
});

describe('two and three messages', () => {
  it('an error goes before a plain note that came first, and the note comes back with its full time', async () => {
    vi.useFakeTimers();
    const { onIsland } = setup();
    toast('Saved');
    await tick();
    vi.advanceTimersByTime(1000);
    toast('Disk is full', 'danger');
    await tick();
    expect(onIsland()).toContain('Disk is full');
    vi.advanceTimersByTime(URGENT_MS);
    await tick();
    expect(onIsland()).toContain('Saved');
    // It had a second on screen before it gave way: three more, and then it goes.
    vi.advanceTimersByTime(PLAIN_MS - 1000 - 1);
    await tick();
    expect(onIsland()).toContain('Saved');
    vi.advanceTimersByTime(1);
    await tick();
    expect(get(toasts)).toEqual([]);
  });

  it('three plain ones each get their full time, in the order they came', async () => {
    vi.useFakeTimers();
    const { onIsland } = setup();
    toast('One');
    toast('Two');
    toast('Three');
    await tick();
    const seen: string[] = [];
    for (let i = 0; i < 3; i++) {
      seen.push(onIsland());
      vi.advanceTimersByTime(PLAIN_MS - 1);
      await tick();
      expect(onIsland()).toBe(seen[i]);
      vi.advanceTimersByTime(1);
      await tick();
    }
    expect(seen).toEqual(['One', 'Two', 'Three']);
    expect(onIsland()).toBe('');
  });
});

describe('a burst', () => {
  it('of twenty plain notes: those whose turn comes within the stack\'s limit are shown, the rest dropped', async () => {
    vi.useFakeTimers();
    const { onIsland } = setup();
    for (let i = 1; i <= 20; i++) toast(`Note ${i}`);
    await tick();
    const shown: [number, string][] = [];
    for (let at = 0; at <= 30_000; at += 1000) {
      const now = onIsland();
      if (now && shown.at(-1)?.[1] !== now) shown.push([at, now]);
      vi.advanceTimersByTime(1000);
      await tick();
    }
    // Each gets its 4 s; the fourth starts at 12 s, which is the limit; at 16 s every one left has waited longer.
    expect(shown).toEqual([[0, 'Note 1'], [4000, 'Note 2'], [8000, 'Note 3'], [12_000, 'Note 4']]);
    expect(PLAIN_MS * 3).toBeLessThanOrEqual(QUEUE_STALE_MS);
    expect(get(toasts)).toEqual([]);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('of ten errors: none is dropped, each gets its time', async () => {
    vi.useFakeTimers();
    const { onIsland } = setup();
    for (let i = 1; i <= 10; i++) toast(`Error ${i}`, 'danger');
    await tick();
    vi.advanceTimersByTime(URGENT_MS * 9);
    await tick();
    expect(onIsland()).toContain('Error 10');
    vi.advanceTimersByTime(URGENT_MS);
    await tick();
    expect(get(toasts)).toEqual([]);
  });
});

describe('what waits behind the message on screen', () => {
  it('is counted on the pill, said with its name, and "Dismiss all" empties the island and gives focus back', async () => {
    const { keys, pill, island } = setup();
    for (let i = 1; i <= 10; i++) toast(`Error ${i}`, 'danger');
    await tick();
    expect(pill().querySelector('[data-part="count-badge"]')?.textContent).toContain('9');
    expect(screen.getByRole('button', { name: `Error 1 ${msg('ui.liveIslandWaiting', { count: 9 })}` })).toBe(pill());

    const page = screen.getByRole('button', { name: 'Page' });
    page.focus();
    await keys.tab();
    expect(document.activeElement).toBe(pill());
    await keys.keyboard('{Enter}');
    await tick();
    await keys.click(within(island() as HTMLElement).getByRole('button', { name: msg('ui.liveIslandDismissAll') }));
    await tick();
    await tick();

    expect(get(toasts)).toEqual([]);
    expect(get(liveIslandCurrent)).toBeUndefined();
    expect(island()?.dataset.phase).toBe('hidden');
    expect(document.activeElement).toBe(page);
  });

  it('has no count and no "Dismiss all" when nothing waits', async () => {
    const { keys, pill, island } = setup();
    toast('Saved');
    await tick();
    expect(pill().querySelector('[data-part="count-badge"]')).toBeNull();
    await keys.click(pill());
    await tick();
    expect(within(island() as HTMLElement).queryByRole('button', { name: msg('ui.liveIslandDismissAll') })).toBeNull();
  });
});

describe('with a dialog open', () => {
  it('the island is outside the inert app, above the dialog, its action can be pressed, and the dialog stays', async () => {
    const { keys, island, pill } = setup({ dialog: true });
    await tick();
    const run = vi.fn();
    toast('Install failed', 'danger', { label: 'Retry', run });
    await tick();
    expect(app.querySelector('.shell')?.closest('[inert]')).not.toBeNull();
    expect(island()?.closest('[inert]')).toBeNull();
    await keys.click(pill());
    await tick();
    await keys.click(within(island() as HTMLElement).getByRole('button', { name: 'Retry' }));
    expect(run).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('dialog', { name: 'Install' })).toBeTruthy();
  });

  it('a toast with an action waits the dialog out and does not hold up what the dialog raises after it', async () => {
    const { onIsland } = setup({ dialog: true });
    await tick();
    toast('Removed', 'info', { label: 'Undo', run: () => {} });
    toast('Saved');
    await tick();
    expect(onIsland()).toContain('Saved');
  });

  it('asks the dialog for no room: no toast stack stands in its corner', async () => {
    // A dialog that takes both corners and leaves no gutter, the boxes AppShell.dom.test.ts gives: with
    // the stack in use it would be asked for room.
    const boxes: [string, DOMRect][] = [
      ['.toast-region', new DOMRect(876, 48, 380, 728)],
      ['.toasts', new DOMRect(876, 716, 380, 60)],
      ['[data-part="dialog"]', new DOMRect(40, 56, 1200, 734)],
      ['.overlay', new DOMRect(0, 0, 1280, 800)],
    ];
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
      return boxes.find(([selector]) => this.matches(selector))?.[1] ?? new DOMRect(0, 0, 0, 0);
    });
    setup({ dialog: true });
    await tick();
    toast('Saved');
    await tick();
    const root = screen.getByRole('dialog', { name: 'Install' }).parentElement as HTMLElement;
    expect(root.style.getPropertyValue('--toast-room')).toBe('');
    expect(app.querySelectorAll('[data-part="toast"]')).toHaveLength(0);
  });
});
