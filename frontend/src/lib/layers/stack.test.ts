import { describe, expect, it, vi } from 'vitest';
import { LayerStack, type LayerSpec } from './stack';

// A layer element is a name with the focus targets inside it; a focus target is a name that may be gone.
interface Box {
  name: string;
  inside: Set<string>;
}

function world() {
  const gone = new Set<string>();
  const stack = new LayerStack<Box, string>({
    contains: (element, target) => element.inside.has(target),
    alive: (target) => !gone.has(target),
  });
  const box = (name: string, ...inside: string[]): Box => ({ name, inside: new Set(inside) });
  const open = (element: Box, spec: Partial<LayerSpec<Box, string>> = {}) =>
    stack.open({ element, modal: true, dismissible: () => true, dismiss: vi.fn(), opener: null, ...spec });
  return { stack, gone, box, open };
}

describe('LayerStack order', () => {
  it('puts the last opened layer on top and tells a modal top from a menu above it', () => {
    const { stack, box, open } = world();
    expect(stack.top()).toBeNull();
    const dialog = box('dialog');
    const id = open(dialog);
    expect(stack.top()).toBe(dialog);
    expect(stack.isTop(id)).toBe(true);

    const menu = box('menu');
    const menuId = open(menu, { modal: false });

    expect(stack.top()).toBe(menu);
    expect(stack.isTop(id)).toBe(false);
    expect(stack.isTopModal(id)).toBe(true);
    expect(stack.isTopModal(menuId)).toBe(false);
  });

  it('finds the layer that holds a focus target', () => {
    const { stack, box, open } = world();
    const lower = box('lower', 'a');
    const upper = box('upper', 'b');
    open(lower);
    open(upper);
    expect(stack.holding('a')).toBe(lower);
    expect(stack.holding('b')).toBe(upper);
    expect(stack.holding('page')).toBeNull();
  });
});

describe('LayerStack inert', () => {
  it('inerts the app under a modal layer and every layer under the topmost modal one', () => {
    const { stack, box, open } = world();
    expect(stack.inert()).toEqual({ app: false, layers: [] });
    const first = box('first');
    const firstId = open(first);
    expect(stack.inert()).toEqual({ app: true, layers: [] });
    const second = box('second');
    const secondId = open(second);
    expect(stack.inert()).toEqual({ app: true, layers: [first] });

    stack.close(secondId);
    expect(stack.inert()).toEqual({ app: true, layers: [] });
    stack.close(firstId);
    expect(stack.inert()).toEqual({ app: false, layers: [] });
  });

  it('leaves a dialog live under a menu opened from it, and inerts nothing for a menu alone', () => {
    const { stack, box, open } = world();
    const menuId = open(box('page menu'), { modal: false });
    expect(stack.inert()).toEqual({ app: false, layers: [] });
    stack.close(menuId);

    open(box('dialog'));
    open(box('select'), { modal: false });
    expect(stack.inert()).toEqual({ app: true, layers: [] });
  });
});

describe('LayerStack Escape', () => {
  it('closes the top layer only, once per press', () => {
    const { stack, box, open } = world();
    const lower = vi.fn();
    const upper = vi.fn();
    open(box('lower'), { dismiss: lower });
    open(box('upper'), { dismiss: upper });

    const first = stack.dismissTop();
    expect(first.consumed).toBe(true);
    first.dismiss?.();
    expect(upper).toHaveBeenCalledTimes(1);
    expect(lower).not.toHaveBeenCalled();
    expect(stack.size).toBe(1);

    stack.dismissTop().dismiss?.();
    expect(lower).toHaveBeenCalledTimes(1);
    expect(stack.dismissTop()).toEqual({ consumed: false, closed: null, dismiss: null });
  });

  it('swallows Escape for a layer that may not close and keeps it open', () => {
    const { stack, box, open } = world();
    const dismiss = vi.fn();
    let busy = true;
    open(box('confirm'), { dismiss, dismissible: () => !busy });

    expect(stack.dismissTop()).toEqual({ consumed: true, closed: null, dismiss: null });
    expect(stack.size).toBe(1);

    busy = false;
    stack.dismissTop().dismiss?.();
    expect(dismiss).toHaveBeenCalledTimes(1);
    expect(stack.size).toBe(0);
  });

  it('closes a menu before the dialog it was opened from', () => {
    const { stack, box, open } = world();
    const dialog = vi.fn();
    const menu = vi.fn();
    open(box('dialog', 'trigger'), { dismiss: dialog });
    open(box('menu'), { modal: false, dismiss: menu, opener: 'trigger' });

    const result = stack.dismissTop();
    result.dismiss?.();

    expect(menu).toHaveBeenCalledTimes(1);
    expect(dialog).not.toHaveBeenCalled();
    expect(result.closed).toEqual({ focus: 'trigger', enter: null });
  });
});

