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
    holds: (anchor, target) => anchor === target,
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
    expect(stack.pressOutside('page')).toEqual([]);

    const listbox = box('listbox', 'option');
    const listboxDismiss = vi.fn();
    open(listbox, { modal: false, owner: dialog, opener: 'trigger', outsidePress: true, dismiss: listboxDismiss });

    expect(stack.pressOutside('option')).toEqual([]);
    const [press] = stack.pressOutside('dialog body');
    press?.dismiss?.();
    expect(listboxDismiss).toHaveBeenCalledTimes(1);
    expect(press?.closed).toEqual({ focus: 'trigger', enter: null });
    expect(dialogDismiss).not.toHaveBeenCalled();
    expect(stack.pressOutside('dialog body')).toEqual([]);
  });

  it('a press on the trigger it was opened from is not an outside press, on the page or in a dialog', () => {
    const { stack, box, open } = world();
    open(box('menu', 'item'), { modal: false, outsidePress: true, anchor: 'trigger' });
    expect(stack.pressOutside('trigger')).toEqual([]);
    expect(stack.pressOutside('elsewhere')).toHaveLength(1);
  });

  it('nested menus: a press outside both closes both, a press in the parent closes the submenu only', () => {
    const { stack, box, open } = world();
    const menu = box('menu', 'menu item', 'other item');
    const menuDismiss = vi.fn();
    const subDismiss = vi.fn();
    const openBoth = () => {
      open(menu, { modal: false, outsidePress: true, dismiss: menuDismiss });
      open(box('submenu', 'sub item'), { modal: false, outsidePress: true, owner: menu, anchor: 'menu item', dismiss: subDismiss });
    };
    openBoth();

    const presses = stack.pressOutside('page');
    for (const press of presses) press.dismiss?.();
    expect(presses).toHaveLength(2);
    expect(subDismiss).toHaveBeenCalledTimes(1);
    expect(menuDismiss).toHaveBeenCalledTimes(1);
    expect(stack.size).toBe(0);

    openBoth();
    expect(stack.pressOutside('menu item')).toEqual([]);
    const inParent = stack.pressOutside('other item');
    for (const press of inParent) press.dismiss?.();
    expect(inParent).toHaveLength(1);
    expect(subDismiss).toHaveBeenCalledTimes(2);
    expect(menuDismiss).toHaveBeenCalledTimes(1);
    expect(stack.size).toBe(1);
  });

  it('a menu that stays open on outside presses is left alone', () => {
    const { stack, box, open } = world();
    open(box('menu'), { modal: false, outsidePress: false });
    expect(stack.pressOutside('page')).toEqual([]);
    expect(stack.size).toBe(1);
  });
});

// A passive layer (a hint) takes no focus and stays open on outside presses: it must never stand in
// the way of the layers under it.
const HINT = { modal: false, outsidePress: false, passive: true } as const;

