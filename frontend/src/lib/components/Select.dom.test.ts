import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { user } from '../testing/dom';
import SelectHarness from '../testing/SelectHarness.svelte';
import Modal from './Modal.svelte';
import type { SelectOption } from './select';

const LANGUAGES: SelectOption[] = [
  { id: 'system', label: 'System' },
  { id: 'ru', label: 'Russian' },
  { id: 'en', label: 'English' },
  { id: 'de', label: 'Deutsch', disabled: true },
  { id: 'es', label: 'Español' },
  { id: 'eo', label: 'Esperanto' },
];

const MANY: SelectOption[] = Array.from({ length: 30 }, (_, index) => ({ id: `o${index}`, label: `Option ${index}` }));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

type Props = {
  options?: SelectOption[];
  value?: string;
  controlled?: boolean;
  accept?: boolean;
  inDialog?: boolean;
  placeholder?: string;
  disabled?: boolean;
  onchange?: (id: string) => void;
};

function setup(props: Props = {}) {
  const onchange = props.onchange ?? vi.fn();
  render(SelectHarness, { target: app, props: { options: LANGUAGES, value: 'ru', ...props, onchange } });
  const keys = user();
  const field = () => screen.getByRole('combobox', { name: 'Language' });
  const list = () => screen.queryByRole('listbox');
  const option = (name: string) => screen.getByRole('option', { name });
  const active = () => {
    const id = field().getAttribute('aria-activedescendant');
    return id ? document.getElementById(id) : null;
  };
  const held = () => document.querySelector('output')?.textContent ?? '';
  return { keys, onchange, field, list, option, active, held };
}

function shown(field: HTMLElement): string {
  return field.textContent?.trim() ?? '';
}

describe('Select: the field', () => {
  it('is a combobox named by its label that shows the chosen option and says its list is closed', () => {
    const { field } = setup();
    expect(field().getAttribute('aria-haspopup')).toBe('listbox');
    expect(field().getAttribute('aria-expanded')).toBe('false');
    expect(shown(field())).toBe('Russian');
  });

  it('a press opens the list out of the page, named like the field, on the chosen option; focus stays on the field', async () => {
    const { keys, field, list, option, active } = setup();

    await keys.click(field());

    const opened = screen.getByRole('listbox', { name: 'Language' });
    expect(list()).toBe(opened);
    expect(app.contains(opened)).toBe(false);
    expect(field().getAttribute('aria-expanded')).toBe('true');
    expect(field().getAttribute('aria-controls')).toBe(opened.id);
    expect(document.activeElement).toBe(field());
    expect(active()).toBe(option('Russian'));
    expect(screen.getAllByRole('option').map((el) => [el.textContent?.trim(), el.getAttribute('aria-selected')])).toEqual([
      ['System', 'false'],
      ['Russian', 'true'],
      ['English', 'false'],
      ['Deutsch', 'false'],
      ['Español', 'false'],
      ['Esperanto', 'false'],
    ]);
  });

  it('announces a disabled option as disabled', async () => {
    const { keys, field, option } = setup();
    await keys.click(field());
    expect(option('Deutsch').getAttribute('aria-disabled')).toBe('true');
    expect(option('English').hasAttribute('aria-disabled')).toBe(false);
  });

  it('a disabled field does not open', async () => {
    const { keys, field, list } = setup({ disabled: true });
    await keys.click(field());
    field().focus();
    await keys.keyboard('{ArrowDown}');
    expect(list()).toBeNull();
  });
});

