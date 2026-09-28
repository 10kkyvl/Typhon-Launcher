import { describe, expect, it } from 'vitest';
import { focusControl, moveFocus, type BigPictureDirection } from './navigation';

interface FakeDocument {
  activeElement: FakeElement | null;
  defaultView: Pick<Window, 'requestAnimationFrame' | 'matchMedia'> | null;
  documentElement?: { classList: { contains: (name: string) => boolean } };
}

interface FakeElement {
  ownerDocument: FakeDocument;
  parentElement: FakeElement | null;
  disabled?: boolean;
  hidden?: boolean;
  style: { display: string; visibility: string };
  attrs: Map<string, string>;
  rect: { left: number; top: number; right: number; bottom: number; width: number; height: number };
  focused: number;
  scrolled: number;
  scrollOptions: Array<ScrollIntoViewOptions | undefined>;
  getAttribute(name: string): string | null;
  getBoundingClientRect(): FakeElement['rect'];
  getClientRects(): ArrayLike<unknown>;
  focus(): void;
  scrollIntoView(options?: ScrollIntoViewOptions): void;
}

function element(document: FakeDocument, left: number, top: number, name: string): FakeElement {
  const value: FakeElement = {
    ownerDocument: document,
    parentElement: null,
    style: { display: '', visibility: '' },
    attrs: new Map(),
    rect: { left, top, right: left + 80, bottom: top + 50, width: 80, height: 50 },
    focused: 0,
    scrolled: 0,
    scrollOptions: [],
    getAttribute(attribute) {
      return this.attrs.get(attribute) ?? null;
    },
    getBoundingClientRect() {
      return this.rect;
    },
    getClientRects() {
      return [this];
    },
    focus() {
      this.ownerDocument.activeElement = this;
      this.focused += 1;
    },
    scrollIntoView(options) {
      this.scrolled += 1;
      this.scrollOptions.push(options);
    },
  };
  value.attrs.set('data-bp-focus', name);
  return value;
}

function rootFor(document: FakeDocument, children: FakeElement[]) {
  return {
    ownerDocument: document,
    querySelectorAll: () => children,
  } as unknown as HTMLElement;
}

function move(document: FakeDocument, root: HTMLElement, direction: BigPictureDirection): boolean {
  return moveFocus(root, direction);
}