describe('LayerStack passive layers', () => {
  it('a press outside passes over a hint on top and closes the menu under it, answering focus as the top', () => {
    const { stack, box, open } = world();
    const menuDismiss = vi.fn();
    open(box('menu', 'item'), { modal: false, outsidePress: true, opener: 'trigger', dismiss: menuDismiss });
    const hint = box('hint', 'hint text');
    open(hint, HINT);

    const closed = stack.pressOutside('page');

    expect(closed.map((dismissal) => dismissal.dismiss)).toEqual([menuDismiss]);
    expect(closed[0].closed).toEqual({ focus: 'trigger', enter: null });
    expect(stack.elements()).toEqual([hint]);
  });

  it('a press on the hint itself, or in the menu under it, closes nothing', () => {
    const { stack, box, open } = world();
    open(box('menu', 'item'), { modal: false, outsidePress: true });
    open(box('hint', 'hint text'), HINT);

    expect(stack.pressOutside('hint text')).toEqual([]);
    expect(stack.pressOutside('item')).toEqual([]);
    expect(stack.size).toBe(2);
  });

  it('a layer that stays open but takes focus is not passive: it still holds the press back', () => {
    const { stack, box, open } = world();
    open(box('menu', 'item'), { modal: false, outsidePress: true });
    open(box('panel', 'field'), { modal: false, outsidePress: false });
    expect(stack.pressOutside('page')).toEqual([]);
    expect(stack.size).toBe(2);
  });

  it('a passive layer never answers focus when it closes, whatever it was given', () => {
    const { stack, box, open } = world();
    open(box('dialog', 'field'));
    const id = open(box('hint'), { ...HINT, opener: 'field', returnFocus: () => 'field' });
    expect(stack.close(id)).toBeNull();
    open(box('hint'), { ...HINT, opener: 'field' });
    expect(stack.dismissTop().closed).toBeNull();
  });

  it('closing a menu with only hints above it answers focus as if it were the top layer', () => {
    const { stack, box, open } = world();
    const menu = open(box('menu', 'item'), { modal: false, opener: 'trigger' });
    open(box('hint'), HINT);
    open(box('another hint'), HINT);
    expect(stack.close(menu)).toEqual({ focus: 'trigger', enter: null });
  });

  it('Escape still closes the top layer first, a hint included', () => {
    const { stack, box, open } = world();
    const menuDismiss = vi.fn();
    const hintDismiss = vi.fn();
    open(box('menu'), { modal: false, dismiss: menuDismiss });
    open(box('hint'), { ...HINT, dismiss: hintDismiss });
    expect(stack.dismissTop().dismiss).toBe(hintDismiss);
    expect(stack.dismissTop().dismiss).toBe(menuDismiss);
  });

  it('the top active layer is the one under the hints', () => {
    const { stack, box, open } = world();
    expect(stack.topActive()).toBeNull();
    const menu = box('menu');
    open(menu, { modal: false });
    open(box('hint'), HINT);
    expect(stack.top()).not.toBe(menu);
    expect(stack.topActive()).toBe(menu);
  });
});

describe('LayerStack a dialog opening over menus', () => {
  it('closes every non-modal layer open at that moment, menus owned by a lower dialog included', () => {
    const { stack, box, open } = world();
    const lower = box('lower dialog');
    open(lower);
    const listbox = vi.fn();
    const pageTip = vi.fn();
    const sticky = vi.fn();
    open(box('listbox'), { modal: false, owner: lower, dismiss: listbox });
    open(box('tooltip'), { modal: false, dismiss: pageTip });
    open(box('busy popover'), { modal: false, dismiss: sticky, dismissible: () => false });

    for (const dismiss of stack.dismissNonModal().dismiss) dismiss();

    expect(listbox).toHaveBeenCalledTimes(1);
    expect(pageTip).toHaveBeenCalledTimes(1);
    expect(sticky).not.toHaveBeenCalled();
    expect(stack.elements().map((element) => element.name)).toEqual(['lower dialog', 'busy popover']);
  });
});

describe('LayerStack a dialog opened from a menu item', () => {
  it('adopts the menu trigger as the way home, ahead of its own fallback', () => {
    const { stack, gone, box, open } = world();
    open(box('menu', 'item'), { modal: false, anchor: 'trigger', opener: 'trigger' });
    const { homes } = stack.dismissNonModal('item');
    expect(homes).toEqual(['trigger']);

    gone.add('item');
    const dialog = open(box('confirm'), { opener: 'item', adopted: homes, returnFocus: () => 'fallback' });
    expect(stack.close(dialog)).toEqual({ focus: 'trigger', enter: null });

    open(box('menu', 'item'), { modal: false, anchor: 'trigger' });
    const again = stack.dismissNonModal('item');
    gone.add('trigger');
    const second = open(box('confirm'), { opener: 'item', adopted: again.homes, returnFocus: () => 'fallback' });
    expect(stack.close(second)).toEqual({ focus: 'fallback', enter: null });
  });
});
