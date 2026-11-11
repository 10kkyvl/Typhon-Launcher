import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { user } from '../testing/dom';
import ListboxHarness from '../testing/ListboxHarness.svelte';
import { focusableIn, initialFocus, nextFocusTarget } from './focus';
import { dismissTopLayer, openLayer as register, type LayerOptions } from './index';

let app: HTMLDivElement;
const open: (() => void)[] = [];

// The stack is per document and outlives a test: every layer a test opens is closed after it.
function openLayer(element: HTMLElement, options: LayerOptions): () => void {
  const close = register(element, options);
  open.push(close);
  return close;
}

function dialog(html: string): HTMLDivElement {
  const node = document.createElement('div');
  node.innerHTML = html;
  document.body.append(node);
  return node;
}

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  app.innerHTML = `
    <header data-layer-keep><button id="close-window">x</button></header>
    <main id="page"><button id="opener">open</button><button id="other">other</button></main>
    <aside id="already-inert" inert></aside>`;
  document.body.append(app);
});

afterEach(() => {
  for (const close of open.splice(0).reverse()) close();
  document.body.replaceChildren();
});

const byId = (id: string) => document.getElementById(id) as HTMLElement;

describe('layer stack and the app content', () => {
  it('inerts the app content, not the window chrome, until the last of nested layers closes, and restores it exactly', () => {
    const closeOuter = openLayer(dialog('<button>a</button>'), { dismiss: vi.fn() });
    expect(byId('page').hasAttribute('inert')).toBe(true);
    expect(app.querySelector('header')?.hasAttribute('inert')).toBe(false);

    const outer = document.body.lastElementChild as HTMLElement;
    const closeInner = openLayer(dialog('<button>b</button>'), { dismiss: vi.fn() });
    expect(outer.hasAttribute('inert')).toBe(true);

    closeInner();
    expect(outer.hasAttribute('inert')).toBe(false);
    expect(byId('page').hasAttribute('inert')).toBe(true);

    closeOuter();
    expect(byId('page').hasAttribute('inert')).toBe(false);
    expect(byId('already-inert').hasAttribute('inert')).toBe(true);
  });

  it('inerts app content mounted while a dialog is open (a locale remount, Big Picture replacing the shell)', async () => {
    const close = openLayer(dialog('<button>a</button>'), { dismiss: vi.fn() });
    const remounted = document.createElement('div');
    app.append(remounted);
    await Promise.resolve();

    expect(remounted.hasAttribute('inert')).toBe(true);
    close();
    expect(remounted.hasAttribute('inert')).toBe(false);
  });

  it('a non-modal layer inerts nothing and takes no focus', () => {
    byId('opener').focus();
    const close = openLayer(dialog('<button>item</button>'), { modal: false, dismiss: vi.fn() });
    expect(byId('page').hasAttribute('inert')).toBe(false);
    expect(document.activeElement).toBe(byId('opener'));
    close();
  });
});

describe('layer stack keyboard', () => {
  it('Escape outside every layer closes the top one only and never reaches the page', async () => {
    const lower = vi.fn();
    const upper = vi.fn();
    const page = vi.fn();
    window.addEventListener('keydown', page);
    openLayer(dialog('<button>a</button>'), { dismiss: lower });
    openLayer(dialog('<button>b</button>'), { dismiss: upper });
    (document.activeElement as HTMLElement).blur();

    await user().keyboard('{Escape}');

    expect(upper).toHaveBeenCalledTimes(1);
    expect(lower).not.toHaveBeenCalled();
    expect(page).not.toHaveBeenCalled();
    window.removeEventListener('keydown', page);
  });

  it('an element inside the layer can claim Escape first', async () => {
    const dismiss = vi.fn();
    const node = dialog('<input id="field" />');
    byId('field').addEventListener('keydown', (event) => event.key === 'Escape' && event.preventDefault());
    openLayer(node, { dismiss });

    await user().keyboard('{Escape}');

    expect(dismiss).not.toHaveBeenCalled();
  });

  it('moves focus in, keeps Tab inside, and returns focus to the opener on close', async () => {
    byId('opener').focus();
    const node = dialog('<button id="a" data-skip-autofocus>close</button><button id="b">b</button><button id="c">c</button>');
    const close = openLayer(node, { dismiss: vi.fn() });
    expect(document.activeElement?.id).toBe('b');

    const keys = user();
    await keys.tab();
    expect(document.activeElement?.id).toBe('c');
    await keys.tab();
    expect(document.activeElement?.id).toBe('a');
    await keys.tab({ shift: true });
    expect(document.activeElement?.id).toBe('c');

    close();
    expect(document.activeElement).toBe(byId('opener'));
  });

  it('dismissing returns focus before the caller runs, and to the fallback when the opener is gone', () => {
    byId('opener').focus();
    const seen: (Element | null)[] = [];
    openLayer(dialog('<button>ok</button>'), { dismiss: () => seen.push(document.activeElement), returnFocus: () => byId('other') });
    byId('opener').remove();

    expect(dismissTopLayer()).toBe(true);

    expect(seen).toEqual([byId('other')]);
    expect(dismissTopLayer()).toBe(false);
  });
});