describe('moveFocus', () => {
  it('keeps controls outside labelled shelves reachable in mixed pages', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const header = element(document, 0, 0, 'header');
    header.attrs.set('data-bp-row', 'header');
    const search = element(document, 0, 100, 'search');
    const game = element(document, 0, 200, 'game');
    const root = rootFor(document, [header, search, game]);
    document.activeElement = header;
    expect(moveFocus(root, 'down')).toBe(true);
    expect(document.activeElement).toBe(search);
    expect(moveFocus(root, 'down')).toBe(true);
    expect(document.activeElement).toBe(game);
    expect(moveFocus(root, 'up')).toBe(true);
    expect(document.activeElement).toBe(search);
  });

  it('does not let a hidden page or inert dialog background steal focus after an async response', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const background = element(document, 0, 0, 'background');
    background.attrs.set('inert', '');
    const child = element(document, 0, 0, 'child'); child.parentElement = background;
    const dialog = element(document, 100, 0, 'dialog');
    document.activeElement = dialog;
    focusControl(child as unknown as HTMLButtonElement);
    expect(document.activeElement).toBe(dialog);
    expect(moveFocus(rootFor(document, [child, dialog]), 'left')).toBe(false);
    expect(document.activeElement).toBe(dialog);
  });
  it('chooses the nearest button in the aligned row or column and scrolls it into view', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const left = element(document, 0, 0, 'left');
    const middle = element(document, 100, 0, 'middle');
    const right = element(document, 200, 0, 'right');
    const below = element(document, 100, 100, 'below');
    const root = rootFor(document, [left, middle, right, below]);

    expect(move(document, root, 'right')).toBe(true);
    expect(document.activeElement).toBe(left);
    expect(left.scrolled).toBe(1);

    expect(move(document, root, 'right')).toBe(true);
    expect(document.activeElement).toBe(middle);
    expect(move(document, root, 'down')).toBe(true);
    expect(document.activeElement).toBe(below);
  });

  it('skips disabled and hidden controls and returns false at an edge', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const current = element(document, 0, 0, 'current');
    const disabled = element(document, 100, 0, 'disabled');
    disabled.disabled = true;
    const hidden = element(document, 200, 0, 'hidden');
    hidden.hidden = true;
    const visible = element(document, 300, 0, 'visible');
    const root = rootFor(document, [current, disabled, hidden, visible]);

    expect(move(document, root, 'right')).toBe(true);
    expect(document.activeElement).toBe(current);
    expect(move(document, root, 'right')).toBe(true);
    expect(document.activeElement).toBe(visible);
    expect(move(document, root, 'right')).toBe(false);
    expect(document.activeElement).toBe(visible);
  });

  it('uses a deterministic first target when focus is outside the root', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const top = element(document, 0, 0, 'top');
    const bottom = element(document, 0, 100, 'bottom');
    const root = rootFor(document, [bottom, top]);

    expect(move(document, root, 'down')).toBe(true);
    expect(document.activeElement).toBe(top);
    expect(move(document, root, 'down')).toBe(true);
    expect(document.activeElement).toBe(bottom);
  });

  it('excludes controls hidden by an ancestor, display:none, or empty client rectangles', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const current = element(document, 0, 0, 'current');
    const hiddenAncestor = element(document, 100, 0, 'ancestor');
    hiddenAncestor.hidden = true;
    const nested = element(document, 100, 0, 'nested');
    nested.parentElement = hiddenAncestor;
    const displayNone = element(document, 200, 0, 'display-none');
    displayNone.style.display = 'none';
    const emptyRects = element(document, 250, 0, 'empty-rects');
    emptyRects.getClientRects = () => [];
    const visible = element(document, 300, 0, 'visible');
    const root = rootFor(document, [current, nested, displayNone, emptyRects, visible]);

    expect(move(document, root, 'right')).toBe(true);
    expect(document.activeElement).toBe(current);
    expect(move(document, root, 'right')).toBe(true);
    expect(document.activeElement).toBe(visible);
  });

  it('stays at a horizontal rail edge instead of jumping to another row or header', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const recentFirst = element(document, 0, 100, 'recent-first');
    const recentLast = element(document, 100, 100, 'recent-last');
    const installedOnly = element(document, 0, 220, 'installed-only');
    const headerAction = element(document, 600, 0, 'header-action');
    const root = rootFor(document, [headerAction, recentFirst, recentLast, installedOnly]);

    document.activeElement = recentFirst;
    expect(move(document, root, 'right')).toBe(true);
    expect(document.activeElement).toBe(recentLast);
    expect(move(document, root, 'right')).toBe(false);
    expect(document.activeElement).toBe(recentLast);

    // Vertical navigation still finds the nearest lower row when its column
    // is offset and therefore has no strict rectangle overlap.
    expect(move(document, root, 'down')).toBe(true);
    expect(document.activeElement).toBe(installedOnly);
  });

  it('visits the short middle row before the aligned card in a longer row, in both directions', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const first = [0, 100, 200].map((x) => element(document, x, 100, `recent-${x}`));
    const middle = [0, 100].map((x) => element(document, x, 220, `favorite-${x}`));
    const last = [0, 100, 200, 300, 400].map((x) => element(document, x, 340, `installed-${x}`));
    [first, middle, last].forEach((row, index) => row.forEach((card) => card.attrs.set('data-bp-row', String(index))));
    const root = rootFor(document, [...first, ...middle, ...last]);

    document.activeElement = first[2];
    expect(moveFocus(root, 'down')).toBe(true);
    expect(document.activeElement).toBe(middle[1]);
    expect(moveFocus(root, 'down')).toBe(true);
    expect(document.activeElement).toBe(last[1]);

    document.activeElement = last[2];
    expect(moveFocus(root, 'up')).toBe(true);
    expect(document.activeElement).toBe(middle[1]);
    expect(moveFocus(root, 'up')).toBe(true);
    expect(document.activeElement).toBe(first[1]);
  });

  it('does not treat focus lift or unequal button heights as another row', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const current = element(document, 200, 97, 'lifted');
    const neighbour = element(document, 100, 100, 'same-row');
    const lowerLeft = element(document, 0, 220, 'lower-left');
    const lowerRight = element(document, 100, 220, 'lower-right');
    lowerLeft.rect.bottom += 20;
    lowerLeft.rect.height += 20;
    const last = element(document, 200, 340, 'last');
    const root = rootFor(document, [current, neighbour, lowerLeft, lowerRight, last]);

    document.activeElement = current;
    expect(moveFocus(root, 'down')).toBe(true);
    expect(document.activeElement).toBe(lowerRight);
    document.activeElement = last;
    expect(moveFocus(root, 'up')).toBe(true);
    expect(document.activeElement).toBe(lowerRight);
  });

  it('keeps shelf order when scrolling puts the fixed header between their screen coordinates', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null };
    const header = element(document, 200, 0, 'menu');
    const previousShelf = element(document, 100, -100, 'previous');
    const currentShelf = element(document, 200, 100, 'current');
    [header, previousShelf, currentShelf].forEach((card, index) => card.attrs.set('data-bp-row', String(index)));
    const root = rootFor(document, [header, previousShelf, currentShelf]);
    document.activeElement = currentShelf;

    expect(moveFocus(root, 'up')).toBe(true);
    expect(document.activeElement).toBe(previousShelf);
    expect(moveFocus(root, 'down')).toBe(true);
    expect(document.activeElement).toBe(currentShelf);
  });

  it('reveals only the latest focused card after layout, with smooth scrolling', () => {
    const frames: FrameRequestCallback[] = [];
    const document: FakeDocument = { activeElement: null, defaultView: {
      requestAnimationFrame: (callback) => frames.push(callback),
      matchMedia: () => ({ matches: false }) as MediaQueryList,
    } };
    const first = element(document, 0, 100, 'first');
    const second = element(document, 100, 100, 'second');
    const root = rootFor(document, [first, second]);

    moveFocus(root, 'right');
    moveFocus(root, 'right');
    moveFocus(root, 'left');
    expect(document.activeElement).toBe(first);
    expect(first.scrolled + second.scrolled).toBe(0);
    frames.forEach((frame) => frame(16));
    expect(second.scrolled).toBe(0);
    expect(first.scrollOptions).toEqual([{ block: 'nearest', inline: 'nearest', behavior: 'smooth' }]);
  });

  it('respects reduced motion when revealing the focused card', () => {
    const document: FakeDocument = { activeElement: null, defaultView: {
      requestAnimationFrame: (callback) => { callback(16); return 1; },
      matchMedia: () => ({ matches: true }) as MediaQueryList,
    } };
    const card = element(document, 0, 100, 'card');
    moveFocus(rootFor(document, [card]), 'down');
    expect(card.scrollOptions[0]?.behavior).toBe('instant');
  });

  it('respects the launcher animation preference even when the OS allows motion', () => {
    const document: FakeDocument = { activeElement: null, defaultView: null,
      documentElement: { classList: { contains: (name) => name === 'no-anim' } } };
    const card = element(document, 0, 100, 'card');
    moveFocus(rootFor(document, [card]), 'down');
    expect(card.scrollOptions[0]?.behavior).toBe('instant');
  });
});
