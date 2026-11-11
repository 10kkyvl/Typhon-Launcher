import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import { user } from '../testing/dom';
import PopoverHarness from '../testing/PopoverHarness.svelte';
import Modal from './Modal.svelte';
import type { Placement } from '../utils/position';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

function setup(props: { inDialog?: boolean; alone?: boolean; firstGoes?: boolean; empty?: boolean; placement?: Placement; width?: string; partId?: string; onfirst?: () => void } = {}) {
  const onfirst = props.onfirst ?? vi.fn();
  render(PopoverHarness, { target: app, props: { ...props, onfirst } });
  const keys = user();
  const trigger = () => screen.getByRole('button', { name: 'Details' });
  const panel = () => screen.queryByRole('dialog', { name: 'Details' });
  const control = (name: string) => screen.getByRole('button', { name });
  return { keys, onfirst, trigger, panel, control };
}

describe('Popover: opening', () => {
  it('a click opens a non-modal dialog out of the page, named, which the trigger says it opens and controls', async () => {
    const { keys, trigger, panel, control } = setup();
    expect(trigger().getAttribute('aria-haspopup')).toBe('dialog');
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
    expect(trigger().hasAttribute('aria-controls')).toBe(false);

    await keys.click(trigger());

    const opened = panel() as HTMLElement;
    expect(opened).not.toBeNull();
    expect(opened.hasAttribute('aria-modal')).toBe(false);
    expect(trigger().getAttribute('aria-expanded')).toBe('true');
    expect(trigger().getAttribute('aria-controls')).toBe(opened.id);
    expect(app.contains(opened)).toBe(false);
    expect(document.activeElement).toBe(control('First'));
  });

  it.each([
    ['Enter', '{Enter}'],
    ['Space', ' '],
  ])('%s on the trigger opens it with focus on its first control', async (_key, press) => {
    const { keys, trigger, panel, control } = setup();
    trigger().focus();

    await keys.keyboard(press);

    expect(panel()).not.toBeNull();
    expect(document.activeElement).toBe(control('First'));
  });

  it('with no control inside, focus is on the panel itself', async () => {
    const { keys, trigger, panel } = setup({ empty: true });
    trigger().focus();

    await keys.keyboard('{Enter}');

    expect(document.activeElement).toBe(panel());
  });

  it('arrow keys on the trigger open nothing', async () => {
    const { keys, trigger, panel } = setup();
    trigger().focus();

    await keys.keyboard('{ArrowDown}{ArrowUp}');

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('carries the theme part, and the id the caller gives it', async () => {
    const { keys, trigger, panel } = setup({ partId: 'notifications' });
    await keys.click(trigger());
    expect(panel()?.dataset.part).toBe('popover');
    expect(panel()?.dataset.id).toBe('notifications');
  });

  it('is placed fixed against the trigger edge its placement names, at the width asked for', async () => {
    const { keys, trigger, panel } = setup({ placement: 'bottom-end', width: '32rem' });
    const rects = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this === trigger()) return DOMRect.fromRect({ x: 700, y: 40, width: 36, height: 36 });
      if (this.getAttribute('role') === 'dialog') return DOMRect.fromRect({ x: 0, y: 0, width: 320, height: 200 });
      return DOMRect.fromRect();
    });
    try {
      await keys.click(trigger());
      const opened = panel() as HTMLElement;
      expect(opened.style.position).toBe('fixed');
      expect(opened.dataset.placement).toBe('bottom-end');
      expect(opened.style.width).toBe('32rem');
      expect(Number.parseFloat(opened.style.left)).toBe(736 - 320);
      expect(Number.parseFloat(opened.style.top)).toBeGreaterThan(76);
    } finally {
      rects.mockRestore();
    }
  });
});

