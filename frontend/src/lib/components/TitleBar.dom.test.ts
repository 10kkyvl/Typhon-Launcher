import { Window } from '@wailsio/runtime';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import TitleBar from './TitleBar.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));

const actions = () => ({
  minimise: vi.mocked(Window.Minimise).mock.calls.length,
  toggleMaximise: vi.mocked(Window.ToggleMaximise).mock.calls.length,
  close: vi.mocked(Window.Close).mock.calls.length,
});

function bar() {
  const view = renderDom(TitleBar);
  const caption = view.container.querySelector<HTMLElement>('.titlebar');
  if (!caption) throw new Error('no caption band');
  return {
    ...view,
    caption,
    minimise: view.getByRole('button', { name: msg('ui.minimize') }),
    maximise: view.getByRole('button', { name: msg('ui.maximize') }),
    close: view.getByRole('button', { name: msg('common.close') }),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('TitleBar in a DOM', () => {
  it.each([
    ['minimise', { minimise: 1, toggleMaximise: 0, close: 0 }],
    ['maximise', { minimise: 0, toggleMaximise: 1, close: 0 }],
    ['close', { minimise: 0, toggleMaximise: 0, close: 1 }],
  ] as const)('a click on %s runs that window action once', async (button, expected) => {
    const view = bar();

    await view.user.click(view[button]);

    expect(actions()).toEqual(expected);
  });

  it('a double-click on the empty caption toggles maximise once', async () => {
    const { user, caption } = bar();

    await user.dblClick(caption);

    expect(actions()).toEqual({ minimise: 0, toggleMaximise: 1, close: 0 });
  });

  it('a double-click on a caption button belongs to the button, not to the caption', async () => {
    const { user, minimise } = bar();

    await user.dblClick(minimise);

    expect(actions()).toEqual({ minimise: 2, toggleMaximise: 0, close: 0 });
  });

  it('keeps the window buttons out of the tab order', async () => {
    const { user, minimise, maximise, close } = bar();

    await user.tab();
    await user.tab();
    await user.tab();

    expect([minimise, maximise, close]).not.toContain(document.activeElement);
    expect(document.activeElement).toBe(document.body);
  });
});
