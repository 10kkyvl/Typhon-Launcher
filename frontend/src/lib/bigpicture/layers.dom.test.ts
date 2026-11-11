import { afterEach, describe, expect, it } from 'vitest';
import { openLayer } from '../layers';
import { layerCommand } from './layers';

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
