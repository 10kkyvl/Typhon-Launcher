import { fireEvent } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { user } from '../testing/dom';
import { contextMenuTrigger, type ContextRequest } from './contextmenu';

afterEach(() => {
  vi.useRealTimers();
  document.body.replaceChildren();
});

function row(answer = true) {
  const node = document.createElement('div');
  const control = document.createElement('button');
  control.textContent = 'Hades';
  node.append(control);
  const outside = document.createElement('button');
  outside.textContent = 'Outside';
  document.body.append(outside, node);
  const open = vi.fn((_request: ContextRequest, _source: HTMLElement) => answer);
  const stop = contextMenuTrigger(open)(node);
  return { node, control, outside, open, stop: () => stop?.() };
}

describe('contextMenuTrigger', () => {
  it('a right-click asks for the menu at the pointer and swallows the browser menu', async () => {
    const { node, control, open } = row();

    expect(await fireEvent.contextMenu(control, { clientX: 120, clientY: 80, button: 2 })).toBe(false);

    expect(open).toHaveBeenCalledExactlyOnceWith({ at: { x: 120, y: 80 }, focus: 'menu' }, node);
  });

  it('leaves the browser menu and the keys alone when there is no menu for it', async () => {
    const { control, open } = row(false);
    const keys = user();
    control.focus();

    expect(await fireEvent.contextMenu(control, { clientX: 120, clientY: 80, button: 2 })).toBe(true);
    expect(await fireEvent.keyDown(control, { key: 'F10', shiftKey: true })).toBe(true);
    await keys.keyboard('{ContextMenu}');

    expect(open).toHaveBeenCalledTimes(3);
  });

  it.each([
    ['Shift+F10', { key: 'F10', shiftKey: true }],
    ['the ContextMenu key', { key: 'ContextMenu' }],
  ])('%s asks for the menu next to the focused control and keeps the key from the browser', async (_key, init) => {
    const { node, control, open } = row();
    control.focus();

    expect(await fireEvent.keyDown(control, init)).toBe(false);

    expect(open).toHaveBeenCalledExactlyOnceWith({ at: { anchor: control }, focus: 'first' }, node);
  });

  it('F10 alone, or with Ctrl, Alt or Meta, asks for nothing', async () => {
    const { control, open } = row();
    control.focus();

    await fireEvent.keyDown(control, { key: 'F10' });
    await fireEvent.keyDown(control, { key: 'F10', shiftKey: true, ctrlKey: true });
    await fireEvent.keyDown(control, { key: 'ContextMenu', altKey: true });
    await fireEvent.keyDown(control, { key: 'ContextMenu', metaKey: true });

    expect(open).not.toHaveBeenCalled();
  });

  it('a held key asks once', async () => {
    const { control, open } = row();
    control.focus();

    await fireEvent.keyDown(control, { key: 'ContextMenu' });
    for (let i = 0; i < 5; i += 1) await fireEvent.keyDown(control, { key: 'ContextMenu', repeat: true });

    expect(open).toHaveBeenCalledTimes(1);
  });

  it('the browser following the key with a contextmenu event of its own does not ask again', async () => {
    vi.useFakeTimers();
    const { control, open } = row();
    control.focus();

    await fireEvent.keyDown(control, { key: 'ContextMenu' });
    expect(await fireEvent.contextMenu(control, { clientX: 130, clientY: 90, button: 2 })).toBe(false);
    expect(open).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(1000);
    await fireEvent.contextMenu(control, { clientX: 130, clientY: 90, button: 2 });
    expect(open).toHaveBeenCalledTimes(2);
    expect(open).toHaveBeenLastCalledWith({ at: { x: 130, y: 90 }, focus: 'menu' }, expect.any(HTMLElement));
  });

  it('a contextmenu event at 0,0 without the right button comes from the keyboard: next to the focused control', async () => {
    const { node, control, open } = row();
    control.focus();

    await fireEvent.contextMenu(control, { clientX: 0, clientY: 0, button: 0 });

    expect(open).toHaveBeenCalledExactlyOnceWith({ at: { anchor: control }, focus: 'first' }, node);
  });

  it('a right-click lets go of focus held by a control outside the element, and keeps focus inside it', async () => {
    const { control, outside } = row();
    outside.focus();

    await fireEvent.contextMenu(control, { clientX: 120, clientY: 80, button: 2 });
    expect(document.activeElement).toBe(document.body);

    control.focus();
    await fireEvent.contextMenu(control, { clientX: 120, clientY: 80, button: 2 });
    expect(document.activeElement).toBe(control);
  });

  it('listens to nothing once detached', async () => {
    const { control, open, stop } = row();
    stop();

    expect(await fireEvent.contextMenu(control, { clientX: 1, clientY: 1, button: 2 })).toBe(true);
    await fireEvent.keyDown(control, { key: 'ContextMenu' });

    expect(open).not.toHaveBeenCalled();
  });
});
