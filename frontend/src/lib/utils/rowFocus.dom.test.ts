import { afterEach, describe, expect, it, vi } from 'vitest';
import { menuOwner, nearOrder, rowFocus, type RowRule } from './rowFocus';

const row = (id: string) => `<li data-row="${id}"><button data-row-control="open">Open ${id}</button><button data-row-control="stop">Stop ${id}</button></li>`;

function page(sections: Record<string, string[]>) {
  document.body.innerHTML =
    '<h1 tabindex="-1">Page</h1>' +
    Object.entries(sections)
      .map(([name, ids]) => `<section data-part="${name}"><button data-part-first>First of ${name}</button><ul>${ids.map(row).join('')}</ul></section>`)
      .join('');
}

const button = (name: string) => Array.from(document.querySelectorAll('button')).find((b) => b.textContent === name) as HTMLButtonElement;
const dropRow = (id: string) => document.querySelector(`[data-row="${id}"]`)?.remove();
const partBy = (name: string | null) => Array.from(document.querySelectorAll<HTMLElement>('[data-part]')).find((el) => el.dataset.part === name) ?? null;

const scoped: RowRule = {
  rowAttr: 'data-row',
  scopeOf: (r) => r.closest<HTMLElement>('[data-part]')?.dataset.part ?? null,
  scopeBy: partBy,
  controlIn: (r, kind) => r.querySelector<HTMLElement>(`[data-row-control="${kind}"]`) ?? r.querySelector<HTMLElement>('button'),
  fallback: (place) => partBy(place.scope)?.querySelector<HTMLElement>('[data-part-first]') ?? document.querySelector<HTMLElement>('h1'),
};

const nativeScroll = Element.prototype.scrollIntoView;

afterEach(() => {
  document.body.replaceChildren();
  Element.prototype.scrollIntoView = nativeScroll;
});

describe('nearOrder', () => {
  it('tries the rows below first, then the rows above, nearest first', () => {
    expect(nearOrder(['a', 'b', 'c', 'd', 'e'], 'c')).toEqual(['d', 'e', 'b', 'a']);
  });

  it('answers nothing for an id the order never held', () => {
    expect(nearOrder(['a', 'b'], 'x')).toEqual([]);
  });
});

describe('rowFocus: the place of focus', () => {
  it('reads the row, its part, the part\'s rows in order and the kind of control', () => {
    page({ one: ['a', 'b'], two: ['c'] });
    const { placeOf } = rowFocus(scoped);

    expect(placeOf(button('Stop b'))).toEqual({ id: 'b', scope: 'one', order: ['a', 'b'], control: 'stop' });
  });

  it('answers no place for focus outside every row, or in a row the screen puts in no part', () => {
    page({ one: ['a'] });
    document.body.insertAdjacentHTML('beforeend', row('loose'));
    const { placeOf } = rowFocus(scoped);

    expect(placeOf(button('First of one'))).toBeNull();
    expect(placeOf(button('Open loose'))).toBeNull();
    expect(placeOf(null)).toBeNull();
  });

  it('reads every row of the page as one list when the screen has no parts', () => {
    page({ one: ['a'], two: ['b'] });
    const { placeOf } = rowFocus({ ...scoped, scopeOf: undefined, scopeBy: undefined });

    expect(placeOf(button('Open b'))).toEqual({ id: 'b', scope: null, order: ['a', 'b'], control: 'open' });
  });

  it('takes an item of an open menu for the menu\'s trigger in its row', () => {
    page({ one: ['a'] });
    const trigger = button('Stop a');
    trigger.id = 'trigger';
    document.body.insertAdjacentHTML('beforeend', '<div role="menu" aria-labelledby="trigger"><button role="menuitem">Remove</button></div>');
    const item = document.querySelector<HTMLElement>('[role="menuitem"]') as HTMLElement;

    expect(menuOwner(item)).toBe(trigger);
    expect(rowFocus({ ...scoped, standsFor: menuOwner }).placeOf(item)?.id).toBe('a');
    expect(rowFocus(scoped).placeOf(item)).toBeNull();
  });
});

describe('rowFocus: where focus goes', () => {
  it('stays in the row when only its control went', () => {
    page({ one: ['a', 'b'] });
    const { placeOf, leaveTarget } = rowFocus(scoped);
    const place = placeOf(button('Stop b'));
    button('Stop b').remove();

    expect(leaveTarget(place!)).toBe(button('Open b'));
  });

  it('goes to the same control of the row below', () => {
    page({ one: ['a', 'b', 'c'] });
    const { placeOf, leaveTarget } = rowFocus(scoped);
    const place = placeOf(button('Stop b'));
    dropRow('b');

    expect(leaveTarget(place!)).toBe(button('Stop c'));
  });

  it('goes to the row above when no row below is left, past rows that left too', () => {
    page({ one: ['a', 'b', 'c'] });
    const { placeOf, leaveTarget } = rowFocus(scoped);
    const place = placeOf(button('Stop c'));
    dropRow('c');
    dropRow('b');

    expect(leaveTarget(place!)).toBe(button('Stop a'));
  });

  it('never takes a row of another part, and falls back as the screen says', () => {
    page({ one: ['a'], two: ['b'] });
    const { placeOf, leaveTarget } = rowFocus(scoped);
    const place = placeOf(button('Stop a'));
    dropRow('a');

    expect(leaveTarget(place!)).toBe(button('First of one'));
  });
});

describe('rowFocus: the catch-all and a dialog\'s return', () => {
  it('hands focus on when the focused row left, and brings the row it lands in into view', async () => {
    page({ one: ['a', 'b'] });
    const scrolled = vi.fn();
    Element.prototype.scrollIntoView = scrolled;
    const { catchLeaving } = rowFocus(scoped);
    button('Stop a').focus();

    catchLeaving();
    dropRow('a');
    await vi.waitFor(() => expect(document.activeElement).toBe(button('Stop b')));
    await vi.waitFor(() => expect(scrolled.mock.contexts).toContain(document.querySelector('[data-row="b"]')));
  });

  it('leaves focus outside the rows alone', async () => {
    page({ one: ['a'] });
    const { catchLeaving } = rowFocus(scoped);
    const first = button('First of one');
    first.focus();

    catchLeaving();
    dropRow('a');
    await Promise.resolve();

    expect(document.activeElement).toBe(first);
  });

  it('answers the rule\'s target for a dialog, and nothing for no place', () => {
    page({ one: ['a', 'b'] });
    const { placeOf, returnTo } = rowFocus(scoped);
    const place = placeOf(button('Stop a'));
    dropRow('a');

    expect(returnTo(place)).toBe(button('Stop b'));
    expect(returnTo(null)).toBeNull();
  });
});
