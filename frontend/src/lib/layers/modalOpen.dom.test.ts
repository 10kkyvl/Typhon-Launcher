import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { modalOpen, openLayer, topModal } from './index';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

function box(): HTMLDivElement {
  const element = document.createElement('div');
  document.body.append(element);
  return element;
}

describe('modalOpen', () => {
  it('is true exactly while a dialog is open, however many', () => {
    expect(get(modalOpen)).toBe(false);

    const first = openLayer(box(), { dismiss: () => {} });
    const second = openLayer(box(), { dismiss: () => {} });
    expect(get(modalOpen)).toBe(true);

    second();
    expect(get(modalOpen)).toBe(true);
    first();
    expect(get(modalOpen)).toBe(false);
  });

  it('a menu or a hint alone does not count', () => {
    const menu = openLayer(box(), { modal: false, dismiss: () => {} });

    expect(get(modalOpen)).toBe(false);
    menu();
  });
});

describe('topModal', () => {
  it('is the dialog on top: the second while it is open, the first again after, none at the end', () => {
    const [one, two] = [box(), box()];
    expect(get(topModal)).toBeNull();

    const first = openLayer(one, { dismiss: () => {} });
    expect(get(topModal)).toBe(one);
    const second = openLayer(two, { dismiss: () => {} });
    expect(get(topModal)).toBe(two);

    second();
    expect(get(topModal)).toBe(one);
    first();
    expect(get(topModal)).toBeNull();
  });

  it('is told once per change: a menu or a hint opened over the dialog is not news', () => {
    const seen: (HTMLElement | null)[] = [];
    const stop = topModal.subscribe((element) => seen.push(element));
    const dialog = box();

    const close = openLayer(dialog, { dismiss: () => {} });
    const menu = openLayer(box(), { modal: false, dismiss: () => {} });
    menu();
    close();
    stop();

    expect(seen).toEqual([null, dialog, null]);
  });
});