describe('LayerStack focus return', () => {
  it('returns focus to the opener of each nested layer in turn', () => {
    const { stack, box, open } = world();
    const outer = open(box('outer', 'inner-opener'), { opener: 'page-button' });
    const inner = open(box('inner'), { opener: 'inner-opener' });

    expect(stack.close(inner)).toEqual({ focus: 'inner-opener', enter: null });
    expect(stack.close(outer)).toEqual({ focus: 'page-button', enter: null });
  });

  it('uses the fallback when the opener is gone, and enters the dialog on top when neither is there', () => {
    const { stack, gone, box, open } = world();
    const outer = box('outer', 'outer-field');
    open(outer, { opener: 'page-button' });
    const inner = open(box('inner'), { opener: 'row-button', returnFocus: () => 'outer-field' });
    gone.add('row-button');

    expect(stack.close(inner)).toEqual({ focus: 'outer-field', enter: null });

    const third = open(box('third'), { opener: 'deleted', returnFocus: () => null });
    gone.add('deleted');
    expect(stack.close(third)).toEqual({ focus: null, enter: outer });
  });

  it('does not send focus behind a dialog that is still open', () => {
    const { stack, box, open } = world();
    const first = box('first');
    open(first, { opener: 'page-button' });
    const second = open(box('second'), { opener: 'another-page-button' });

    expect(stack.close(second)).toEqual({ focus: null, enter: first });
  });

  it('closed out of order: the upper layer inherits the way home of the layer it was opened from', () => {
    const { stack, gone, box, open } = world();
    const lower = open(box('lower', 'lower-button'), { opener: 'page-button' });
    const upper = open(box('upper'), { opener: 'lower-button' });

    expect(stack.close(lower)).toBeNull();
    gone.add('lower-button');
    expect(stack.inert()).toEqual({ app: true, layers: [] });

    expect(stack.close(upper)).toEqual({ focus: 'page-button', enter: null });
    expect(stack.size).toBe(0);
  });

  it('forgets a layer closed twice', () => {
    const { stack, box, open } = world();
    const id = open(box('dialog'), { opener: 'page-button' });
    expect(stack.close(id)).toEqual({ focus: 'page-button', enter: null });
    expect(stack.close(id)).toBeNull();
  });
});

describe('LayerStack owned layers', () => {
  it('a listbox owned by a dialog counts as inside it, transitively, and for the dialog Tab trap', () => {
    const { stack, box, open } = world();
    const dialog = box('dialog', 'trigger');
    const dialogId = open(dialog);
    const listbox = box('listbox', 'option');
    const listboxId = open(listbox, { modal: false, owner: dialog, opener: 'trigger' });
    const submenu = box('submenu', 'deep');
    open(submenu, { modal: false, owner: listbox });
    const page = box('page menu', 'page item');
    const pageId = open(page, { modal: false });

    expect(stack.owned(dialog)).toEqual([
      { element: listbox, opener: 'trigger' },
      { element: submenu, opener: null },
    ]);
    expect(stack.within(dialog, 'option')).toBe(true);
    expect(stack.within(dialog, 'deep')).toBe(true);
    expect(stack.within(dialog, 'page item')).toBe(false);
    expect(stack.trapFor(dialogId)).toBe(dialog);
    expect(stack.trapFor(listboxId)).toBe(dialog);
    expect(stack.trapFor(pageId)).toBeNull();
  });

  it('focus may return into a layer the top dialog owns', () => {
    const { stack, box, open } = world();
    const dialog = box('dialog');
    open(dialog);
    const listbox = box('listbox', 'option');
    open(listbox, { modal: false, owner: dialog });
    const nested = open(box('confirm'), { opener: 'option' });

    expect(stack.close(nested)).toEqual({ focus: 'option', enter: null });
  });
});

describe('LayerStack outside press', () => {
  it('a press outside the top menu and what it owns closes it; inside, or for a dialog, nothing closes', () => {
    const { stack, box, open } = world();
    const dialog = box('dialog', 'dialog body', 'trigger');
    const dialogDismiss = vi.fn();
    open(dialog, { dismiss: dialogDismiss });
    expect(stack.pressOutside('page')).toBeNull();

    const listbox = box('listbox', 'option');
    const listboxDismiss = vi.fn();
    open(listbox, { modal: false, owner: dialog, opener: 'trigger', outsidePress: true, dismiss: listboxDismiss });

    expect(stack.pressOutside('option')).toBeNull();
    const press = stack.pressOutside('dialog body');
    press?.dismiss?.();
    expect(listboxDismiss).toHaveBeenCalledTimes(1);
    expect(press?.closed).toEqual({ focus: 'trigger', enter: null });
    expect(dialogDismiss).not.toHaveBeenCalled();
    expect(stack.pressOutside('dialog body')).toBeNull();
  });

  it('a menu that stays open on outside presses is left alone', () => {
    const { stack, box, open } = world();
    open(box('menu'), { modal: false, outsidePress: false });
    expect(stack.pressOutside('page')).toBeNull();
    expect(stack.size).toBe(1);
  });
});