describe('Popover: closing', () => {
  it('Escape closes it and focus is on the trigger', async () => {
    const { keys, trigger, panel } = setup();
    await keys.click(trigger());

    await keys.keyboard('{Escape}');

    expect(panel()).toBeNull();
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(trigger());
  });

  it('a press outside closes it and focus goes where the press put it', async () => {
    const { keys, trigger, panel, control } = setup();
    await keys.click(trigger());

    await keys.click(control('Before'));

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(control('Before'));
  });

  it('a press on the trigger closes it and it stays closed', async () => {
    const { keys, trigger, panel } = setup();
    await keys.click(trigger());

    await keys.click(trigger());

    expect(panel()).toBeNull();
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
  });

  it('a press on an icon (an <svg> inside a button) outside closes it', async () => {
    const { keys, trigger, panel } = setup();
    await keys.click(trigger());
    const icon = screen.getByRole('button', { name: 'Help' }).querySelector('path') as Element;

    await keys.pointer({ keys: '[MouseLeft]', target: icon });

    expect(panel()).toBeNull();
  });

  it('a press inside it keeps it open', async () => {
    const { keys, onfirst, trigger, panel, control } = setup();
    await keys.click(trigger());

    await keys.click(control('First'));

    expect(onfirst).toHaveBeenCalledOnce();
    expect(panel()).not.toBeNull();
  });

  it('close from its content closes it and puts focus back on the trigger', async () => {
    const { keys, trigger, panel, control } = setup();
    await keys.click(trigger());

    await keys.click(control('Done'));

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('a dialog that opens while it is open closes it first', async () => {
    const { keys, trigger, panel } = setup();
    await keys.click(trigger());
    expect(panel()).not.toBeNull();

    render(Modal, { target: app, props: { open: true, title: 'The game closed' } });

    expect(panel()).toBeNull();
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
  });

  it('closes when its trigger scrolls out of the window', async () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb));
    vi.stubGlobal('cancelAnimationFrame', () => {});
    try {
      const { keys, trigger, panel } = setup();
      let top = 300;
      trigger().getBoundingClientRect = () => DOMRect.fromRect({ x: 200, y: top, width: 80, height: 32 });
      await keys.click(trigger());

      top = 120;
      document.dispatchEvent(new Event('scroll'));
      for (const frame of frames.splice(0)) frame(0);
      expect(panel()).not.toBeNull();

      top = -60;
      document.dispatchEvent(new Event('scroll'));
      for (const frame of frames.splice(0)) frame(0);
      await tick();

      expect(panel()).toBeNull();
      expect(trigger().getAttribute('aria-expanded')).toBe('false');
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

describe('Popover: keyboard inside', () => {
  it('Tab moves through its controls; Tab past the last closes it and focus moves on from the trigger', async () => {
    const { keys, trigger, panel, control } = setup();
    trigger().focus();
    await keys.keyboard('{Enter}');

    await keys.tab();
    expect(document.activeElement).toBe(control('Remove me'));
    await keys.tab();
    expect(document.activeElement).toBe(control('Done'));
    expect(panel()).not.toBeNull();

    await keys.tab();

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(control('After'));
  });

  it('Shift+Tab before the first control closes it and focus is on the trigger', async () => {
    const { keys, trigger, panel } = setup();
    trigger().focus();
    await keys.keyboard('{Enter}');

    await keys.tab({ shift: true });

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('Tab out of a panel with no control closes it and moves on from the trigger', async () => {
    const { keys, trigger, panel, control } = setup({ empty: true });
    trigger().focus();
    await keys.keyboard('{Enter}');

    await keys.tab();

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(control('After'));
  });

  it('a control removed under focus leaves focus in the panel, and Tab still works from there', async () => {
    const { keys, trigger, panel, control } = setup();
    await keys.click(trigger());
    await keys.tab();
    expect(document.activeElement).toBe(control('Remove me'));

    await keys.keyboard('{Enter}');
    await tick();

    expect(screen.queryByRole('button', { name: 'Remove me' })).toBeNull();
    expect(panel()).not.toBeNull();
    expect(panel()?.contains(document.activeElement)).toBe(true);
    await keys.tab();
    expect(panel()?.contains(document.activeElement)).toBe(true);
  });

  it('the control focused on open, removed by a press on it, leaves focus in the panel', async () => {
    const { keys, trigger, panel, control } = setup({ firstGoes: true });
    await keys.click(trigger());
    expect(document.activeElement).toBe(control('First'));

    await keys.click(control('First'));
    await tick();

    expect(screen.queryByRole('button', { name: 'First' })).toBeNull();
    expect(panel()?.contains(document.activeElement)).toBe(true);
  });

  it('Enter and Space on a control inside press it', async () => {
    const { keys, onfirst, trigger } = setup();
    trigger().focus();
    await keys.keyboard('{Enter}');

    await keys.keyboard('{Enter}');
    await keys.keyboard(' ');

    expect(onfirst).toHaveBeenCalledTimes(2);
  });
});

describe('Popover: held keys', () => {
  it('Enter held on the trigger opens it once and presses nothing inside', async () => {
    const { keys, onfirst, trigger, panel, control } = setup();
    trigger().focus();

    await keys.keyboard('{Enter>7}');
    await keys.keyboard('{/Enter}');

    expect(onfirst).not.toHaveBeenCalled();
    expect(panel()).not.toBeNull();
    expect(document.activeElement).toBe(control('First'));
  });

  it('Enter held on a control that closes it closes it once: the trigger does not reopen it', async () => {
    const { keys, trigger, panel, control } = setup();
    await keys.click(trigger());
    control('Done').focus();

    await keys.keyboard('{Enter>7}');
    await keys.keyboard('{/Enter}');

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('Space held on the trigger opens it once; releasing it inside presses nothing', async () => {
    const { keys, onfirst, trigger, panel, control } = setup();
    trigger().focus();

    await keys.keyboard('[Space>7]');
    await keys.keyboard('[/Space]');

    expect(onfirst).not.toHaveBeenCalled();
    expect(panel()).not.toBeNull();
    expect(document.activeElement).toBe(control('First'));
  });
});

describe('Popover inside a dialog', () => {
  async function inDialog(props: { alone?: boolean } = {}) {
    const view = setup({ inDialog: true, ...props });
    await view.keys.click(screen.getByRole('button', { name: 'Open settings' }));
    const dialog = screen.getByRole('dialog', { name: 'Settings' });
    view.trigger().focus();
    await view.keys.keyboard('{Enter}');
    expect(view.panel()).not.toBeNull();
    return { ...view, dialog };
  }

  it('Tab goes through the popover after its trigger and on to the dialog, never out of the two', async () => {
    const { keys, dialog, panel, control } = await inDialog();
    expect(document.activeElement).toBe(control('First'));

    await keys.tab();
    await keys.tab();
    expect(document.activeElement).toBe(control('Done'));

    await keys.tab();

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(control('After'));
    expect(dialog.contains(document.activeElement)).toBe(true);
  });

  it('Tab past the last control, its trigger last in the dialog, wraps to the dialog\'s first control', async () => {
    const { keys, dialog, panel, control } = await inDialog({ alone: true });
    control('Done').focus();

    await keys.tab();

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(within(dialog).getAllByRole('button')[0]);
    expect(dialog.contains(document.activeElement)).toBe(true);
  });

  it('Shift+Tab from the first control closes it onto its trigger, inside the dialog', async () => {
    const { keys, trigger, panel } = await inDialog();

    await keys.tab({ shift: true });

    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('Escape closes the popover first, focus on its trigger, then the dialog', async () => {
    const { keys, trigger, panel } = await inDialog();

    await keys.keyboard('{Escape}');
    expect(panel()).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
    expect(document.activeElement).toBe(trigger());

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('one press on the scrim closes the popover only; the next one closes the dialog', async () => {
    const { keys, dialog, panel } = await inDialog();
    const scrim = dialog.parentElement as HTMLElement;

    await keys.click(scrim);
    expect(panel()).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();

    await keys.click(scrim);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