describe('Select: keys with the list closed', () => {
  it.each([
    ['ArrowDown', '{ArrowDown}'],
    ['ArrowUp', '{ArrowUp}'],
    ['Enter', '{Enter}'],
    ['Space', ' '],
  ])('%s opens the list on the chosen option and chooses nothing', async (_key, press) => {
    const { keys, onchange, field, list, option, active, held } = setup();
    field().focus();

    await keys.keyboard(press);

    expect(list()).not.toBeNull();
    expect(active()).toBe(option('Russian'));
    expect(document.activeElement).toBe(field());
    expect(onchange).not.toHaveBeenCalled();
    expect(held()).toBe('ru');
  });

  it.each([
    ['Home', '{Home}', 'System'],
    ['End', '{End}', 'Esperanto'],
  ])('%s opens the list on the %s option', async (_key, press, name) => {
    const { keys, field, option, active } = setup();
    field().focus();
    await keys.keyboard(press);
    expect(active()).toBe(option(name));
  });

  it('a typed letter opens the list on the next option starting with it', async () => {
    const { keys, field, list, option, active, held } = setup();
    field().focus();

    await keys.keyboard('e');

    expect(list()).not.toBeNull();
    expect(active()).toBe(option('English'));
    expect(held()).toBe('ru');
  });

  it('Enter held on the field opens the list once and chooses nothing', async () => {
    const { keys, onchange, field, list, option, active } = setup({ controlled: true });
    field().focus();

    await keys.keyboard('{Enter>6}');
    await keys.keyboard('{/Enter}');

    expect(list()).not.toBeNull();
    expect(active()).toBe(option('Russian'));
    expect(onchange).not.toHaveBeenCalled();
  });

  it('Space held on the field opens the list once; releasing it chooses nothing and leaves it open', async () => {
    const { keys, onchange, field, list } = setup({ controlled: true });
    field().focus();

    await keys.keyboard('[Space>6]');
    await keys.keyboard('[/Space]');

    expect(list()).not.toBeNull();
    expect(onchange).not.toHaveBeenCalled();
  });

  it('a keyboard open on a field scrolled out of sight brings it back, so the list opens', async () => {
    const { keys, field, list } = setup();
    let top = -60;
    field().getBoundingClientRect = () => DOMRect.fromRect({ x: 200, y: top, width: 200, height: 32 });
    const reveal = vi.fn(() => {
      top = 300;
    });
    field().scrollIntoView = reveal;
    field().focus();

    await keys.keyboard('{ArrowDown}');

    expect(reveal).toHaveBeenCalledExactlyOnceWith({ block: 'nearest' });
    expect(list()).not.toBeNull();
  });
});

describe('Select: keys with the list open', () => {
  it('arrows move without wrapping and skip a disabled option; Home and End jump; focus stays on the field', async () => {
    const { keys, field, option, active } = setup();
    field().focus();
    await keys.keyboard('{ArrowDown}');
    expect(active()).toBe(option('Russian'));

    await keys.keyboard('{ArrowDown}');
    expect(active()).toBe(option('English'));
    await keys.keyboard('{ArrowDown}');
    expect(active()).toBe(option('Español'));
    await keys.keyboard('{End}{ArrowDown}');
    expect(active()).toBe(option('Esperanto'));
    await keys.keyboard('{Home}{ArrowUp}');
    expect(active()).toBe(option('System'));
    expect(document.activeElement).toBe(field());
  });

  it('PageDown and PageUp move by ten and stop at the ends', async () => {
    const { keys, field, option, active } = setup({ options: MANY, value: 'o3' });
    field().focus();
    await keys.keyboard('{ArrowDown}');

    await keys.keyboard('{PageDown}');
    expect(active()).toBe(option('Option 13'));
    await keys.keyboard('{PageDown}{PageDown}');
    expect(active()).toBe(option('Option 29'));
    await keys.keyboard('{PageUp}');
    expect(active()).toBe(option('Option 19'));
    await keys.keyboard('{PageUp}{PageUp}');
    expect(active()).toBe(option('Option 0'));
  });

  it('typing moves to the next option starting with it and cycles on a repeated letter', async () => {
    const { keys, field, option, active } = setup();
    field().focus();
    await keys.keyboard('{ArrowDown}');

    await keys.keyboard('e');
    expect(active()).toBe(option('English'));
    await keys.keyboard('e');
    expect(active()).toBe(option('Español'));
    await keys.keyboard('e');
    expect(active()).toBe(option('Esperanto'));
  });

  it.each([
    ['Enter', '{Enter}'],
    ['Space', ' '],
    ['Alt+ArrowUp', '{Alt>}{ArrowUp}{/Alt}'],
  ])('%s chooses the active option and closes the list; focus stays on the field', async (_key, press) => {
    const { keys, field, list, held } = setup();
    field().focus();
    await keys.keyboard('{ArrowDown}{ArrowDown}');

    await keys.keyboard(press);

    expect(list()).toBeNull();
    expect(field().getAttribute('aria-expanded')).toBe('false');
    expect(field().hasAttribute('aria-activedescendant')).toBe(false);
    expect(held()).toBe('en');
    expect(shown(field())).toBe('English');
    expect(document.activeElement).toBe(field());
  });

  it('Tab chooses the active option, closes the list and moves on', async () => {
    const { keys, field, list, held } = setup();
    field().focus();
    await keys.keyboard('{ArrowDown}{ArrowDown}');

    await keys.tab();

    expect(list()).toBeNull();
    expect(held()).toBe('en');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
  });

  it('Shift+Tab chooses the active option, closes the list and moves back', async () => {
    const { keys, field, list, held } = setup();
    field().focus();
    await keys.keyboard('{ArrowDown}{ArrowDown}');

    await keys.tab({ shift: true });

    expect(list()).toBeNull();
    expect(held()).toBe('en');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Before' }));
  });

  it('Escape closes the list and keeps the old value; focus stays on the field', async () => {
    const { keys, onchange, field, list, held } = setup({ controlled: true });
    field().focus();
    await keys.keyboard('{ArrowDown}{ArrowDown}');

    await keys.keyboard('{Escape}');

    expect(list()).toBeNull();
    expect(onchange).not.toHaveBeenCalled();
    expect(held()).toBe('ru');
    expect(shown(field())).toBe('Russian');
    expect(document.activeElement).toBe(field());
  });

  it.each([
    ['Enter', '{Enter>6}', '{/Enter}'],
    ['Space', '[Space>6]', '[/Space]'],
  ])('%s held on the open list chooses once and does not reopen it', async (_key, hold, release) => {
    const { keys, onchange, field, list } = setup({ controlled: true });
    field().focus();
    await keys.keyboard('{ArrowDown}{ArrowDown}');

    await keys.keyboard(hold);
    await keys.keyboard(release);

    expect(onchange).toHaveBeenCalledExactlyOnceWith('en');
    expect(list()).toBeNull();
    expect(document.activeElement).toBe(field());
  });
});

