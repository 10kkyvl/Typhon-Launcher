import { afterEach, describe, expect, it, vi } from 'vitest';
import { openLayer } from '../layers';
import { layerCommand, layerTakesInput } from './layers';
import shellSource from '../components/BigPictureShell.svelte?raw';

const closers: (() => void)[] = [];

function layerOf(html: string, modal: boolean): HTMLElement {
  const node = document.createElement('div');
  node.innerHTML = html;
  document.body.append(node);
  closers.push(openLayer(node, { modal, dismiss: () => {} }));
  return node;
}

afterEach(() => {
  for (const close of closers.splice(0).reverse()) close();
  document.body.replaceChildren();
});

describe('Big Picture commands and non-modal layers', () => {
  it('a tooltip alone (nothing to focus) leaves the commands to the shell, and is not made focusable', () => {
    const tip = layerOf('<span>Hint</span>', false);

    expect(layerCommand('down')).toBe(false);
    expect(layerCommand('confirm')).toBe(false);
    expect(tip.hasAttribute('tabindex')).toBe(false);
  });

  it('with a dialog under a tooltip, the directions move between the dialog buttons, never to the tooltip', () => {
    const dialog = layerOf('<button id="a">A</button><button id="b">B</button>', true);
    layerOf('<span>Hint</span>', false);
    expect(document.activeElement?.id).toBe('a');

    expect(layerCommand('down')).toBe(true);
    expect(document.activeElement?.id).toBe('b');
    expect(layerCommand('down')).toBe(true);
    expect(dialog.contains(document.activeElement)).toBe(true);
  });
});

describe('one rule for when a layer takes Big Picture input', () => {
  it('a tooltip alone does not; a dialog or a layer with something to focus does', () => {
    expect(layerTakesInput()).toBe(false);
    layerOf('<span>Hint</span>', false);
    expect(layerTakesInput()).toBe(false);
    layerOf('<button>Pick</button>', false);
    expect(layerTakesInput()).toBe(true);
  });

  it("the shell's Tab trap and focus recovery stand aside on that same rule, not on any layer", () => {
    expect(shellSource).not.toMatch(/\btopLayer\(\)/);
    expect(shellSource.match(/\blayerTakesInput\(\)/g)).toHaveLength(2);
  });
});

describe('confirm inside layers a dialog owns', () => {
  it('presses the focused option of a listbox that sits between the dialog and its submenu', () => {
    const dialog = layerOf('<button id="trigger">Language</button>', true);
    const trigger = dialog.querySelector('#trigger') as HTMLElement;
    const listbox = document.createElement('div');
    listbox.innerHTML = '<button id="english">English</button><button id="more">More</button>';
    document.body.append(listbox);
    closers.push(openLayer(listbox, { modal: false, owner: () => trigger, dismiss: () => {} }));
    const more = listbox.querySelector('#more') as HTMLElement;
    const submenu = document.createElement('div');
    submenu.innerHTML = '<button>Deutsch</button>';
    document.body.append(submenu);
    closers.push(openLayer(submenu, { modal: false, owner: () => more, dismiss: () => {} }));
    const english = listbox.querySelector('#english') as HTMLElement;
    const picked = vi.fn();
    english.addEventListener('click', picked);
    english.focus();

    expect(layerCommand('confirm')).toBe(true);

    expect(picked).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(english);
  });
});
