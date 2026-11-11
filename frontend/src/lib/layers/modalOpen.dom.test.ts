import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { modalOpen, openLayer } from './index';

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