describe('focus helpers', () => {
  it('nextFocusTarget wraps both ways and enters at the edge', () => {
    const list = ['a', 'b', 'c'];
    expect(nextFocusTarget(list, 'c', false)).toBe('a');
    expect(nextFocusTarget(list, 'a', true)).toBe('c');
    expect(nextFocusTarget(list, null, false)).toBe('a');
    expect(nextFocusTarget(list, null, true)).toBe('c');
    expect(nextFocusTarget([], null, false)).toBeNull();
  });

  it('focusableIn lists tabbable elements in DOM order and skips disabled, hidden, inert and negative tabindex', () => {
    const root = dialog(`
      <button id="one">1</button>
      <button disabled>x</button>
      <a>no href</a>
      <a id="two" href="#">2</a>
      <span tabindex="-1">x</span>
      <div hidden><button>x</button></div>
      <div inert><button>x</button></div>
      <input id="three" />
      <input type="hidden" />
      <div id="four" tabindex="0"></div>
      <input type="radio" name="r" id="r1" />
      <input type="radio" name="r" id="r2" checked />`);
    expect(focusableIn(root).map((el) => el.id)).toEqual(['one', 'two', 'three', 'four', 'r2']);
  });

  it('initialFocus prefers the caller, then [data-autofocus] or the first control inside it, then the first control', () => {
    const root = dialog('<span data-skip-autofocus><button id="x">x</button></span><input id="field" /><span data-autofocus><button id="safe">safe</button></span>');
    expect(initialFocus(root).id).toBe('safe');
    expect(initialFocus(root, '#field').id).toBe('field');
    expect(initialFocus(root, byId('x')).id).toBe('x');
    byId('safe').parentElement?.removeAttribute('data-autofocus');
    expect(initialFocus(root).id).toBe('field');
  });
});

describe('a listbox portalled out of a dialog, owned by it', () => {
  async function select() {
    app.replaceChildren();
    render(ListboxHarness, { target: app });
    const keys = user();
    const opener = screen.getByRole('button', { name: 'Open settings' });
    await keys.click(opener);
    const dialog = screen.getByRole('dialog', { name: 'Settings' });
    const trigger = screen.getByRole('button', { name: 'Russian' });
    await keys.click(trigger);
    const listbox = screen.getByRole('listbox', { name: 'Language' });
    return { keys, opener, dialog, trigger, listbox };
  }

  it('opens on the press that opened it and takes focus; the dialog stays live under it', async () => {
    const { dialog, listbox } = await select();
    expect(dialog.contains(listbox)).toBe(false);
    expect(listbox.contains(document.activeElement)).toBe(true);
    expect(dialog.closest('[inert]')).toBeNull();
  });

  it('Tab stays within the dialog and the listbox, the options following their trigger', async () => {
    const { keys, dialog, trigger, listbox } = await select();
    const seen: Element[] = [];
    for (let step = 0; step < 14; step += 1) {
      await keys.tab();
      const active = document.activeElement as Element;
      expect(dialog.contains(active) || listbox.contains(active), active.outerHTML).toBe(true);
      seen.push(active);
    }
    const options = screen.getAllByRole('option');
    expect(seen[seen.indexOf(trigger) + 1]).toBe(options[0]);
  });

  it('Escape closes the listbox first, focus back on its trigger, then the dialog, focus back on its opener', async () => {
    const { keys, opener, trigger } = await select();

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
    expect(document.activeElement).toBe(trigger);

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it('a press on the dialog body closes the listbox only', async () => {
    const { keys } = await select();

    await keys.click(screen.getByText('Language', { selector: 'p' }));

    expect(screen.queryByRole('listbox')).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
  });

  it('holding Escape closes one layer, not the listbox and then the dialog', async () => {
    const { keys } = await select();

    await keys.keyboard('{Escape>4}');
    await keys.keyboard('{/Escape}');

    expect(screen.queryByRole('listbox')).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
  });
});

describe('presses around a non-modal layer', () => {
  async function pageSelect(inDialog: boolean) {
    app.replaceChildren();
    render(ListboxHarness, { target: app, props: { inDialog } });
    const keys = user();
    if (inDialog) await keys.click(screen.getByRole('button', { name: 'Open settings' }));
    const trigger = screen.getByRole('button', { name: 'Russian' });
    await keys.click(trigger);
    expect(screen.getByRole('listbox', { name: 'Language' })).toBeTruthy();
    return { keys, trigger };
  }

  it('a press on an icon (an <svg> inside a button) outside the listbox closes it', async () => {
    const { keys } = await pageSelect(true);
    const icon = screen.getByRole('button', { name: 'Help' }).querySelector('path') as Element;

    await keys.pointer({ keys: '[MouseLeft]', target: icon });

    expect(screen.queryByRole('listbox', { name: 'Language' })).toBeNull();
  });
});