describe('Select: pointer', () => {
  it('a press on an option chooses it, closes the list and leaves focus on the field', async () => {
    const { keys, field, list, option, held } = setup();
    await keys.click(field());

    await keys.click(option('English'));

    expect(list()).toBeNull();
    expect(held()).toBe('en');
    expect(shown(field())).toBe('English');
    expect(document.activeElement).toBe(field());
  });

  it('a press on a disabled option chooses nothing and keeps the list open', async () => {
    const { keys, onchange, field, list, option } = setup({ controlled: true });
    await keys.click(field());

    await keys.click(option('Deutsch'));

    expect(onchange).not.toHaveBeenCalled();
    expect(list()).not.toBeNull();
  });

  it('the option under the pointer becomes the active one', async () => {
    const { keys, field, option, active } = setup();
    await keys.click(field());

    await keys.hover(option('Español'));

    expect(active()).toBe(option('Español'));
  });

  it('a press outside closes the list without choosing; focus goes where the press put it', async () => {
    const { keys, onchange, field, list, held } = setup({ controlled: true });
    await keys.click(field());
    const before = screen.getByRole('button', { name: 'Before' });

    await keys.click(before);

    expect(list()).toBeNull();
    expect(onchange).not.toHaveBeenCalled();
    expect(held()).toBe('ru');
    expect(document.activeElement).toBe(before);
  });

  it('a press on the field closes the list and it stays closed', async () => {
    const { keys, field, list } = setup();
    await keys.click(field());

    await keys.click(field());

    expect(list()).toBeNull();
    expect(field().getAttribute('aria-expanded')).toBe('false');
  });

  it('a press on an icon (an <svg> inside a button) outside closes the list', async () => {
    const { keys, field, list } = setup();
    await keys.click(field());
    const icon = screen.getByRole('button', { name: 'Help' }).querySelector('path') as Element;

    await keys.pointer({ keys: '[MouseLeft]', target: icon });

    expect(list()).toBeNull();
  });
});

describe('Select: values', () => {
  it('a value the list does not hold shows as it is, marks no option, and choosing replaces it', async () => {
    const { keys, field, option, held } = setup({ value: 'pt' });
    expect(shown(field())).toBe('pt');

    await keys.click(field());
    expect(screen.getAllByRole('option').filter((el) => el.getAttribute('aria-selected') === 'true')).toEqual([]);
    await keys.click(option('English'));

    expect(held()).toBe('en');
    expect(shown(field())).toBe('English');
  });

  it('an empty value shows the placeholder the caller gives, and nothing without one', () => {
    setup({ value: '', placeholder: 'Pick a language' });
    expect(shown(screen.getByRole('combobox', { name: 'Language' }))).toBe('Pick a language');
    document.body.replaceChildren();
    app = document.createElement('div');
    app.id = 'app';
    document.body.append(app);

    setup({ value: '' });
    expect(shown(screen.getByRole('combobox', { name: 'Language' }))).toBe('');
  });

  it('controlled: a change the caller refuses leaves the field on the value the caller holds', async () => {
    const { keys, onchange, field, option, held } = setup({ controlled: true, accept: false });
    await keys.click(field());

    await keys.click(option('English'));

    expect(onchange).toHaveBeenCalledExactlyOnceWith('en');
    expect(held()).toBe('ru');
    expect(shown(field())).toBe('Russian');
    await keys.click(field());
    expect(option('Russian').getAttribute('aria-selected')).toBe('true');
  });

  it('controlled: a change the caller takes shows on the field', async () => {
    const { keys, onchange, field, option } = setup({ controlled: true });
    await keys.click(field());

    await keys.click(option('English'));

    expect(onchange).toHaveBeenCalledExactlyOnceWith('en');
    expect(shown(field())).toBe('English');
  });

  it('choosing the option already chosen still reports it (a choice such as "Other folder" acts each time)', async () => {
    const { keys, onchange, field, option } = setup({ controlled: true });
    await keys.click(field());

    await keys.click(option('Russian'));

    expect(onchange).toHaveBeenCalledExactlyOnceWith('ru');
  });
});

