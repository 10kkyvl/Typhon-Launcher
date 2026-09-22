import { describe, expect, it } from 'vitest';
import { moveFocus, type BigPictureDirection } from './navigation';

interface FakeDocument {
  activeElement: FakeElement | null;
  defaultView: null;
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
  getAttribute(name: string): string | null;
  getBoundingClientRect(): FakeElement['rect'];
  getClientRects(): ArrayLike<unknown>;
  focus(): void;
  scrollIntoView(): void;
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
    scrollIntoView() {
      this.scrolled += 1;
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
});