describe('Select: where the list goes', () => {
  it('opens fixed under the field, at least as wide as it', async () => {
    const { keys, field, list } = setup();
    field().getBoundingClientRect = () => DOMRect.fromRect({ x: 100, y: 200, width: 240, height: 32 });

    await keys.click(field());

    expect(list()?.style.position).toBe('fixed');
    expect(list()?.dataset.placement).toBe('bottom-start');
    expect(list()?.style.minWidth).toBe('240px');
  });

  // jsdom has no layout: the field's and the list's boxes, the options' offsets and the list's
  // visible height are stubbed.
  it('a long list is held to the room in the window and scrolls itself to the active option', async () => {
    const OPTION = 32;
    const VISIBLE = 320;
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.getAttribute('role') === 'listbox') return DOMRect.fromRect({ x: 0, y: 0, width: 240, height: MANY.length * OPTION });
      if (this.getAttribute('role') === 'combobox') return DOMRect.fromRect({ x: 100, y: 300, width: 240, height: 32 });
      return DOMRect.fromRect({ x: 0, y: 0, width: 0, height: 0 });
    });
    const indexOf = (el: HTMLElement) => [...(el.parentElement?.children ?? [])].indexOf(el);
    vi.spyOn(HTMLElement.prototype, 'offsetTop', 'get').mockImplementation(function (this: HTMLElement) {
      return this.getAttribute('role') === 'option' ? indexOf(this) * OPTION : 0;
    });
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockImplementation(function (this: HTMLElement) {
      return this.getAttribute('role') === 'option' ? OPTION : 0;
    });
    vi.spyOn(Element.prototype, 'clientHeight', 'get').mockImplementation(function (this: Element) {
      return this.getAttribute('role') === 'listbox' ? VISIBLE : 0;
    });
    const { keys, field, list } = setup({ options: MANY, value: 'o25' });

    field().focus();
    await keys.keyboard('{ArrowDown}');

    const opened = list() as HTMLElement;
    expect(opened.style.maxHeight).not.toBe('');
    expect(parseFloat(opened.style.maxHeight)).toBeLessThan(MANY.length * OPTION);
    expect(opened.scrollTop).toBe(26 * OPTION - VISIBLE);

    await keys.keyboard('{Home}');
    expect(opened.scrollTop).toBe(0);
    await keys.keyboard('{PageDown}{PageDown}');
    expect(opened.scrollTop).toBe(21 * OPTION - VISIBLE);
  });
});

describe('Select inside a dialog', () => {
  async function inDialog(props: Props = {}) {
    const view = setup({ inDialog: true, ...props });
    await view.keys.click(screen.getByRole('button', { name: 'Open settings' }));
    const dialog = screen.getByRole('dialog', { name: 'Settings' });
    await view.keys.click(view.field());
    expect(view.list()).not.toBeNull();
    return { ...view, dialog };
  }

  it('the list is not inside the dialog, so its scrolling body cannot cut it', async () => {
    const { dialog, list } = await inDialog();
    expect(dialog.contains(list())).toBe(false);
    expect(list()?.style.position).toBe('fixed');
  });

  it('Escape closes the list first, focus on the field, then the dialog', async () => {
    const { keys, field, list } = await inDialog();

    await keys.keyboard('{Escape}');
    expect(list()).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
    expect(document.activeElement).toBe(field());

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('one press on the scrim closes the list only; the next one closes the dialog', async () => {
    const { keys, dialog, list } = await inDialog();
    const scrim = dialog.parentElement as HTMLElement;

    await keys.click(scrim);
    expect(list()).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();

    await keys.click(scrim);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('Tab chooses the active option and moves on inside the dialog', async () => {
    const { keys, dialog, list, held } = await inDialog();
    await keys.keyboard('{ArrowDown}');

    await keys.tab();

    expect(list()).toBeNull();
    expect(held()).toBe('en');
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
  });

  it('a press on an option chooses it and the dialog stays', async () => {
    const { keys, field, option, list, held } = await inDialog();

    await keys.click(option('English'));

    expect(list()).toBeNull();
    expect(held()).toBe('en');
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
    expect(document.activeElement).toBe(field());
  });
});

describe('Select and other layers', () => {
  it('a dialog that opens while the list is open closes the list', async () => {
    const { keys, field, list } = setup();
    await keys.click(field());

    render(Modal, { target: app, props: { open: true, title: 'The game closed' } });
    await tick();

    expect(list()).toBeNull();
    expect(field().getAttribute('aria-expanded')).toBe('false');
  });
});
